package orders

import (
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AmountLine struct {
	ID        string            `json:"id"`
	ItemID    string            `json:"item_id"`
	UnitPrice string            `json:"unit_price"`
	Quantity  string            `json:"quantity"`
	Totals    map[string]string `json:"totals"`
}
type AmountShipping struct {
	ID               string            `json:"id"`
	ShippingMethodID string            `json:"shipping_method_id"`
	Name             string            `json:"name"`
	Amount           string            `json:"amount"`
	IsTaxInclusive   bool              `json:"is_tax_inclusive"`
	Totals           map[string]string `json:"totals"`
}
type AmountSummary struct {
	TransactionTotal  string `json:"transaction_total"`
	PendingDifference string `json:"pending_difference"`
}
type AmountDetail struct {
	OrderID         string            `json:"order_id"`
	Version         int               `json:"version"`
	CurrencyCode    string            `json:"currency_code"`
	Totals          map[string]string `json:"totals"`
	Lines           []AmountLine      `json:"lines"`
	ShippingMethods []AmountShipping  `json:"shipping_methods"`
	Summary         *AmountSummary    `json:"summary"`
}
type amountItem struct {
	ID, ItemID, UnitPrice, Quantity                                                  string
	RawUnitPrice                                                                     *string
	RawQuantity                                                                      string
	IsTaxInclusive                                                                   bool
	Fulfilled, Shipped, Requested, Received, Dismissed, WrittenOff                   string
	RawFulfilled, RawShipped, RawRequested, RawReceived, RawDismissed, RawWrittenOff string
}
type amountShipping struct {
	ID, MethodID, Name, Amount, RawAmount string
	IsTaxInclusive                        bool
}
type amountTax struct{ ParentID, Rate, RawRate string }
type amountAdjustment struct {
	ParentID, Amount, RawAmount string
	IsTaxInclusive              bool
}

// Amounts reads a complete current-version financial snapshot in one read-only
// transaction. It neither recomputes prices from the catalog nor confirms paid
// status from order state, transactions, summary JSON or pending_difference.
func (h Handler) Amounts(c *gin.Context) {
	id := c.Param("id")
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 || strings.ContainsRune(id, 0) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order request")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "orders.unavailable", "Orders unavailable")
		return
	}
	var result AmountDetail
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = ReadAmountSnapshot(tx, id)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "orders.not_found", "Order not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "orders.amounts_unavailable", "Order amounts unavailable")
		return
	}
	c.JSON(200, result)
}

