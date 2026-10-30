// Package customernotices persists new-account notices in the business transaction.
package customernotices

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrUnavailable = errors.New("customer notice unavailable")
var resourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`)

// EmitOrderTx does not derive identity from orders.customer_id or email. Missing
// new-account ownership means no recipient, never an implicit account binding.
// Caller holds the order/business locks and must roll back on any failure.
// The inbox has no account FK: emitting after an order lock must not acquire an
// account lock opposite to customer account -> order reads. Ownership is read
// above through the authoritative account relation; old identities never enter.
func EmitOrderTx(tx *gorm.DB, orderID, kind, resource string) error {
	if tx == nil || tx.Statement == nil || !resourceID.MatchString(orderID) || !resourceID.MatchString(resource) {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	switch kind {
	case "shipment.created", "delivery.created", "order.canceled", "after_sales.processing", "after_sales.completed", "after_sales.rejected":
	default:
		return ErrUnavailable
	}
	var recipients []struct {
		CustomerID      string
		DisplayID       uint64
		CustomDisplayID *string
	}
	if tx.Table("customer_order_ownership own").Select("own.customer_id,o.display_id,o.custom_display_id").Joins("JOIN customer_accounts a ON a.id=own.customer_id AND a.enabled=TRUE AND a.verified_at IS NOT NULL").Joins("JOIN orders o ON o.id=own.order_id AND o.deleted_at IS NULL AND o.is_draft_order=FALSE").Where("own.order_id=?", orderID).Limit(2).Scan(&recipients).Error != nil || len(recipients) > 1 {
		return ErrUnavailable
	}
	if len(recipients) == 0 {
		return nil
	}
	to := recipients[0]
	reference := strconv.FormatUint(to.DisplayID, 10)
	if to.CustomDisplayID != nil && *to.CustomDisplayID != "" {
		reference = *to.CustomDisplayID
	}
	key := kind + ":" + resource
	var prior []struct{ CustomerID, Kind, ResourceID, Reference, Action string }
	if tx.Table("customer_account_notices").Where("event_key=?", key).Limit(2).Find(&prior).Error != nil || len(prior) > 1 {
		return ErrUnavailable
	}
	if len(prior) == 1 {
		if prior[0].CustomerID != to.CustomerID || prior[0].Kind != kind || prior[0].ResourceID != orderID || prior[0].Action != "" {
			return ErrUnavailable
		}
		return nil
	}
	return tx.Table("customer_account_notices").Create(map[string]any{"id": "notice_" + uuid.NewString(), "customer_id": to.CustomerID, "event_key": key, "kind": kind, "resource_id": orderID, "reference": reference, "action": "", "created_at": time.Now().UTC()}).Error
}
