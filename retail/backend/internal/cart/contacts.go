package cart

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ContactAddress is a value copy, never a customer/history address identifier.
type ContactAddress struct {
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Company    string `json:"company"`
	Address1   string `json:"address_1"`
	Address2   string `json:"address_2"`
	City       string `json:"city"`
	Country    string `json:"country_code"`
	Province   string `json:"province"`
	PostalCode string `json:"postal_code"`
	Phone      string `json:"phone"`
}
type ContactCommand struct {
	OperationID string         `json:"-"`
	Version     string         `json:"version"`
	Email       string         `json:"email"`
	Shipping    ContactAddress `json:"shipping_address"`
	Billing     ContactAddress `json:"billing_address"`
}
type ContactResult struct {
	CartID   string `json:"cart_id"`
	Version  string `json:"version"`
	Replayed bool   `json:"replayed"`
}

func validContactAddress(a ContactAddress) bool {
	for _, field := range []struct {
		value    string
		max      int
		required bool
	}{
		{a.FirstName, 160, true}, {a.LastName, 160, true}, {a.Company, 160, false},
		{a.Address1, 512, true}, {a.Address2, 512, false}, {a.City, 160, true},
		{a.Province, 191, false}, {a.PostalCode, 64, true}, {a.Phone, 64, false},
	} {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.max || strings.ContainsFunc(field.value, func(r rune) bool { return r < 32 || r == 127 }) || field.required && strings.TrimSpace(field.value) == "" {
			return false
		}
	}
	return len(a.Country) == 2 && a.Country[0] >= 'a' && a.Country[0] <= 'z' && a.Country[1] >= 'a' && a.Country[1] <= 'z'
}
func validateContact(in ContactCommand) (uint64, error) {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	parsed, f := mail.ParseAddress(in.Email)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID || e != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version ||
		f != nil || parsed.Address != in.Email || parsed.Name != "" || len(in.Email) > 254 || !utf8.ValidString(in.Email) || strings.ContainsFunc(in.Email, func(r rune) bool { return r < 32 || r == 127 }) || strings.TrimSpace(in.Email) != in.Email || strings.ContainsAny(in.Email, "\r\n") || !validContactAddress(in.Shipping) || !validContactAddress(in.Billing) {
		return 0, ErrInput
	}
	return version, nil
}