// ReadAmountSnapshot is an internal financial projection in the caller-owned
// transaction. The caller must establish order access; it grants no ownership.
// Reused by the HTTP reader and checkout copy verification to avoid two totals
// implementations. It does not refresh catalog prices or classify cash.
func ReadAmountSnapshot(tx *gorm.DB, id string) (AmountDetail, error) {
	if tx == nil || tx.Statement == nil {
		return AmountDetail{}, ErrSchema
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return AmountDetail{}, ErrSchema
	}
	if err := Ready(tx.Statement.Context, tx); err != nil {
		return AmountDetail{}, err
	}
	var order Summary
	if err := tx.Table("orders o").Select(summaryColumns()).Where("o.id=? AND o.deleted_at IS NULL", id).Take(&order).Error; err != nil {
		return AmountDetail{}, err
	}
	var currency struct{ DecimalDigits int }
	if tx.Table("commerce_currencies").Select("decimal_digits").Where("code=? AND deleted_at IS NULL", order.CurrencyCode).Take(&currency).Error != nil {
		return AmountDetail{}, ErrSchema
	}
	input := totals.Input{CurrencyDecimals: &currency.DecimalDigits}
	var items []amountItem
	if order.LineCount > 1000 {
		return AmountDetail{}, ErrSchema
	}
	if err := tx.Table("order_items i").Joins("JOIN order_line_items l ON l.id=i.item_id AND l.deleted_at IS NULL").Where("i.order_id=? AND i.version=? AND i.deleted_at IS NULL", id, order.Version).
		Select(`i.id,i.item_id,CAST(COALESCE(i.unit_price,l.unit_price) AS CHAR) AS unit_price,COALESCE(i.raw_unit_price,l.raw_unit_price) AS raw_unit_price,CAST(i.quantity AS CHAR) AS quantity,i.raw_quantity,l.is_tax_inclusive,CAST(i.fulfilled_quantity AS CHAR) AS fulfilled,i.raw_fulfilled_quantity AS raw_fulfilled,CAST(i.shipped_quantity AS CHAR) AS shipped,i.raw_shipped_quantity AS raw_shipped,CAST(i.return_requested_quantity AS CHAR) AS requested,i.raw_return_requested_quantity AS raw_requested,CAST(i.return_received_quantity AS CHAR) AS received,i.raw_return_received_quantity AS raw_received,CAST(i.return_dismissed_quantity AS CHAR) AS dismissed,i.raw_return_dismissed_quantity AS raw_dismissed,CAST(i.written_off_quantity AS CHAR) AS written_off,i.raw_written_off_quantity AS raw_written_off`).Order("i.id ASC").Limit(1001).Scan(&items).Error; err != nil {
		return AmountDetail{}, err
	}
	if int64(len(items)) != order.LineCount {
		return AmountDetail{}, ErrSchema
	}
	result := AmountDetail{OrderID: id, Version: order.Version, CurrencyCode: order.CurrencyCode, Lines: make([]AmountLine, 0, len(items)), ShippingMethods: []AmountShipping{}}
	itemIDs := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, row := range items {
		if seen[row.ItemID] {
			return AmountDetail{}, ErrSchema
		}
		seen[row.ItemID] = true
		itemIDs = append(itemIDs, row.ItemID)
		price, err := amountNumber(row.UnitPrice, row.RawUnitPrice)
		if err != nil {
			return AmountDetail{}, err
		}
		quantity, err := amountNumber(row.Quantity, &row.RawQuantity)
		if err != nil {
			return AmountDetail{}, err
		}
		item := totals.Item{ID: row.ID, UnitPrice: price, Quantity: quantity, IsTaxInclusive: &row.IsTaxInclusive}
		for _, p := range []struct {
			value, raw string
			target     **totals.Number
		}{
			{row.Fulfilled, row.RawFulfilled, &item.Detail.Fulfilled}, {row.Shipped, row.RawShipped, &item.Detail.Shipped},
			{row.Requested, row.RawRequested, &item.Detail.ReturnRequested}, {row.Received, row.RawReceived, &item.Detail.ReturnReceived},
			{row.Dismissed, row.RawDismissed, &item.Detail.ReturnDismissed}, {row.WrittenOff, row.RawWrittenOff, &item.Detail.WrittenOff},
		} {
			n, err := amountNumber(p.value, &p.raw)
			if err != nil {
				return AmountDetail{}, err
			}
			*p.target = &n
		}
		input.Items = append(input.Items, item)
		unit, _ := totals.Resolve(price)
		qty, _ := totals.Resolve(quantity)
		result.Lines = append(result.Lines, AmountLine{ID: row.ID, ItemID: row.ItemID, UnitPrice: unit, Quantity: qty})
	}
	if err := loadItemAmounts(tx, &input, itemIDs, order.Version); err != nil {
		return AmountDetail{}, err
	}
	var shippingCount int64
	if tx.Table("order_shipping_versions").Where("order_id=? AND version=? AND deleted_at IS NULL", id, order.Version).Count(&shippingCount).Error != nil || shippingCount > 100 {
		return AmountDetail{}, ErrSchema
	}
	var shipping []amountShipping
	if err := tx.Table("order_shipping_versions v").Joins("JOIN order_shipping_methods m ON m.id=v.shipping_method_id AND m.deleted_at IS NULL").Where("v.order_id=? AND v.version=? AND v.deleted_at IS NULL", id, order.Version).
		Select("v.id,m.id AS method_id,m.name,CAST(m.amount AS CHAR) AS amount,m.raw_amount,m.is_tax_inclusive").Order("v.id ASC").Limit(101).Scan(&shipping).Error; err != nil {
		return AmountDetail{}, err
	}
	if int64(len(shipping)) != shippingCount {
		return AmountDetail{}, ErrSchema
	}
	methodIDs := make([]string, 0, len(shipping))
	seen = map[string]bool{}
	for _, row := range shipping {
		if seen[row.MethodID] {
			return AmountDetail{}, ErrSchema
		}
		seen[row.MethodID] = true
		methodIDs = append(methodIDs, row.MethodID)
		n, err := amountNumber(row.Amount, &row.RawAmount)
		if err != nil {
			return AmountDetail{}, err
		}
		input.ShippingMethods = append(input.ShippingMethods, totals.Shipping{ID: row.ID, Amount: n, IsTaxInclusive: &row.IsTaxInclusive})
		effective, _ := totals.Resolve(n)
		result.ShippingMethods = append(result.ShippingMethods, AmountShipping{ID: row.ID, ShippingMethodID: row.MethodID, Name: row.Name, Amount: effective, IsTaxInclusive: row.IsTaxInclusive})
	}
	if err := loadShippingAmounts(tx, &input, methodIDs, order.Version); err != nil {
		return AmountDetail{}, err
	}
	var credits []struct{ Amount, RawAmount string }
	if tx.Table("order_credit_lines").Select("CAST(amount AS CHAR) AS amount,raw_amount").Where("order_id=? AND version=? AND deleted_at IS NULL", id, order.Version).Order("id ASC").Limit(1001).Scan(&credits).Error != nil || len(credits) > 1000 {
		return AmountDetail{}, ErrSchema
	}
	for _, row := range credits {
		n, err := amountNumber(row.Amount, &row.RawAmount)
		if err != nil {
			return AmountDetail{}, err
		}
		input.CreditLines = append(input.CreditLines, totals.Credit{Amount: &n})
	}
	var summaries []struct{ Totals *string }
	if tx.Table("order_summaries").Select("totals").Where("order_id=? AND version=? AND deleted_at IS NULL", id, order.Version).Limit(2).Scan(&summaries).Error != nil || len(summaries) > 1 {
		return AmountDetail{}, ErrSchema
	}
	var transaction *string
	if len(summaries) == 1 {
		var err error
		input.Summary, transaction, err = transactionSummary(summaries[0].Totals)
		if err != nil {
			return AmountDetail{}, err
		}
	}
	calculated, err := totals.Calculate(input)
	if err != nil {
		return AmountDetail{}, ErrSchema
	}
	result.Totals = calculated.Totals
	for i, row := range calculated.Items {
		result.Lines[i].Totals = row.Totals
	}
	for i, row := range calculated.ShippingMethods {
		result.ShippingMethods[i].Totals = row.Totals
	}
	if calculated.Summary != nil {
		result.Summary = &AmountSummary{TransactionTotal: *transaction, PendingDifference: calculated.Summary.PendingDifference}
	}
	return result, nil
}

