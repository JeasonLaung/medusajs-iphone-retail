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
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PromotionCommand struct {
	OperationID string   `json:"-"`
	Version     string   `json:"version"`
	Codes       []string `json:"codes"`
}
type PromotionResult struct {
	CartID   string `json:"cart_id"`
	Version  string `json:"version"`
	Replayed bool   `json:"replayed"`
}
type AppliedPromotion struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Automatic bool   `json:"is_automatic"`
}
type PromotionDetail struct {
	CartID     string             `json:"cart_id"`
	Version    string             `json:"version"`
	Promotions []AppliedPromotion `json:"promotions"`
}

func validPromotionCode(code string) bool {
	return code != "" && len(code) <= 191 && utf8.ValidString(code) && !strings.ContainsFunc(code, func(r rune) bool { return r < 32 || r == 127 })
}
func validatePromotion(in PromotionCommand) (uint64, error) {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID || e != nil || version == 0 || version == ^uint64(0) || strconv.FormatUint(version, 10) != in.Version || in.Codes == nil || len(in.Codes) > 100 {
		return 0, ErrInput
	}
	for _, code := range in.Codes {
		if !validPromotionCode(code) {
			return 0, ErrInput
		}
	}
	return version, nil
}

// ReadAppliedPromotions returns only saved cart relationships, in the same
// authorized snapshot as their version. It neither applies nor claims a code.
func ReadAppliedPromotions(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference) (PromotionDetail, error) {
	var result PromotionDetail
	err := readCartSnapshot(ctx, db, store, ref, func(tx *gorm.DB, current Snapshot) error {
		rows := []AppliedPromotion{}
		if tx.Table("cart_promotions c").Select("p.id,p.code,p.is_automatic AS automatic").Joins("JOIN promotions p ON p.id=c.promotion_id AND p.deleted_at IS NULL").Where("c.cart_id=? AND c.deleted_at IS NULL", current.ID).Order("c.created_at,c.id").Limit(101).Scan(&rows).Error != nil || len(rows) > 100 {
			return ErrSchema
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if !idPattern.MatchString(row.ID) || !validPromotionCode(row.Code) || seen[row.ID] {
				return ErrSchema
			}
			seen[row.ID] = true
		}
		result = PromotionDetail{current.ID, current.Version, rows}
		return nil
	})
	if err != nil {
		return PromotionDetail{}, err
	}
	return result, nil
}

// SavePromotions implements the original replacement-code operation, including
// automatic rules. All catalog prices, saved shipping, taxes and discounts are
// refreshed before totals, version and receipt commit. Provider-attached carts
// remain immutable until their explicit payment invalidation is coordinated.
func SavePromotions(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in PromotionCommand, manualProviders []string) (PromotionResult, error) {
	version, err := validatePromotion(in)
	if err != nil || !refOK(store, ref) || db == nil || db.Statement == nil {
		return PromotionResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return PromotionResult{}, ErrInput
	}
	if len(manualProviders) > 100 {
		return PromotionResult{}, ErrInput
	}
	for _, id := range manualProviders {
		if !idPattern.MatchString(id) {
			return PromotionResult{}, ErrInput
		}
	}
	request, _ := json.Marshal(in)
	digest := sha256.Sum256(request)
	var result PromotionResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_promotion_command_runtime WHERE id=1 FOR SHARE").Scan(&gate)
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
		r = tx.Raw("SELECT generation,HEX(request_sha256) AS hash,result_version FROM cart_promotion_command_operations WHERE cart_id=? AND operation_id=? FOR SHARE", current.ID, in.OperationID).Scan(&old)
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
			result = PromotionResult{current.ID, strconv.FormatUint(old.ResultVersion, 10), true}
			return nil
		}
		if current.Version != in.Version {
			return ErrConflict
		}
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
		if _, err := RefreshManualShippingMethods(ctx, tx, store, ref, in.Version, manualProviders, at); err != nil {
			return err
		}
		if _, err := RefreshSystemTaxes(ctx, tx, store, ref, in.Version, true); err != nil {
			return err
		}
		if _, err := RefreshPromotions(ctx, tx, store, ref, in.Version, PromotionRefresh{At: at, Codes: in.Codes, Action: "replace"}); err != nil {
			return err
		}
		if _, err := assembleAmounts(tx, current); err != nil {
			return err
		}
		if r := tx.Exec("UPDATE cart_ownership SET version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", current.ID, version); r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		if tx.Exec("INSERT INTO cart_promotion_command_operations(cart_id,operation_id,generation,request_sha256,result_version,created_at) VALUES(?,?,?,UNHEX(?),?,UTC_TIMESTAMP(6))", current.ID, in.OperationID, ref.Generation, hex.EncodeToString(digest[:]), version+1).Error != nil {
			return ErrStorage
		}
		result = PromotionResult{current.ID, strconv.FormatUint(version+1, 10), false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		if errors.Is(err, ErrItemUnavailable) || errors.Is(err, ErrItemStock) {
			return PromotionResult{}, err
		}
		return PromotionResult{}, sanitize(err)
	}
	return result, nil
}
