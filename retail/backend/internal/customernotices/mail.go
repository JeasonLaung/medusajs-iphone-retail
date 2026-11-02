package customernotices

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

// StageMailTx is called inside the business transaction, independently of inbox
// coalescing. It stores no recipient address or private message content. Account
// identity is explicit; legacy email equality never grants a recipient.
func StageMailTx(tx *gorm.DB, account, key, kind, resource, reference, action string, now time.Time) error {
	if tx == nil || tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	if !resourceID.MatchString(account) || !resourceID.MatchString(resource) || len(key) > 255 || key == "" || len(reference) > 191 {
		return ErrUnavailable
	}
	var existing []struct{ CustomerID, Kind, ResourceID, Reference, Action string }
	if tx.Table("customer_event_mail").Where("event_key=?", key).Limit(2).Find(&existing).Error != nil || len(existing) > 1 {
		return ErrUnavailable
	}
	if len(existing) == 1 {
		old := existing[0]
		if old.CustomerID != account || old.Kind != kind || old.ResourceID != resource || old.Reference != reference || old.Action != action {
			return ErrUnavailable
		}
		return nil
	}
	return tx.Table("customer_event_mail").Create(map[string]any{"id": uuid.NewString(), "customer_id": account, "event_key": key, "kind": kind, "resource_id": resource, "reference": reference, "action": action, "status": "pending", "created_at": now}).Error
}
