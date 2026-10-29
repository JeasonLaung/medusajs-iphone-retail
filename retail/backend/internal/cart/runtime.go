package cart

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInput         = errors.New("invalid cart command")
	ErrDenied        = errors.New("cart authority unavailable")
	ErrConflict      = errors.New("cart operation or version changed")
	ErrConfiguration = errors.New("store cart configuration unavailable")
	ErrRegion        = errors.New("cart region unavailable")
	ErrStorage       = errors.New("cart transaction unavailable")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`)
var generationPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var localePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)
var currencyPattern = regexp.MustCompile(`^[a-z]{3}$`)

// CreateCommand is internal. StoreID comes from server configuration; ScopeHash
// comes from a separately authenticated server-created anonymous session. An
// HTTP caller must never be permitted to choose either field. No legacy ID,
// email or customer principal can substitute for this scope.
type CreateCommand struct {
	OperationID string
	ScopeHash   [32]byte
	StoreID     string
	RegionID    string
	Locale      string
}

type CreateResult struct {
	Reference sessionstore.CartReference
	Replayed  bool
}

// Snapshot deliberately excludes email, address, metadata and source customer
// IDs. Lines/totals require a separate trusted assembler, not a payload dump.
type Snapshot struct {
	ID             string `json:"id"`
	StoreID        string `json:"-"`
	RegionID       string `json:"region_id"`
	SalesChannelID string `json:"sales_channel_id"`
	CurrencyCode   string `json:"currency_code"`
	Locale         string `json:"locale"`
	Version        string `json:"version"`
}

func validCreate(in CreateCommand) bool {
	id, err := uuid.Parse(in.OperationID)
	return err == nil && id.Version() == 4 && id.String() == in.OperationID && in.ScopeHash != ([32]byte{}) && idPattern.MatchString(in.StoreID) && idPattern.MatchString(in.RegionID) && (in.Locale == "" || len(in.Locale) <= 35 && localePattern.MatchString(in.Locale))
}

func commandReady(ctx context.Context, tx *gorm.DB, command string) error {
	if tx == nil || tx.Statement == nil {
		return ErrInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrInput
	}
	shared := tx.Clauses(clause.Locking{Strength: "SHARE"})
	if Ready(ctx, shared) != nil || catalog.Ready(ctx, shared) != nil {
		return ErrSchema
	}
	var cap struct {
		Version int
		Enabled bool
	}
	r := tx.WithContext(ctx).Raw("SELECT version,enabled FROM cart_command_capabilities WHERE command=? FOR SHARE", command).Scan(&cap)
	if r.Error != nil || r.RowsAffected != 1 || cap.Version != 1 || !cap.Enabled {
		return ErrSchema
	}
	return nil
}

func storeChannel(ctx context.Context, tx *gorm.DB, store string) (string, error) {
	var channel string
	r := tx.WithContext(ctx).Raw(`SELECT c.id FROM commerce_stores s JOIN commerce_sales_channels c ON c.id=s.default_sales_channel_id AND c.deleted_at IS NULL AND c.is_disabled=FALSE WHERE s.id=? AND s.deleted_at IS NULL FOR SHARE`, store).Scan(&channel)
	if r.Error != nil || r.RowsAffected != 1 || !idPattern.MatchString(channel) {
		return "", ErrConfiguration
	}
	return channel, nil
}

func regionCurrency(ctx context.Context, tx *gorm.DB, store, region, locale string) (string, error) {
	var currency string
	r := tx.WithContext(ctx).Raw(`SELECT currency_code FROM commerce_regions WHERE id=? AND deleted_at IS NULL FOR SHARE`, region).Scan(&currency)
	if r.Error != nil || r.RowsAffected != 1 || !currencyPattern.MatchString(currency) {
		return "", ErrRegion
	}
	var currencies []string
	if tx.WithContext(ctx).Raw(`SELECT id FROM commerce_store_currencies WHERE store_id=? AND currency_code=? AND deleted_at IS NULL LIMIT 2 FOR SHARE`, store, currency).Scan(&currencies).Error != nil || len(currencies) != 1 {
		return "", ErrRegion
	}
	// If locales are configured, honor that directory. Without a directory the
	// original optional locale remains supported; syntax is still bounded.
	if locale != "" {
		var locales []string
		if tx.WithContext(ctx).Raw(`SELECT locale_code FROM commerce_store_locales WHERE store_id=? AND deleted_at IS NULL LIMIT 101 FOR SHARE`, store).Scan(&locales).Error != nil || len(locales) > 100 {
			return "", ErrConfiguration
		}
		found := len(locales) == 0
		for _, code := range locales {
			found = found || code == locale
		}
		if !found {
			return "", ErrRegion
		}
	}
	return currency, nil
}

// Create commits the empty cart, anonymous authority and creation receipt as
// one MySQL transaction. Redis issuance happens only AFTER this returns. On
// uncertain commit/issuance, retry the SAME authenticated scope and operation.
// No lines, prices, stock reservation, customer ownership or payment is created.
func Create(ctx context.Context, db *gorm.DB, in CreateCommand) (CreateResult, error) {
	if db == nil || !validCreate(in) {
		return CreateResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return CreateResult{}, ErrInput
	}
	request, _ := json.Marshal(struct{ StoreID, RegionID, Locale string }{in.StoreID, in.RegionID, in.Locale})
	hash := sha256.Sum256(request)
	var result CreateResult
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.create"); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, in.StoreID)
		if err != nil {
			return err
		}
		// Serialize the authenticated scope before looking up its operation.
		// A missing receipt must not leave a concurrent first-create race.
		if tx.Exec(`INSERT INTO cart_creation_scopes(scope_sha256,created_at) VALUES (UNHEX(?),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE scope_sha256=scope_sha256`, hex.EncodeToString(in.ScopeHash[:])).Error != nil {
			return ErrStorage
		}
		var old struct {
			RequestSHA256 []byte
			CartID        string
			Generation    string
		}
		r := tx.Raw(`SELECT request_sha256,cart_id,generation FROM cart_creation_operations WHERE scope_sha256=? AND operation_id=? FOR UPDATE`, in.ScopeHash[:], in.OperationID).Scan(&old)
		if r.Error != nil {
			return ErrStorage
		}
		if r.RowsAffected == 1 {
			if subtle.ConstantTimeCompare(old.RequestSHA256, hash[:]) != 1 {
				return ErrConflict
			}
			ref := sessionstore.CartReference{CartID: old.CartID, StoreID: in.StoreID, Generation: old.Generation}
			if _, err := lockCurrent(ctx, tx, in.StoreID, channel, ref, "UPDATE"); err != nil {
				return err
			}
			result = CreateResult{Reference: ref, Replayed: true}
			return nil
		}
		currency, err := regionCurrency(ctx, tx, in.StoreID, in.RegionID, in.Locale)
		if err != nil {
			return err
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return ErrStorage
		}
		var generation [32]byte
		if _, err := rand.Read(generation[:]); err != nil {
			return ErrStorage
		}
		ref := sessionstore.CartReference{CartID: "cart_" + id.String(), StoreID: in.StoreID, Generation: hex.EncodeToString(generation[:])}
		if tx.Exec(`INSERT INTO carts(id,region_id,sales_channel_id,currency_code,locale,created_at,updated_at) VALUES (?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID, in.RegionID, channel, currency, in.Locale).Error != nil ||
			tx.Exec(`INSERT INTO cart_ownership(cart_id,store_id,generation,version,active,created_at,updated_at) VALUES (?,?,?,1,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID, ref.StoreID, ref.Generation).Error != nil ||
			tx.Exec(`INSERT INTO cart_creation_operations(scope_sha256,operation_id,request_sha256,cart_id,generation,created_at) VALUES (UNHEX(?),?,UNHEX(?),?,?,UTC_TIMESTAMP(6))`, hex.EncodeToString(in.ScopeHash[:]), in.OperationID, hex.EncodeToString(hash[:]), ref.CartID, ref.Generation).Error != nil {
			return ErrStorage
		}
		result.Reference = ref
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return CreateResult{}, sanitize(err)
	}
	return result, nil
}

func lockCurrent(ctx context.Context, tx *gorm.DB, store, channel string, ref sessionstore.CartReference, strength string) (Snapshot, error) {
	return lockCurrentPaymentOperation(ctx, tx, store, channel, ref, strength, "")
}

func lockCurrentPaymentOperation(ctx context.Context, tx *gorm.DB, store, channel string, ref sessionstore.CartReference, strength, operation string) (Snapshot, error) {
	return lockCurrentAuthority(ctx, tx, store, channel, ref, strength, operation, "")
}

func lockCurrentAuthority(ctx context.Context, tx *gorm.DB, store, channel string, ref sessionstore.CartReference, strength, operation, checkout string) (Snapshot, error) {
	var owner struct {
		StoreID, Generation       string
		Version                   uint64
		Active                    bool
		PaymentRefreshOperationID *string
		CheckoutAttemptID         *string
	}
	r := tx.WithContext(ctx).Table("cart_ownership").Select("store_id,generation,version,active,payment_refresh_operation_id,checkout_attempt_id").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: strength}).Take(&owner)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return Snapshot{}, ErrDenied
	}
	if r.Error != nil {
		return Snapshot{}, ErrStorage
	}
	if !owner.Active || owner.Version == 0 || owner.StoreID != store || subtle.ConstantTimeCompare([]byte(owner.Generation), []byte(ref.Generation)) != 1 {
		return Snapshot{}, ErrDenied
	}
	if strength == "UPDATE" && owner.PaymentRefreshOperationID != nil && *owner.PaymentRefreshOperationID != operation {
		return Snapshot{}, ErrConflict
	}
	if strength == "UPDATE" && owner.CheckoutAttemptID != nil && *owner.CheckoutAttemptID != checkout {
		return Snapshot{}, ErrConflict
	}
	var row struct {
		ID, RegionID, SalesChannelID, CurrencyCode string
		Locale, CustomerID                         *string
		CompletedAt, DeletedAt                     *time.Time
	}
	r = tx.WithContext(ctx).Table("carts").Select("id,region_id,sales_channel_id,currency_code,locale,customer_id,completed_at,deleted_at").Where("id=?", ref.CartID).Clauses(clause.Locking{Strength: strength}).Take(&row)
	if errors.Is(r.Error, gorm.ErrRecordNotFound) {
		return Snapshot{}, ErrDenied
	}
	if r.Error != nil {
		return Snapshot{}, ErrStorage
	}
	if row.CompletedAt != nil || row.DeletedAt != nil || row.CustomerID != nil || row.SalesChannelID != channel || !idPattern.MatchString(row.RegionID) {
		return Snapshot{}, ErrDenied
	}
	locale := ""
	if row.Locale != nil {
		locale = *row.Locale
	}
	currency, err := regionCurrency(ctx, tx, store, row.RegionID, locale)
	if err != nil || currency != row.CurrencyCode {
		return Snapshot{}, ErrConfiguration
	}
	return Snapshot{row.ID, store, row.RegionID, channel, currency, locale, strconv.FormatUint(owner.Version, 10)}, nil
}

func refOK(store string, ref sessionstore.CartReference) bool {
	return idPattern.MatchString(store) && store == ref.StoreID && idPattern.MatchString(ref.CartID) && generationPattern.MatchString(ref.Generation)
}

// Inspect rechecks current durable authority; it never accepts a raw cart ID.
// The caller must first resolve possession using the Redis component and bind
// StoreID to configuration, not request parameters. There is no HTTP route yet.
func Inspect(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference) (Snapshot, error) {
	if db == nil || !refOK(store, ref) {
		return Snapshot{}, ErrDenied
	}
	var result Snapshot
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		result, err = lockCurrent(ctx, tx, store, channel, ref, "SHARE")
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
	if err != nil {
		return Snapshot{}, sanitize(err)
	}
	return result, nil
}

// LockForWrite participates in the caller's transaction and never commits. It
// rechecks current authority and optimistic version under locks. Callers must
// still enforce CSRF/Origin/idempotency, cart-specific business rules, update
// the version and roll back the WHOLE transaction after any later failure.
func LockForWrite(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (Snapshot, error) {
	return lockForPaymentOperation(ctx, tx, store, ref, version, "")
}

func lockForPaymentOperation(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version, operation string) (Snapshot, error) {
	n, err := strconv.ParseUint(version, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != version || !refOK(store, ref) {
		return Snapshot{}, ErrInput
	}
	if err := commandReady(ctx, tx, "cart.access"); err != nil {
		return Snapshot{}, err
	}
	channel, err := storeChannel(ctx, tx, store)
	if err != nil {
		return Snapshot{}, err
	}
	result, err := lockCurrentPaymentOperation(ctx, tx, store, channel, ref, "UPDATE", operation)
	if err != nil {
		return Snapshot{}, err
	}
	if result.Version != version {
		return Snapshot{}, ErrConflict
	}
	return result, nil
}

func sanitize(err error) error {
	for _, known := range []error{ErrInput, ErrDenied, ErrConflict, ErrConfiguration, ErrRegion, ErrSchema, ErrStorage} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrStorage
}
