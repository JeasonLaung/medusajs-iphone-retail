package cart

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
)

type AmountLine struct {
	ID             string            `json:"id"`
	VariantID      *string           `json:"variant_id"`
	UnitPrice      string            `json:"unit_price"`
	Quantity       string            `json:"quantity"`
	IsTaxInclusive bool              `json:"is_tax_inclusive"`
	Totals         map[string]string `json:"totals"`
}
type AmountShipping struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Amount         string            `json:"amount"`
	IsTaxInclusive bool              `json:"is_tax_inclusive"`
	Totals         map[string]string `json:"totals"`
}

// AmountDetail is the exact persisted snapshot, not a newly selected quote or a
// payable authorization. Current tax/shipping/promotion selection is separate.
type AmountDetail struct {
	CartID              string            `json:"cart_id"`
	Version             string            `json:"version"`
	CurrencyCode        string            `json:"currency_code"`
	Totals              map[string]string `json:"totals"`
	Lines               []AmountLine      `json:"lines"`
	ShippingMethods     []AmountShipping  `json:"shipping_methods"`
	PricingRecalculated bool              `json:"pricing_recalculated"`
	Checkout            bool              `json:"checkout"`
}

// ReadAmounts authenticates durable ownership and assembles all relations in a
// single snapshot. Callers must resolve the possession cookie with Redis first.
// Historical source IDs/customer references do not confer this authority.
func ReadAmounts(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference) (AmountDetail, error) {
	var result AmountDetail
	err := readCartSnapshot(ctx, db, store, ref, func(tx *gorm.DB, cart Snapshot) error {
		var err error
		result, err = assembleAmounts(tx, cart)
		return err
	})
	if err != nil {
		return AmountDetail{}, err
	}
	return result, nil
}

func readCartSnapshot(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, read func(*gorm.DB, Snapshot) error) error {
	if db == nil || db.Statement == nil || !refOK(store, ref) {
		return ErrDenied
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return ErrInput
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		cart, err := lockCurrent(ctx, tx, store, channel, ref, "SHARE")
		if err != nil {
			return err
		}
		return read(tx, cart)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return sanitize(err)
	}
	return nil
}

// The caller owns the transaction and current cart authorization. All SQL
// identifiers below are compile-time constants, never chosen from HTTP data.
func assembleAmounts(tx *gorm.DB, cart Snapshot) (AmountDetail, error) {
	return assembleAmountPhase(tx, cart, true)
}

// ReadPromotionAmounts computes the undiscounted monetary facts for the next
// promotion refresh, using the currently saved prices and taxes. It rechecks
// ownership/version in the caller's write transaction, but neither refreshes
// prices/taxes nor commits. Call after those phases and roll back on any error.
// This is internal input preparation, not a public quote or checkout command.
func ReadPromotionAmounts(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string) (AmountDetail, error) {
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return AmountDetail{}, err
	}
	return assembleAmountPhase(tx, cart, false)
}

