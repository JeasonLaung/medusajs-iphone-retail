package orders

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// RefreshChangeSummaryTx projects current order amounts after the owning
// command's item/quantity/adjustment changes. It neither charges/refunds nor
// confirms payment. Caller authorizes, owns replay and rolls back the entire
// transaction on error. Transaction rows and historical versions remain unchanged; cached aggregates
// are refreshed from the actual ledger, never used to infer payment status.
func RefreshChangeSummaryTx(ctx context.Context, tx *gorm.DB, orderID string, version int) (AmountDetail, error) {
	if !checkoutID.MatchString(orderID) || version < 1 || numberTransaction(ctx, tx) != nil {
		return AmountDetail{}, ErrSchema
	}
	var order struct {
		Version      int
		Status       string
		IsDraftOrder bool
		CanceledAt   *time.Time
	}
	if tx.Table("orders").Select("version,status,is_draft_order,canceled_at").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error != nil || order.Version != version || order.IsDraftOrder || order.Status == "draft" || order.Status == "canceled" || order.CanceledAt != nil {
		return AmountDetail{}, ErrVersionConflict
	}
	var rows []struct {
		ID     string
		Totals *string
	}
	if tx.Table("order_summaries").Select("id,totals").Where("order_id=? AND version=? AND deleted_at IS NULL", orderID, version).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(2).Find(&rows).Error != nil || len(rows) != 1 || rows[0].Totals == nil {
		return AmountDetail{}, ErrSchema
	}
	obj, e := amountObject(*rows[0].Totals)
	if e != nil {
		return AmountDetail{}, e
	}
	// Recompute financial aggregates from immutable transaction rows, as the
	// original order-change calculator does. A cached summary is not payment proof.
	amounts, e := ReadAmountSnapshot(tx, orderID)
	if e != nil {
		return AmountDetail{}, e
	}
	var transactions []struct {
		Amount, RawAmount, CurrencyCode string
		Version                         int
	}
	if tx.Table("order_transactions").Select("CAST(amount AS CHAR) AS amount,raw_amount,currency_code,version").Where("order_id=? AND deleted_at IS NULL", orderID).Order("id").Limit(10001).Clauses(clause.Locking{Strength: "SHARE"}).Find(&transactions).Error != nil || len(transactions) > 10000 {
		return AmountDetail{}, ErrSchema
	}
	all, paid, refunded := []totals.Number{}, []totals.Number{}, []totals.Number{}
	for _, r := range transactions {
		if r.CurrencyCode != amounts.CurrencyCode || r.Version < 1 || r.Version > version {
			return AmountDetail{}, ErrSchema
		}
		n, e := amountNumber(r.Amount, &r.RawAmount)
		if e != nil {
			return AmountDetail{}, e
		}
		v, e := totals.Resolve(n)
		if e != nil {
			return AmountDetail{}, e
		}
		all = append(all, n)
		if strings.HasPrefix(v, "-") {
			refunded = append(refunded, totals.Number{Decimal: strings.TrimPrefix(v, "-")})
		} else {
			paid = append(paid, n)
		}
	}
	var credits []struct{ Amount, RawAmount string }
	if tx.Table("order_credit_lines").Select("CAST(amount AS CHAR) AS amount,raw_amount").Where("order_id=? AND version=? AND deleted_at IS NULL", orderID, version).Order("id").Limit(1001).Find(&credits).Error != nil || len(credits) > 1000 {
		return AmountDetail{}, ErrSchema
	}
	creditNumbers := []totals.Number{}
	for _, r := range credits {
		n, e := amountNumber(r.Amount, &r.RawAmount)
		if e != nil {
			return AmountDetail{}, e
		}
		creditNumbers = append(creditNumbers, n)
	}
	values := map[string]string{}
	for field, numbers := range map[string][]totals.Number{"transaction_total": all, "paid_total": paid, "refunded_total": refunded, "credit_line_total": creditNumbers} {
		v, e := totals.Sum(numbers...)
		if e != nil {
			return AmountDetail{}, e
		}
		values[field] = v
	}
	total, ok := amounts.Totals["total"]
	if !ok {
		return AmountDetail{}, ErrSchema
	}
	negative := "-" + values["transaction_total"]
	if strings.HasPrefix(values["transaction_total"], "-") {
		negative = strings.TrimPrefix(values["transaction_total"], "-")
	}
	pending, e := totals.Sum(totals.Number{Decimal: total}, totals.Number{Decimal: negative})
	if e != nil {
		return AmountDetail{}, e
	}
	values["current_order_total"], values["accounting_total"], values["pending_difference"] = total, total, pending
	amounts.Summary = &AmountSummary{TransactionTotal: values["transaction_total"], PendingDifference: pending}
	for field, value := range values {
		obj[field], e = json.Marshal(value)
		if e != nil {
			return AmountDetail{}, ErrSchema
		}
		obj["raw_"+field], e = json.Marshal(totals.RawValue{Value: value, Precision: 100})
		if e != nil {
			return AmountDetail{}, ErrSchema
		}
	}
	wire, e := json.Marshal(obj)
	if e != nil {
		return AmountDetail{}, ErrSchema
	}
	r := tx.Table("order_summaries").Where("id=? AND order_id=? AND version=? AND deleted_at IS NULL", rows[0].ID, orderID, version).Updates(map[string]any{"totals": string(wire), "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil {
		return AmountDetail{}, ErrSchema
	}
	return amounts, nil
}