// SaveContacts is the complete pre-payment contact transaction: persist copied
// addresses, refresh all local amounts and commit one version/receipt together.
// Provider-backed payment collections remain closed until their durable refresh
// orchestration is attached; no local status can substitute for provider work.
// manualProviders must be server registrations, never request data.
func SaveContacts(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContactCommand, manualProviders []string) (ContactResult, error) {
	version, err := validateContact(in)
	if err != nil || !refOK(store, ref) || db == nil || db.Statement == nil {
		return ContactResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ContactResult{}, ErrInput
	}
	if len(manualProviders) > 100 {
		return ContactResult{}, ErrInput
	}
	for _, id := range manualProviders {
		if !idPattern.MatchString(id) {
			return ContactResult{}, ErrInput
		}
	}
	request, _ := json.Marshal(in)
	digest := sha256.Sum256(request)
	var result ContactResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_contact_runtime WHERE id=1 FOR SHARE").Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		current, err := lockCurrent(ctx, tx, store, channel, ref, "UPDATE")
		if err != nil {
			return err
		}
		var old struct {
			Generation, Hash string
			ResultVersion    uint64
		}
		r = tx.Raw("SELECT generation,HEX(request_sha256) AS hash,result_version FROM cart_contact_operations WHERE cart_id=? AND operation_id=? FOR SHARE", current.ID, in.OperationID).Scan(&old)
		if r.Error != nil {
			return ErrStorage
		}
		if r.RowsAffected == 1 {
			currentVersion, _ := strconv.ParseUint(current.Version, 10, 64)
			if old.Generation != ref.Generation || subtle.ConstantTimeCompare([]byte(strings.ToLower(old.Hash)), []byte(hex.EncodeToString(digest[:]))) != 1 {
				return ErrConflict
			}
			if old.ResultVersion < 2 || old.ResultVersion > currentVersion {
				return ErrStorage
			}
			result = ContactResult{current.ID, strconv.FormatUint(old.ResultVersion, 10), true}
			return nil
		}
		if current.Version != in.Version {
			return ErrConflict
		}
		var countries []struct{ Country, Region string }
		if tx.Table("commerce_countries").Select("iso_2 AS country,COALESCE(region_id,'') AS region").Where("deleted_at IS NULL AND iso_2 IN ?", []string{in.Shipping.Country, in.Billing.Country}).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&countries).Error != nil {
			return ErrSchema
		}
		shipping, billing := false, false
		for _, country := range countries {
			shipping = shipping || country.Country == in.Shipping.Country && country.Region == current.RegionID
			billing = billing || country.Country == in.Billing.Country
		}
		if !shipping || !billing {
			return ErrRegion
		}
		// Refuse an attached payment before any address/amount changes. A subsequent
		// iteration will coordinate its provider invalidation rather than delete it.
		var links []string
		if tx.Table("cart_payment_collections").Select("payment_collection_id").Where("cart_id=? AND deleted_at IS NULL", current.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1).Scan(&links).Error != nil {
			return ErrSchema
		}
		if len(links) > 0 {
			return ErrConflict
		}
		if err := supportedSavedShipping(tx, current.ID, manualProviders); err != nil {
			return err
		}
		shippingID, billingID := "caaddr_"+uuid.NewString(), "caaddr_"+uuid.NewString()
		for _, address := range []struct {
			id    string
			value ContactAddress
		}{{shippingID, in.Shipping}, {billingID, in.Billing}} {
			a := address.value
			if tx.Exec(`INSERT INTO cart_addresses(id,first_name,last_name,company,address_1,address_2,city,country_code,province,postal_code,phone,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, address.id, a.FirstName, a.LastName, a.Company, a.Address1, a.Address2, a.City, a.Country, a.Province, a.PostalCode, a.Phone).Error != nil {
				return ErrStorage
			}
		}
		if r := tx.Exec("UPDATE carts SET email=?,shipping_address_id=?,billing_address_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", in.Email, shippingID, billingID, current.ID); r.Error != nil || r.RowsAffected != 1 {
			return ErrStorage
		}
		at := time.Now().UTC()
		if _, err := RefreshItemSnapshots(ctx, tx, store, ref, in.Version); err != nil {
			return err
		}
		if _, err := RefreshItemPrices(ctx, tx, store, ref, in.Version, at); err != nil {
			return err
		}
		if _, err := RefreshManualShippingMethods(ctx, tx, store, ref, in.Version, manualProviders, at); err != nil {
			return err
		}
		if _, err := RefreshSystemTaxes(ctx, tx, store, ref, in.Version, true); err != nil {
			return err
		}
		if _, err := RefreshPromotions(ctx, tx, store, ref, in.Version, PromotionRefresh{At: at}); err != nil {
			return err
		}
		if _, err := assembleAmounts(tx, current); err != nil {
			return err
		}
		if r := tx.Exec("UPDATE cart_ownership SET version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", current.ID, version); r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		if tx.Exec("INSERT INTO cart_contact_operations(cart_id,operation_id,generation,request_sha256,result_version,created_at) VALUES(?,?,?,UNHEX(?),?,UTC_TIMESTAMP(6))", current.ID, in.OperationID, ref.Generation, hex.EncodeToString(digest[:]), version+1).Error != nil {
			return ErrStorage
		}
		result = ContactResult{current.ID, strconv.FormatUint(version+1, 10), false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		if errors.Is(err, ErrItemUnavailable) || errors.Is(err, ErrItemStock) {
			return ContactResult{}, err
		}
		return ContactResult{}, sanitize(err)
	}
	return result, nil
}