func assembleAmountPhase(tx *gorm.DB, cart Snapshot, includeAdjustments bool) (AmountDetail, error) {
	var currency struct{ DecimalDigits int }
	if tx.Table("commerce_currencies").Select("decimal_digits").Where("code=? AND deleted_at IS NULL", cart.CurrencyCode).Take(&currency).Error != nil {
		return AmountDetail{}, ErrSchema
	}
	var region struct{ AutomaticTaxes bool }
	if tx.Table("commerce_regions").Select("automatic_taxes").Where("id=? AND deleted_at IS NULL", cart.RegionID).Take(&region).Error != nil {
		return AmountDetail{}, ErrSchema
	}
	input := totals.Input{CurrencyDecimals: &currency.DecimalDigits, AutomaticTaxes: region.AutomaticTaxes}
	result := AmountDetail{CartID: cart.ID, Version: cart.Version, CurrencyCode: cart.CurrencyCode, Lines: []AmountLine{}, ShippingMethods: []AmountShipping{}}
	var lines []struct {
		ID, UnitPrice, RawUnitPrice string
		VariantID                   *string
		Quantity                    int64
		IsTaxInclusive              bool
	}
	if tx.Table("cart_line_items").Select("id,variant_id,quantity,CAST(unit_price AS CHAR) AS unit_price,raw_unit_price,is_tax_inclusive").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Limit(1001).Scan(&lines).Error != nil || len(lines) > 1000 {
		return AmountDetail{}, ErrSchema
	}
	ids := make([]string, 0, len(lines))
	for _, row := range lines {
		if !idPattern.MatchString(row.ID) || row.Quantity < 1 || row.Quantity > 1000000000 {
			return AmountDetail{}, ErrSchema
		}
		price, err := nonNegativeAmount(row.UnitPrice, &row.RawUnitPrice)
		if err != nil {
			return AmountDetail{}, ErrSchema
		}
		quantity := strconv.FormatInt(row.Quantity, 10)
		inclusive := row.IsTaxInclusive
		input.Items = append(input.Items, totals.Item{ID: row.ID, UnitPrice: price, Quantity: totals.Number{Decimal: quantity}, IsTaxInclusive: &inclusive})
		unit, _ := totals.Resolve(price)
		result.Lines = append(result.Lines, AmountLine{ID: row.ID, VariantID: row.VariantID, UnitPrice: unit, Quantity: quantity, IsTaxInclusive: inclusive})
		ids = append(ids, row.ID)
	}
	taxes, adjustments, err := amountPhaseRelations(tx, "cart_line_tax_lines", "cart_line_adjustments", "item_id", ids, true, includeAdjustments)
	if err != nil {
		return AmountDetail{}, err
	}
	for i, id := range ids {
		input.Items[i].TaxLines = taxes[id]
		input.Items[i].Adjustments = adjustments[id]
	}
	var shipping []struct {
		ID, Name, Amount, RawAmount string
		IsTaxInclusive              bool
	}
	if tx.Table("cart_shipping_methods").Select("id,name,CAST(amount AS CHAR) AS amount,raw_amount,is_tax_inclusive").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Limit(101).Scan(&shipping).Error != nil || len(shipping) > 100 {
		return AmountDetail{}, ErrSchema
	}
	ids = make([]string, 0, len(shipping))
	for _, row := range shipping {
		if !idPattern.MatchString(row.ID) {
			return AmountDetail{}, ErrSchema
		}
		amount, err := nonNegativeAmount(row.Amount, &row.RawAmount)
		if err != nil {
			return AmountDetail{}, ErrSchema
		}
		inclusive := row.IsTaxInclusive
		input.ShippingMethods = append(input.ShippingMethods, totals.Shipping{ID: row.ID, Amount: amount, IsTaxInclusive: &inclusive})
		effective, _ := totals.Resolve(amount)
		result.ShippingMethods = append(result.ShippingMethods, AmountShipping{ID: row.ID, Name: row.Name, Amount: effective, IsTaxInclusive: inclusive})
		ids = append(ids, row.ID)
	}
	taxes, adjustments, err = amountPhaseRelations(tx, "cart_shipping_tax_lines", "cart_shipping_adjustments", "shipping_method_id", ids, false, includeAdjustments)
	if err != nil {
		return AmountDetail{}, err
	}
	for i, id := range ids {
		input.ShippingMethods[i].TaxLines = taxes[id]
		input.ShippingMethods[i].Adjustments = adjustments[id]
	}
	calculated, err := totals.Calculate(input)
	if err != nil || len(calculated.Items) != len(lines) || len(calculated.ShippingMethods) != len(shipping) {
		return AmountDetail{}, ErrSchema
	}
	result.Totals = calculated.Totals
	for i := range result.Lines {
		result.Lines[i].Totals = calculated.Items[i].Totals
	}
	for i := range result.ShippingMethods {
		result.ShippingMethods[i].Totals = calculated.ShippingMethods[i].Totals
	}
	return result, nil
}

func amountPhaseRelations(tx *gorm.DB, taxTable, adjustmentTable, parent string, ids []string, inclusive, includeAdjustments bool) (map[string][]totals.TaxLine, map[string][]totals.Adjustment, error) {
	taxes, adjustments := map[string][]totals.TaxLine{}, map[string][]totals.Adjustment{}
	if len(ids) == 0 {
		return taxes, adjustments, nil
	}
	var taxRows []struct{ ParentID, Rate string }
	if tx.Table(taxTable).Select(parent+" AS parent_id,CAST(rate AS CHAR) AS rate").Where(parent+" IN ? AND deleted_at IS NULL", ids).Order("id").Limit(len(ids)*100+1).Scan(&taxRows).Error != nil || len(taxRows) > len(ids)*100 {
		return nil, nil, ErrSchema
	}
	for _, row := range taxRows {
		if len(taxes[row.ParentID]) >= 100 {
			return nil, nil, ErrSchema
		}
		taxes[row.ParentID] = append(taxes[row.ParentID], totals.TaxLine{Rate: totals.Number{Decimal: row.Rate}})
	}
	if !includeAdjustments {
		return taxes, adjustments, nil
	}
	var adjustmentRows []struct {
		ParentID, Amount, RawAmount string
		IsTaxInclusive              bool
	}
	projection := parent + " AS parent_id,CAST(amount AS CHAR) AS amount,raw_amount"
	if inclusive {
		projection += ",is_tax_inclusive"
	}
	if tx.Table(adjustmentTable).Select(projection).Where(parent+" IN ? AND deleted_at IS NULL", ids).Order("id").Limit(len(ids)*100+1).Scan(&adjustmentRows).Error != nil || len(adjustmentRows) > len(ids)*100 {
		return nil, nil, ErrSchema
	}
	for _, row := range adjustmentRows {
		if len(adjustments[row.ParentID]) >= 100 {
			return nil, nil, ErrSchema
		}
		n, err := nonNegativeAmount(row.Amount, &row.RawAmount)
		if err != nil {
			return nil, nil, ErrSchema
		}
		adjustments[row.ParentID] = append(adjustments[row.ParentID], totals.Adjustment{Amount: &n, IsTaxInclusive: row.IsTaxInclusive})
	}
	return taxes, adjustments, nil
}

// These cart source columns have nonnegative constraints; their effective raw
// value must honor the same constraint instead of bypassing it via JSON.
func nonNegativeAmount(scalar string, raw *string) (totals.Number, error) {
	n, err := totals.StoredNumber(scalar, raw)
	if err != nil {
		return totals.Number{}, ErrSchema
	}
	effective, err := totals.Resolve(n)
	if err != nil || strings.HasPrefix(effective, "-") {
		return totals.Number{}, ErrSchema
	}
	return n, nil
}
