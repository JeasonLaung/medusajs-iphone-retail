package buyback

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrReviewNote = errors.New("buyback retake reason required")

type AdminCommand struct {
	Action               string `json:"action"`
	PhotoID              string `json:"photo_id,omitempty"`
	ReviewStatus         string `json:"review_status,omitempty"`
	Note                 string `json:"note,omitempty"`
	ShippingInstructions string `json:"shipping_instructions,omitempty"`
}

func ParseAdminCommand(raw []byte) (AdminCommand, error) {
	var out AdminCommand
	if decode(raw, &out, "action") != nil {
		return out, ErrInput
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	switch out.Action {
	case "review_photo":
		if len(fields) < 3 || len(fields) > 4 || fields["photo_id"] == nil || fields["review_status"] == nil || fields["shipping_instructions"] != nil {
			return AdminCommand{}, ErrInput
		}
		var e error
		out.PhotoID, e = trimSubmission(out.PhotoID, 1, 100)
		if e != nil {
			return AdminCommand{}, e
		}
		if out.ReviewStatus != "approved" && out.ReviewStatus != "retake" {
			return AdminCommand{}, ErrInput
		}
		out.Note, e = trimSubmission(out.Note, 0, 500)
		if e != nil {
			return AdminCommand{}, e
		}
	case "approve_shipping":
		if len(fields) != 2 || fields["shipping_instructions"] == nil {
			return AdminCommand{}, ErrInput
		}
		var e error
		out.ShippingInstructions, e = trimSubmission(out.ShippingInstructions, 5, 2000)
		if e != nil {
			return AdminCommand{}, e
		}
	case "mark_received":
		if len(fields) != 1 {
			return AdminCommand{}, ErrInput
		}
	case "cancel":
		if len(fields) != 2 || fields["note"] == nil {
			return AdminCommand{}, ErrInput
		}
		var e error
		out.Note, e = trimSubmission(out.Note, 3, 1000)
		if e != nil {
			return AdminCommand{}, e
		}
	default:
		return AdminCommand{}, ErrInput
	}
	return out, nil
}
func adminGate(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) (bool, error) {
	actions := []authorization.Action{authorization.BuybackRead}
	if write {
		actions = append(actions, authorization.BuybackWrite)
	}
	if e := authorization.CheckTx(ctx, tx, actor, actions...); e != nil {
		return false, e
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("buyback_admin_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 {
		return false, ErrUnavailable
	}
	if write && !gate.Enabled {
		return false, ErrUnavailable
	}
	return gate.Enabled, nil
}

// Staff scope is explicit and current. Legacy customer IDs never grant access.
func adminOwner(tx *gorm.DB, store, id string, lock bool) (string, error) {
	if !validID(store, 191) || !validID(id, 191) {
		return "", ErrMissing
	}
	var row struct{ CustomerID string }
	q := tx.Table("buyback_requests r").Select("b.customer_id").Joins("JOIN customer_buyback_submissions b ON b.request_id=r.id AND b.customer_id=r.customer_id").Joins("JOIN commerce_stores s ON s.id=b.store_id AND s.deleted_at IS NULL").Where("r.id=? AND r.deleted_at IS NULL AND b.store_id=?", id, store)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	e := q.Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return "", ErrMissing
	}
	if e != nil {
		return "", ErrUnavailable
	}
	return row.CustomerID, nil
}

type AdminDetail struct {
	Request RequestDetail `json:"request"`
	Enabled bool          `json:"enabled"`
}

func AdminRequestTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, id string, in *AdminCommand) (AdminDetail, error) {
	var out AdminDetail
	enabled, e := adminGate(ctx, tx, actor, in != nil)
	if e != nil {
		return out, e
	}
	out.Enabled = enabled
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return out, ErrInput
		}
		v, err := ParseAdminCommand(raw)
		if err != nil {
			return out, err
		}
		in = &v
	}
	account, e := adminOwner(tx, store, id, in != nil)
	if e != nil {
		return out, e
	}
	request, e := ReadRequest(ctx, tx, store, account, id)
	if e != nil {
		return out, e
	}
	if in == nil {
		out.Request = request
		return out, nil
	}
	status := request.Status
	changes := map[string]any{}
	notice := true
	switch in.Action {
	case "review_photo":
		if status != "awaiting_photos" && status != "photo_review" {
			return out, ErrCommandState
		}
		found := false
		for _, p := range request.Photos {
			if p.ID == in.PhotoID && p.ReviewStatus != "expired" {
				found = true
			}
		}
		if !found {
			return out, ErrPhotoMissing
		}
		if in.ReviewStatus == "retake" {
			if _, e := trimSubmission(in.Note, 3, 500); e != nil {
				return out, ErrReviewNote
			}
			status = "awaiting_photos"
			changes["review_note"] = in.Note
		} else {
			notice = false
		}
		if tx.Table("buyback_photos").Where("id=? AND request_id=? AND deleted_at IS NULL", in.PhotoID, id).Updates(map[string]any{"review_status": in.ReviewStatus, "review_note": in.Note, "updated_at": time.Now().UTC()}).Error != nil {
			return out, ErrUnavailable
		}
	case "approve_shipping":
		if status != "photo_review" {
			return out, ErrCommandState
		}
		for _, item := range request.Items {
			for _, required := range item.Assessment.RequiredPhotos() {
				found := false
				for _, p := range request.Photos {
					if p.ItemID == item.ID && p.PhotoType == required && p.ReviewStatus == "approved" {
						found = true
					}
				}
				if !found {
					return out, ErrCommandPhotos
				}
			}
		}
		status = "ready_to_ship"
		changes["shipping_instructions"] = in.ShippingInstructions
	case "mark_received":
		if status != "ready_to_ship" && status != "in_transit" {
			return out, ErrCommandState
		}
		status = "received"
	case "cancel":
		if status != "awaiting_photos" && status != "photo_review" && status != "ready_to_ship" {
			return out, ErrCommandState
		}
		status = "canceled"
		changes["review_note"] = in.Note
	}
	return finishAdminCommandTx(ctx, tx, actor, store, account, request, out, in.Action, in.Note, in.PhotoID, status, changes, notice)
}

func finishAdminCommandTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, account string, request RequestDetail, out AdminDetail, action, note, photoID, status string, changes map[string]any, notice bool) (AdminDetail, error) {
	id := request.ID
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := map[string]any{"at": now, "action": action, "actor": "platform", "status": status, "note": note}
	if action == "review_photo" {
		event["photo_id"] = photoID
	}
	raw, e := json.Marshal(event)
	if e != nil {
		return out, ErrUnavailable
	}
	history, e := json.Marshal(append(request.History, raw))
	if e != nil || len(history) > 1048576 {
		return out, ErrUnavailable
	}
	changes["status"] = status
	changes["history"] = string(history)
	changes["updated_at"] = now
	if status == "canceled" {
		changes["closed_at"] = now
	}
	if tx.Table("buyback_requests").Where("id=? AND deleted_at IS NULL", id).Updates(changes).Error != nil {
		return out, ErrUnavailable
	}
	if notice {
		// Owned recipient, same transaction; neither mail nor provider calls under locks.
		var active int64
		if tx.Table("customer_accounts").Where("id=? AND enabled=TRUE AND verified_at IS NOT NULL", account).Count(&active).Error != nil {
			return out, ErrUnavailable
		}
		if active == 1 {
			if tx.Table("customer_account_notices").Create(map[string]any{"id": "notice_" + uuid.NewString(), "customer_id": account, "event_key": "buyback:" + id + ":" + uuid.NewString(), "kind": "buyback.updated", "resource_id": id, "reference": id, "action": action, "created_at": now}).Error != nil {
				return out, ErrUnavailable
			}
		}
	}
	audit, _ := json.Marshal(map[string]any{"action": action, "photo_id": photoID, "from_status": request.Status, "to_status": status})
	if tx.Table("audit_events").Create(map[string]any{"id": "audit_" + uuid.NewString(), "actor_id": actor.ID, "action": "buyback.command", "resource_id": id, "data": string(audit), "created_at": now}).Error != nil {
		return out, ErrUnavailable
	}
	out.Request, e = ReadRequest(ctx, tx, store, account, id)
	return out, e
}
func AdminPhotoTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, id, photo string) (PhotoContent, error) {
	if _, e := adminGate(ctx, tx, actor, false); e != nil {
		return PhotoContent{}, e
	}
	if _, e := adminOwner(tx, store, id, false); e != nil {
		return PhotoContent{}, e
	}
	return readPhotoBytes(ctx, tx, id, photo)
}

type AdminList struct {
	Requests []RequestDetail `json:"requests"`
	Count    int64           `json:"count"`
	Offset   int             `json:"offset"`
	Limit    int             `json:"limit"`
}

func AdminListTx(ctx context.Context, tx *gorm.DB, actor identity.User, store, status, search string, offset int) (AdminList, error) {
	out := AdminList{Requests: []RequestDetail{}, Offset: offset, Limit: 20}
	if _, e := adminGate(ctx, tx, actor, false); e != nil {
		return out, e
	}
	if !validID(store, 191) || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	query := func() *gorm.DB {
		q := tx.Table("buyback_requests r").Joins("JOIN customer_buyback_submissions b ON b.request_id=r.id AND b.customer_id=r.customer_id").Joins("JOIN commerce_stores s ON s.id=b.store_id AND s.deleted_at IS NULL").Where("b.store_id=? AND r.deleted_at IS NULL", store)
		if status != "" {
			q = q.Where("r.status=?", status)
		}
		if search != "" {
			pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(search, "!", "!!"), "%", "!%"), "_", "!_") + "%"
			q = q.Where("(r.id LIKE ? ESCAPE '!' OR r.email LIKE ? ESCAPE '!' OR r.contact_name LIKE ? ESCAPE '!' OR r.tracking_number LIKE ? ESCAPE '!')", pattern, pattern, pattern, pattern)
		}
		return q
	}
	if query().Count(&out.Count).Error != nil {
		return out, ErrUnavailable
	}
	var rows []struct{ ID, CustomerID string }
	if query().Select("r.id,b.customer_id").Order("r.created_at DESC,r.id DESC").Limit(20).Offset(offset).Scan(&rows).Error != nil {
		return out, ErrUnavailable
	}
	for _, r := range rows {
		request, e := ReadRequest(ctx, tx, store, r.CustomerID, r.ID)
		if e != nil {
			return out, e
		}
		out.Requests = append(out.Requests, request)
	}
	return out, nil
}
