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

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrItemUnavailable = errors.New("cart variant or price unavailable")
var ErrItemStock = errors.New("cart stock unavailable")

type AddItemCommand struct{ OperationID, Version, VariantID, Quantity string }
type AddItemResult struct {
	CartID   string `json:"cart_id"`
	Version  string `json:"version"`
	LineID   string `json:"line_id"`
	Quantity string `json:"quantity"`
	Replayed bool   `json:"replayed"`
}

// sqlAmount retains the exact effective value in raw, with an explicitly
// truncated scale-18 compatibility scalar. MySQL never chooses how to round it.
func sqlAmount(value string) (string, string, error) {
	canonical, err := totals.Resolve(totals.Number{Decimal: value})
	if err != nil || canonical != value || strings.HasPrefix(value, "-") {
		return "", "", ErrItemUnavailable
	}
	parts := strings.Split(value, ".")
	if len(parts[0]) > 20 {
		return "", "", ErrItemUnavailable
	}
	scalar := parts[0]
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 18 {
			fraction = fraction[:18]
		}
		scalar += "." + fraction
	}
	raw, err := json.Marshal(totals.RawValue{Value: value, Precision: 100})
	if err != nil {
		return "", "", ErrItemUnavailable
	}
	return scalar, string(raw), nil
}

// AddItem owns one atomic draft mutation. Resolve possession and CSRF before
// calling; store/ref are trusted server context. No inventory reservation,
// payment, event, worker or provider call occurs here. Tax/promo/shipping rule
// selection and payable checkout remain separate, unaccepted capabilities.
func AddItem(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in AddItemCommand) (AddItemResult, error) {
	return mutateItem(ctx, db, store, ref, in, "add", "")
}

// SetItem replaces the absolute quantity; zero is an explicit removal. The
// immutable receipt namespace is shared with AddItem, with a distinct digest.
func SetItem(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, lineID string, in AddItemCommand) (AddItemResult, error) {
	if !idPattern.MatchString(lineID) || in.VariantID != "" {
		return AddItemResult{}, ErrInput
	}
	return mutateItem(ctx, db, store, ref, in, "set", lineID)
}

