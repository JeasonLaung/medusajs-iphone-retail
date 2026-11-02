package cart

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ContextCommand contains only the intended directory choices. Currency,
// channel, address policy and every price are resolved inside the transaction.
type ContextCommand struct {
	OperationID string `json:"-"`
	Version     string `json:"version"`
	RegionID    string `json:"region_id"`
	Locale      string `json:"locale"`
}
type ContextResult struct {
	CartID   string `json:"cart_id"`
	Version  string `json:"version"`
	Replayed bool   `json:"replayed"`
}

func validateContext(in ContextCommand) (uint64, error) {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID || e != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version || !idPattern.MatchString(in.RegionID) || len(in.Locale) > 35 || !localePattern.MatchString(in.Locale) {
		return 0, ErrInput
	}
	return version, nil
}

// SaveContext atomically applies the source region/locale policy and refreshes
// snapshots, prices, delivery, taxes and promotions before committing a receipt.
// Existing provider collections are refused until provider refresh coordination
// is connected. This command never calls a provider or changes payment facts.
// manualProviders must come from server registrations.
func SaveContext(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ContextCommand, manualProviders []string) (ContextResult, error) {
	version, err := validateContext(in)
	if err != nil || !refOK(store, ref) || db == nil || db.Statement == nil {
		return ContextResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ContextResult{}, ErrInput
	}
	if len(manualProviders) > 100 {
		return ContextResult{}, ErrInput
	}
	for _, id := range manualProviders {
		if !idPattern.MatchString(id) {
			return ContextResult{}, ErrInput
		}
	}
	request, _ := json.Marshal(in)
	digest := sha256.Sum256(request)
	var result ContextResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_context_runtime WHERE id=1 FOR SHARE").Scan(&gate)
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
		r = tx.Raw("SELECT generation,HEX(request_sha256) AS hash,result_version FROM cart_context_operations WHERE cart_id=? AND operation_id=? FOR SHARE", current.ID, in.OperationID).Scan(&old)
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
			result = ContextResult{current.ID, strconv.FormatUint(old.ResultVersion, 10), true}
			return nil
		}
		if current.Version != in.Version {
			return ErrConflict
		}
		plan, err := resolveContextPlan(ctx, tx, store, current, in)
		if err != nil {
			return err
		}
		if _, err := reusablePaymentCollection(ctx, tx, current.ID); err != nil {
			return err
		}

		current, err = applyContextPlan(ctx, tx, store, ref, current, in, plan, manualProviders)
		if err != nil {
			return err
		}
		if _, err := assembleAmounts(tx, current); err != nil {
			return err
		}
		if err := syncEmptyPaymentCollection(ctx, tx, store, ref, in.Version); err != nil {
			return err
		}
		if r := tx.Exec("UPDATE cart_ownership SET version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", current.ID, version); r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		if tx.Exec("INSERT INTO cart_context_operations(cart_id,operation_id,generation,request_sha256,result_version,created_at) VALUES(?,?,?,UNHEX(?),?,UTC_TIMESTAMP(6))", current.ID, in.OperationID, ref.Generation, hex.EncodeToString(digest[:]), version+1).Error != nil {
			return ErrStorage
		}
		result = ContextResult{current.ID, strconv.FormatUint(version+1, 10), false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		if errors.Is(err, ErrItemUnavailable) || errors.Is(err, ErrItemStock) {
			return ContextResult{}, err
		}
		return ContextResult{}, sanitize(err)
	}
	return result, nil
}

// resolveContextPlan and applyContextPlan participate in the caller's transaction.
// They never advance a version, write a receipt, dispatch a provider or commit.
func resolveContextPlan(ctx context.Context, tx *gorm.DB, store string, current Snapshot, in ContextCommand) (ContextPlan, error) {
	currency, err := regionCurrency(ctx, tx, store, in.RegionID, in.Locale)
	if err != nil {
		return ContextPlan{}, err
	}
	var countries []string
	if tx.Raw("SELECT iso_2 FROM commerce_countries WHERE region_id=? AND deleted_at IS NULL ORDER BY iso_2 LIMIT 1001 FOR SHARE", in.RegionID).Scan(&countries).Error != nil || len(countries) > 1000 {
		return ContextPlan{}, ErrSchema
	}
	return PlanContext(ContextState{Region: ContextRegion{ID: current.RegionID, Currency: current.CurrencyCode}, Locale: &current.Locale}, ContextChange{RegionID: in.RegionID, Locale: ContextField[string]{Present: true, Value: &in.Locale}}, ContextRegion{ID: in.RegionID, Currency: currency, Countries: countries})
}

func applyContextPlan(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, current Snapshot, in ContextCommand, plan ContextPlan, manualProviders []string) (Snapshot, error) {
	if err := supportedSavedShipping(tx, current.ID, manualProviders); err != nil {
		return Snapshot{}, err
	}
	var shippingID *string
	if plan.Address.Present && plan.Address.Value != nil {
		id := "caaddr_" + uuid.NewString()
		country := (*plan.Address.Value)["country_code"]
		if country == nil {
			return Snapshot{}, ErrSchema
		}
		if tx.Exec("INSERT INTO cart_addresses(id,country_code,created_at,updated_at) VALUES(?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, *country).Error != nil {
			return Snapshot{}, ErrStorage
		}
		shippingID = &id
	}
	if plan.Address.Present {
		if r := tx.Exec("UPDATE carts SET region_id=?,currency_code=?,locale=?,shipping_address_id=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", plan.RegionID, plan.Currency, in.Locale, shippingID, current.ID); r.Error != nil || r.RowsAffected != 1 {
			return Snapshot{}, ErrStorage
		}
	} else {
		if r := tx.Exec("UPDATE carts SET region_id=?,currency_code=?,locale=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND deleted_at IS NULL", plan.RegionID, plan.Currency, in.Locale, current.ID); r.Error != nil || r.RowsAffected != 1 {
			return Snapshot{}, ErrStorage
		}
	}
	// The source deletes custom-priced lines on region change, rather than
	// converting their old currency amount to a normal price.
	if plan.RemoveCustomPrices {
		if tx.Exec("UPDATE cart_line_items SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND is_custom_price=TRUE AND deleted_at IS NULL", current.ID).Error != nil {
			return Snapshot{}, ErrStorage
		}
	}
	current, err := LockForWrite(ctx, tx, store, ref, in.Version)
	if err != nil {
		return Snapshot{}, err
	}
	at := time.Now().UTC()
	if _, err := RefreshItemSnapshots(ctx, tx, store, ref, in.Version); err != nil {
		return Snapshot{}, err
	}
	if _, err := RefreshItemPrices(ctx, tx, store, ref, in.Version, at); err != nil {
		return Snapshot{}, err
	}
	if _, err := RefreshManualShippingMethods(ctx, tx, store, ref, in.Version, manualProviders, at); err != nil {
		return Snapshot{}, err
	}
	// Source force_refresh selects replacement; it does not set
	// force_tax_calculation. Missing address or disabled automatic tax
	// therefore clears old tax lines instead of forcing a calculation.
	if _, err := RefreshSystemTaxes(ctx, tx, store, ref, in.Version, false); err != nil {
		return Snapshot{}, err
	}
	if _, err := RefreshPromotions(ctx, tx, store, ref, in.Version, PromotionRefresh{At: at}); err != nil {
		return Snapshot{}, err
	}
	return current, nil
}