func loadItemAmounts(tx *gorm.DB, input *totals.Input, ids []string, version int) error {
	if len(ids) == 0 {
		return nil
	}
	tax, adj, err := loadAmountRelations(tx, "order_line_tax_lines", "order_line_adjustments", "item_id", ids, version, true)
	if err != nil {
		return err
	}
	for i, id := range ids {
		input.Items[i].TaxLines = tax[id]
		input.Items[i].Adjustments = adj[id]
	}
	return nil
}
func loadShippingAmounts(tx *gorm.DB, input *totals.Input, ids []string, version int) error {
	if len(ids) == 0 {
		return nil
	}
	tax, adj, err := loadAmountRelations(tx, "order_shipping_tax_lines", "order_shipping_adjustments", "shipping_method_id", ids, version, false)
	if err != nil {
		return err
	}
	for i, id := range ids {
		input.ShippingMethods[i].TaxLines = tax[id]
		input.ShippingMethods[i].Adjustments = adj[id]
	}
	return nil
}
func loadAmountRelations(tx *gorm.DB, taxTable, adjustmentTable, column string, ids []string, version int, hasInclusive bool) (map[string][]totals.TaxLine, map[string][]totals.Adjustment, error) {
	// Table/column identifiers are compile-time constants from the two callers.
	var taxRows []amountTax
	if tx.Table(taxTable).Select(column+" AS parent_id,CAST(rate AS CHAR) AS rate,raw_rate").Where(column+" IN ? AND deleted_at IS NULL", ids).Order("id ASC").Limit(len(ids)*100+1).Scan(&taxRows).Error != nil {
		return nil, nil, ErrSchema
	}
	tax := map[string][]totals.TaxLine{}
	for _, row := range taxRows {
		if len(tax[row.ParentID]) >= 100 {
			return nil, nil, ErrSchema
		}
		n, err := amountNumber(row.Rate, &row.RawRate)
		if err != nil {
			return nil, nil, err
		}
		tax[row.ParentID] = append(tax[row.ParentID], totals.TaxLine{Rate: n})
	}
	var adjustmentRows []amountAdjustment
	projection := column + " AS parent_id,CAST(amount AS CHAR) AS amount,raw_amount"
	// The installed legacy shipping adjustment model has no inclusive flag;
	// its calculator treats that missing flag as false, independently of shipping.
	if hasInclusive {
		projection += ",is_tax_inclusive"
	}
	if tx.Table(adjustmentTable).Select(projection).Where(column+" IN ? AND version=? AND deleted_at IS NULL", ids, version).Order("id ASC").Limit(len(ids)*100+1).Scan(&adjustmentRows).Error != nil {
		return nil, nil, ErrSchema
	}
	adj := map[string][]totals.Adjustment{}
	for _, row := range adjustmentRows {
		if len(adj[row.ParentID]) >= 100 {
			return nil, nil, ErrSchema
		}
		n, err := amountNumber(row.Amount, &row.RawAmount)
		if err != nil {
			return nil, nil, err
		}
		adj[row.ParentID] = append(adj[row.ParentID], totals.Adjustment{Amount: &n, IsTaxInclusive: row.IsTaxInclusive})
	}
	return tax, adj, nil
}