func mutateItem(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, in AddItemCommand, mode, target string) (AddItemResult, error) {
	id, err := uuid.Parse(in.OperationID)
	version, e := strconv.ParseUint(in.Version, 10, 64)
	quantity, f := strconv.ParseInt(in.Quantity, 10, 64)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID || e != nil || version == 0 || strconv.FormatUint(version, 10) != in.Version || f != nil || (quantity < 1 && !(mode == "set" && quantity == 0)) || quantity > 1000000000 || strconv.FormatInt(quantity, 10) != in.Quantity || (mode == "add" && !idPattern.MatchString(in.VariantID)) || !refOK(store, ref) || db == nil || db.Statement == nil {
		return AddItemResult{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return AddItemResult{}, ErrInput
	}
	request, _ := json.Marshal(in)
	if mode == "set" {
		request, _ = json.Marshal(struct {
			Mode, LineID string
			Command      AddItemCommand
		}{mode, target, in})
	}
	digest := sha256.Sum256(request)
	var result AddItemResult
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		var cap struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_item_runtime WHERE id=1 FOR SHARE").Scan(&cap)
		if r.Error != nil || r.RowsAffected != 1 || cap.Version != 1 || !cap.Enabled {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		cart, err := lockCurrent(ctx, tx, store, channel, ref, "UPDATE")
		if err != nil {
			return err
		}
		var old struct{ Generation, RequestHash, ResultJSON string }
		r = tx.Raw("SELECT generation,HEX(request_sha256) AS request_hash,result_json FROM cart_item_operations WHERE cart_id=? AND operation_id=? FOR SHARE", ref.CartID, in.OperationID).Scan(&old)
		if r.Error != nil {
			return ErrStorage
		}
		if r.RowsAffected == 1 {
			if old.Generation != ref.Generation || subtle.ConstantTimeCompare([]byte(strings.ToLower(old.RequestHash)), []byte(hex.EncodeToString(digest[:]))) != 1 {
				return ErrConflict
			}
			if json.Unmarshal([]byte(old.ResultJSON), &result) != nil {
				return ErrStorage
			}
			savedVersion, ve := strconv.ParseUint(result.Version, 10, 64)
			currentVersion, ce := strconv.ParseUint(cart.Version, 10, 64)
			savedQuantity, qe := strconv.ParseInt(result.Quantity, 10, 64)
			if result.CartID != ref.CartID || !idPattern.MatchString(result.LineID) || ve != nil || ce != nil || savedVersion < 2 || savedVersion > currentVersion || strconv.FormatUint(savedVersion, 10) != result.Version || qe != nil || (savedQuantity < 1 && !(mode == "set" && savedQuantity == 0)) || savedQuantity > 1000000000 || strconv.FormatInt(savedQuantity, 10) != result.Quantity || result.Replayed {
				return ErrStorage
			}
			if mode == "set" && (result.LineID != target || result.Quantity != in.Quantity) {
				return ErrStorage
			}
			result.Replayed = true
			return nil
		}
		if cart.Version != in.Version || version == ^uint64(0) {
			return ErrConflict
		}
		var lines []struct {
			ID            string
			VariantID     *string
			Quantity      int64
			IsCustomPrice bool
		}
		if tx.Table("cart_line_items").Select("id,variant_id,quantity,is_custom_price").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Limit(1001).Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&lines).Error != nil || len(lines) > 1000 {
			return ErrSchema
		}
		if mode == "set" {
			found := false
			for _, line := range lines {
				if line.ID == target {
					found = true
					if quantity > 0 {
						if line.VariantID == nil || line.IsCustomPrice {
							return ErrItemUnavailable
						}
						in.VariantID = *line.VariantID
					}
				}
			}
			if !found {
				return ErrDenied
			}
		}
		lineID := ""
		total := quantity
		ids := []string{}
		seen := map[string]bool{}
		stockLines := make([]inventory.CartStockLine, 0, len(lines)+1)
		for _, line := range lines {
			if mode == "set" && quantity == 0 {
				if line.ID == target {
					lineID = target
				}
				continue
			}
			if line.VariantID == nil || line.IsCustomPrice || line.Quantity < 1 || line.Quantity > 1000000000 || !idPattern.MatchString(line.ID) {
				return ErrItemUnavailable
			}
			v := *line.VariantID
			if !seen[v] {
				ids = append(ids, v)
				seen[v] = true
			}
			q := line.Quantity
			if (mode == "add" && v == in.VariantID) || (mode == "set" && line.ID == target) {
				if lineID != "" {
					return ErrConflict
				}
				lineID = line.ID
				if mode == "add" {
					total += q
				}
				if total > 1000000000 {
					return ErrInput
				}
				q = total
			}
			stockLines = append(stockLines, inventory.CartStockLine{ID: line.ID, VariantID: v, Quantity: strconv.FormatInt(q, 10)})
		}
		fresh := lineID == ""
		if fresh {
			if len(lines) == 1000 {
				return ErrInput
			}
			lineID = "cali_" + uuid.NewString()
			stockLines = append(stockLines, inventory.CartStockLine{ID: lineID, VariantID: in.VariantID, Quantity: in.Quantity})
		}
		if !(mode == "set" && quantity == 0) && !seen[in.VariantID] {
			ids = append(ids, in.VariantID)
		}
		variants, err := catalog.ReadCartVariants(tx, ids, channel)
		if err != nil {
			return ErrItemUnavailable
		}
		available := make([]inventory.AvailableVariant, 0, len(ids))
		for _, id := range ids {
			v := variants[id]
			available = append(available, inventory.AvailableVariant{ID: id, ManageInventory: v.ManageInventory, AllowBackorder: v.AllowBackorder, Complete: v.Complete})
		}
		stock, err := inventory.ReadCartStock(tx, available, stockLines, channel)
		if err != nil {
			return ErrStorage
		}
		if !stock.CanFulfill {
			return ErrItemStock
		}
		evidencePrice, evidenceSet := "", ""
		if mode == "set" && quantity == 0 {
			r := tx.Exec("UPDATE cart_line_items SET deleted_at=UTC_TIMESTAMP(6),updated_at=UTC_TIMESTAMP(6) WHERE id=? AND cart_id=? AND deleted_at IS NULL", target, cart.ID)
			if r.Error != nil || r.RowsAffected != 1 {
				return ErrStorage
			}
			// Preserve saved tax/adjustment history. The removed parent is
			// excluded from amount reads; no orphan relation is reattached.
		} else {
			quotes, err := pricing.Read(tx, []string{in.VariantID}, pricing.Context{Region: cart.RegionID, Currency: cart.CurrencyCode, Channel: channel, Quantity: strconv.FormatInt(total, 10)}, time.Now().UTC())
			if err != nil || quotes[in.VariantID] == nil {
				return ErrItemUnavailable
			}
			quote := quotes[in.VariantID]
			scalar, raw, err := sqlAmount(quote.Amount)
			if err != nil {
				return err
			}
			var compareScalar, compareRaw *string
			if quote.CalculatedKind.Type != nil && *quote.CalculatedKind.Type == "sale" && quote.OriginalAmount != nil && *quote.OriginalAmount != quote.Amount {
				a, b, err := sqlAmount(*quote.OriginalAmount)
				if err != nil {
					return err
				}
				compareScalar, compareRaw = &a, &b
			}
			evidencePrice, evidenceSet = quote.PriceID, quote.SetID
			v := variants[in.VariantID]
			if fresh {
				if tx.Exec(`INSERT INTO cart_line_items(id,cart_id,title,subtitle,thumbnail,quantity,variant_id,product_id,product_title,product_description,product_subtitle,product_type,product_collection,product_handle,variant_sku,variant_barcode,variant_title,variant_option_values,requires_shipping,is_discountable,is_tax_inclusive,compare_at_unit_price,raw_compare_at_unit_price,unit_price,raw_unit_price,metadata,created_at,updated_at,product_type_id,is_custom_price,is_giftcard)
    VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'{}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),?,FALSE,?)`, lineID, cart.ID, v.ProductTitle, v.Title, v.Thumbnail, total, v.ID, v.ProductID, v.ProductTitle, v.ProductDescription, v.ProductSubtitle, v.ProductType, v.ProductCollection, v.ProductHandle, v.SKU, v.Barcode, v.Title, v.OptionValues, v.RequiresShipping, v.Discountable, quote.CalculatedTaxInclusive, compareScalar, compareRaw, scalar, raw, v.ProductTypeID, v.IsGiftcard).Error != nil {
					return ErrStorage
				}
			} else {
				r := tx.Exec(`UPDATE cart_line_items SET quantity=?,unit_price=?,raw_unit_price=?,is_tax_inclusive=?,compare_at_unit_price=?,raw_compare_at_unit_price=?,updated_at=UTC_TIMESTAMP(6) WHERE id=? AND cart_id=? AND deleted_at IS NULL`, total, scalar, raw, quote.CalculatedTaxInclusive, compareScalar, compareRaw, lineID, cart.ID)
				if r.Error != nil || r.RowsAffected != 1 {
					return ErrStorage
				}
			}
		}
		next := strconv.FormatUint(version+1, 10)
		if r := tx.Exec("UPDATE cart_ownership SET version=version+1,updated_at=UTC_TIMESTAMP(6) WHERE cart_id=? AND version=?", cart.ID, version); r.Error != nil || r.RowsAffected != 1 {
			return ErrConflict
		}
		if tx.Exec("UPDATE carts SET updated_at=UTC_TIMESTAMP(6) WHERE id=?", cart.ID).Error != nil {
			return ErrStorage
		}
		result = AddItemResult{CartID: cart.ID, Version: next, LineID: lineID, Quantity: strconv.FormatInt(total, 10)}
		encoded, err := json.Marshal(result)
		if err != nil {
			return ErrStorage
		}
		if tx.Exec("INSERT INTO cart_item_operations(cart_id,operation_id,generation,request_sha256,price_id,price_set_id,result_json,created_at) VALUES(?,?,?,UNHEX(?),?,?,?,UTC_TIMESTAMP(6))", cart.ID, in.OperationID, ref.Generation, hex.EncodeToString(digest[:]), evidencePrice, evidenceSet, string(encoded)).Error != nil {
			return ErrStorage
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		if errors.Is(err, ErrItemUnavailable) || errors.Is(err, ErrItemStock) {
			return AddItemResult{}, err
		}
		return AddItemResult{}, sanitize(err)
	}
	return result, nil
}
