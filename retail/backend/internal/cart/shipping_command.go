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
	"gorm.io/gorm/clause"
)

type ShippingCommand struct {
	OperationID string `json:"-"`
	Version     string `json:"version"`
	OptionID    string `json:"option_id"`
}
type ShippingResult struct {
	CartID   string `json:"cart_id"`
	Version  string `json:"version"`
	OptionID string `json:"option_id"`
	Replayed bool   `json:"replayed"`
}

func validateShipping(in ShippingCommand) (uint64, error) {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID || e != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version || !idPattern.MatchString(in.OptionID) {
		return 0, ErrInput
	}
	return version, nil
}
func shippingContactReady(tx *gorm.DB, current Snapshot) error {
	detail, err := readContactValues(tx, current)
	if err != nil {
		return err
	}
	if detail.Email == "" || detail.Shipping == nil || detail.Billing == nil {
		return ErrInput
	}
	return nil
}

// SaveShipping selects one current manual fixed-price option and refreshes all
// local amounts, committing its version and immutable receipt atomically.
// Provider-backed payment collections remain closed until their durable refresh
// orchestration is attached; no local status can substitute for provider work.
// manualProviders must be server registrations, never request data.
func SaveShipping(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in ShippingCommand, manualProviders []string) (ShippingResult, error) {
	version, err := validateShipping(in)
	if err != nil || !refOK(store, ref) || db == nil || db.Statement == nil {
		return ShippingResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ShippingResult{}, ErrInput
	}
	if len(manualProviders) > 100 {
		return ShippingResult{}, ErrInput
	}
	for _, id := range manualProviders {
		if !idPattern.MatchString(id) {
			return ShippingResult{}, ErrInput
		}
	}
	request, _ := json.Marshal(in)
	digest := sha256.Sum256(request)
	var result ShippingResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_shipping_command_runtime WHERE id=1 FOR SHARE").Scan(&gate)
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
		r = tx.Raw("SELECT generation,HEX(request_sha256) AS hash,result_version FROM cart_shipping_command_operations WHERE cart_id=? AND operation_id=? FOR SHARE", current.ID, in.OperationID).Scan(&old)
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
			result = ShippingResult{current.ID, strconv.FormatUint(old.ResultVersion, 10), in.OptionID, true}
			return nil
		}
		if current.Version != in.Version {
			return ErrConflict
		}
		if err := shippingContactReady(tx, current); err != nil {
			return err
		}
		// Refuse an attached payment before any method/amount changes. A subsequent
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
		at := time.Now().UTC()
		if _, err := RefreshItemSnapshots(ctx, tx, store, ref, in.Version); err != nil {
			return err
		}
		if _, err := RefreshItemPrices(ctx, tx, store, ref, in.Version, at); err != nil {
			return err
		}
		if _, err := ReplaceManualShippingMethods(ctx, tx, store, ref, in.Version, []ManualShippingMethodInput{{OptionID: in.OptionID}}, manualProviders, at); err != nil {
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
		if tx.Exec("INSERT INTO cart_shipping_command_operations(cart_id,operation_id,generation,request_sha256,result_version,created_at) VALUES(?,?,?,UNHEX(?),?,UTC_TIMESTAMP(6))", current.ID, in.OperationID, ref.Generation, hex.EncodeToString(digest[:]), version+1).Error != nil {
			return ErrStorage
		}
		result = ShippingResult{current.ID, strconv.FormatUint(version+1, 10), in.OptionID, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		if errors.Is(err, ErrItemUnavailable) || errors.Is(err, ErrItemStock) {
			return ShippingResult{}, err
		}
		return ShippingResult{}, sanitize(err)
	}
	return result, nil
}

func supportedSavedShipping(tx *gorm.DB, cartID string, manualProviders []string) error {
	// Do not silently discard a live saved method for a provider whose
	// implementation this command cannot execute.
	var providers []struct {
		Provider  *string
		PriceType string
	}
	if tx.Table("cart_shipping_methods m").Select("o.provider_id AS provider,o.price_type").Joins("JOIN fulfillment_shipping_options o ON o.id=m.shipping_option_id AND o.deleted_at IS NULL").Where("m.cart_id=? AND m.deleted_at IS NULL", cartID).Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&providers).Error != nil || len(providers) > 100 {
		return ErrSchema
	}
	for _, provider := range providers {
		if provider.PriceType != "flat" || provider.Provider == nil {
			return ErrConfiguration
		}
		registered := false
		for _, id := range manualProviders {
			registered = registered || id == *provider.Provider
		}
		if !registered {
			return ErrConfiguration
		}
	}
	return nil
}
