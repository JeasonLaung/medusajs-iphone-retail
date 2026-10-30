package orders

import (
	"context"
	"encoding/json"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

type DeviceExchangeInput struct {
	OrderID, OriginalItemID, LocationID, OriginalAssignmentID, RequestID, ActorID string
	ExpectedVersion                                                               int
}
type DeviceExchangeResult struct {
	ExchangeID, ReturnID, ItemID, VariantID string
	ExchangeVersion, ReceivedVersion        int
	Amounts                                 AmountDetail
}

// ApplyDeviceExchangeTx is an internal native order participant. The owning
// device command authorizes and locks the paid order, processing exchange,
// returned original assignment and available replacement, then writes stock,
// device assignment and immutable command receipt in THIS transaction. It must
// roll back everything on any error. This does not commit or expose an API.
func ApplyDeviceExchangeTx(ctx context.Context, tx *gorm.DB, in DeviceExchangeInput) (out DeviceExchangeResult, err error) {
	for _, id := range []string{in.OrderID, in.OriginalItemID, in.LocationID, in.OriginalAssignmentID, in.RequestID, in.ActorID} {
		if !checkoutID.MatchString(id) {
			return out, ErrExchangeItem
		}
	}
	if in.ExpectedVersion < 1 || in.ExpectedVersion > 2147483645 || numberTransaction(ctx, tx) != nil {
		return out, ErrVersionConflict
	}
	var locationCount int64
	if tx.Table("inventory_locations").Where("id=? AND deleted_at IS NULL", in.LocationID).Count(&locationCount).Error != nil || locationCount != 1 {
		return out, ErrExchangeItem
	}
	first, e := AdvanceVersionTx(ctx, tx, in.OrderID, in.ExpectedVersion)
	if e != nil {
		return out, e
	}
	// Serialize native display-id allocation with all Gin native return writers.
	var schemaID int
	if tx.Table("order_runtime_schema").Select("id").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Scan(&schemaID).Error != nil || schemaID != 1 {
		return out, ErrSchema
	}
	item, e := CreateExchangeItemTx(ctx, tx, in.OrderID, in.OriginalItemID, first)
	if e != nil {
		return out, e
	}
	out = DeviceExchangeResult{ExchangeID: "oexc_" + uuid.NewString(), ReturnID: "return_" + uuid.NewString(), ItemID: item.ItemID, VariantID: item.VariantID, ExchangeVersion: first}
	var exchangeDisplay, returnDisplay int64
	if tx.Table("order_exchanges").Select("COALESCE(MAX(display_id),0)+1").Scan(&exchangeDisplay).Error != nil || tx.Table("returns").Select("COALESCE(MAX(display_id),0)+1").Scan(&returnDisplay).Error != nil || exchangeDisplay > maxDisplayID || returnDisplay > maxDisplayID {
		return out, ErrSchema
	}
	now := time.Now().UTC()
	metadata, _ := json.Marshal(map[string]string{"replacement_of": in.OriginalAssignmentID, "after_sales_request_id": in.RequestID})
	if tx.Table("order_exchanges").Create(map[string]any{"id": out.ExchangeID, "order_id": in.OrderID, "return_id": nil, "order_version": in.ExpectedVersion, "display_id": exchangeDisplay, "allow_backorder": false, "created_by": in.ActorID, "metadata": string(metadata), "created_at": now, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	if tx.Table("returns").Create(map[string]any{"id": out.ReturnID, "order_id": in.OrderID, "exchange_id": out.ExchangeID, "order_version": in.ExpectedVersion, "display_id": returnDisplay, "status": "requested", "location_id": in.LocationID, "requested_at": now, "metadata": string(metadata), "created_at": now, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	if tx.Table("order_exchanges").Where("id=?", out.ExchangeID).Update("return_id", out.ReturnID).Error != nil {
		return out, ErrSchema
	}
	_, rawOne, _ := exchangeNumber("1")
	_, rawZero, _ := exchangeNumber("0")
	retItem := "retitem_" + uuid.NewString()
	if tx.Table("return_items").Create(map[string]any{"id": retItem, "return_id": out.ReturnID, "item_id": in.OriginalItemID, "quantity": "1", "raw_quantity": rawOne, "received_quantity": "0", "raw_received_quantity": rawZero, "damaged_quantity": "0", "raw_damaged_quantity": rawZero, "created_at": now, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	if tx.Table("order_exchange_items").Create(map[string]any{"id": "oexitem_" + uuid.NewString(), "exchange_id": out.ExchangeID, "item_id": item.ItemID, "quantity": "1", "raw_quantity": rawOne, "metadata": "{}", "created_at": now, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	if e = exchangeQuantityTx(tx, in.OrderID, in.OriginalItemID, first, false); e != nil {
		return out, e
	}
	change, e := exchangeChangeTx(tx, in, out, first, "exchange", now)
	if e != nil {
		return out, e
	}
	if e = exchangeActionTx(tx, in, out, change, first, 0, "RETURN_ITEM", in.OriginalItemID, item.UnitPrice, now); e != nil {
		return out, e
	}
	if e = exchangeActionTx(tx, in, out, change, first, 1, "ITEM_ADD", item.ItemID, item.UnitPrice, now); e != nil {
		return out, e
	}
	if _, e = RefreshChangeSummaryTx(ctx, tx, in.OrderID, first); e != nil {
		return out, e
	}
	second, e := AdvanceVersionTx(ctx, tx, in.OrderID, first)
	if e != nil {
		return out, e
	}
	out.ReceivedVersion = second
	if e = exchangeQuantityTx(tx, in.OrderID, in.OriginalItemID, second, true); e != nil {
		return out, e
	}
	receive, e := exchangeChangeTx(tx, in, out, second, "return_receive", now)
	if e != nil {
		return out, e
	}
	if e = exchangeActionTx(tx, in, out, receive, second, 0, "RECEIVE_DAMAGED_RETURN_ITEM", in.OriginalItemID, item.UnitPrice, now); e != nil {
		return out, e
	}
	if tx.Table("return_items").Where("id=?", retItem).Updates(map[string]any{"received_quantity": "1", "raw_received_quantity": rawOne, "damaged_quantity": "1", "raw_damaged_quantity": rawOne, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	if tx.Table("returns").Where("id=?", out.ReturnID).Updates(map[string]any{"status": "received", "received_at": now, "updated_at": now}).Error != nil {
		return out, ErrSchema
	}
	out.Amounts, e = RefreshChangeSummaryTx(ctx, tx, in.OrderID, second)
	if e != nil {
		return out, e
	}
	return out, nil
}

func exchangeChangeTx(tx *gorm.DB, in DeviceExchangeInput, out DeviceExchangeResult, version int, kind string, now time.Time) (string, error) {
	id := "ordch_" + uuid.NewString()
	row := map[string]any{"id": id, "order_id": in.OrderID, "version": version, "status": "confirmed", "change_type": kind, "return_id": out.ReturnID, "created_by": in.ActorID, "confirmed_by": in.ActorID, "confirmed_at": now, "created_at": now, "updated_at": now}
	if kind == "exchange" {
		row["exchange_id"] = out.ExchangeID
	}
	if tx.Table("order_changes").Create(row).Error != nil {
		return "", ErrSchema
	}
	return id, nil
}
func exchangeActionTx(tx *gorm.DB, in DeviceExchangeInput, out DeviceExchangeResult, change string, version, ordering int, action, item, price string, now time.Time) error {
	detail := map[string]any{"reference_id": item, "quantity": 1}
	if action == "ITEM_ADD" {
		detail["unit_price"] = price
	}
	details, _ := json.Marshal(detail)
	scalar, raw, e := exchangeNumber(price)
	if e != nil {
		return e
	}
	ref, refID := "return", out.ReturnID
	row := map[string]any{"id": "ordact_" + uuid.NewString(), "order_id": in.OrderID, "order_change_id": change, "version": version, "ordering": ordering, "action": action, "details": string(details), "amount": scalar, "raw_amount": raw, "applied": true, "created_at": now, "updated_at": now, "return_id": out.ReturnID}
	if action == "ITEM_ADD" {
		ref, refID = "order_exchange", out.ExchangeID
		delete(row, "return_id")
	}
	if action != "RECEIVE_DAMAGED_RETURN_ITEM" {
		row["exchange_id"] = out.ExchangeID
	}
	row["reference"], row["reference_id"] = ref, refID
	if tx.Table("order_change_actions").Create(row).Error != nil {
		return ErrSchema
	}
	return nil
}

func exchangeQuantityTx(tx *gorm.DB, orderID, itemID string, version int, receive bool) error {
	var rows []map[string]any
	if tx.Table("order_items").Where("order_id=? AND item_id=? AND version=? AND deleted_at IS NULL", orderID, itemID, version).Clauses(clause.Locking{Strength: "UPDATE"}).Limit(2).Find(&rows).Error != nil || len(rows) != 1 {
		return ErrVersionConflict
	}
	row := rows[0]
	values := map[string]string{}
	for _, field := range []string{"fulfilled_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity"} {
		// GORM MySQL maps decimal/JSON columns to []byte; retain raw precedence.
		text := func(v any) (string, bool) {
			switch n := v.(type) {
			case string:
				return n, true
			case []byte:
				return string(n), true
			default:
				return "", false
			}
		}
		scalar, ok := text(row[field])
		if !ok {
			return ErrSchema
		}
		raw, ok := text(row["raw_"+field])
		if !ok {
			return ErrSchema
		}
		n, e := amountNumber(scalar, &raw)
		if e != nil {
			return e
		}
		v, e := totals.Resolve(n)
		if e != nil {
			return e
		}
		q, ok := new(big.Rat).SetString(v)
		if !ok || q.Sign() < 0 {
			return ErrSchema
		}
		values[field] = v
	}
	updates := map[string]any{"updated_at": gorm.Expr("UTC_TIMESTAMP(6)")}
	if !receive {
		remaining, e := totals.Sum(totals.Number{Decimal: values["fulfilled_quantity"]}, totals.Number{Decimal: "-" + values["return_requested_quantity"]}, totals.Number{Decimal: "-" + values["return_received_quantity"]}, totals.Number{Decimal: "-" + values["return_dismissed_quantity"]})
		if e != nil {
			return e
		}
		q, ok := new(big.Rat).SetString(remaining)
		if !ok || q.Cmp(big.NewRat(1, 1)) < 0 {
			return ErrVersionConflict
		}
	}
	for _, field := range []string{"return_requested_quantity", "return_dismissed_quantity", "written_off_quantity"} {
		if !receive && field != "return_requested_quantity" {
			continue
		}
		delta := "1"
		if receive && field == "return_requested_quantity" {
			delta = "-1"
		}
		value, e := totals.Sum(totals.Number{Decimal: values[field]}, totals.Number{Decimal: delta})
		if e != nil {
			return e
		}
		scalar, raw, e := exchangeNumber(value)
		if e != nil {
			return e
		}
		updates[field], updates["raw_"+field] = scalar, raw
	}
	if tx.Table("order_items").Where("id=?", row["id"]).Updates(updates).Error != nil {
		return ErrSchema
	}
	return nil
}
