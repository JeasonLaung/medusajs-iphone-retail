package buyback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCommandState = errors.New("buyback command not allowed in current state")
var ErrCommandIdentity = errors.New("buyback identities required")
var ErrCommandPhotos = errors.New("buyback photos required")
var ErrCommandOffer = errors.New("buyback offer changed")
var commandIMEI = regexp.MustCompile(`^[0-9]{15}$`)

type CommandIdentity struct {
	ItemID       string `json:"item_id"`
	SerialNumber string `json:"serial_number"`
	IMEI         string `json:"imei"`
}
type CustomerCommand struct {
	Action         string            `json:"action"`
	Items          []CommandIdentity `json:"items,omitempty"`
	TrackingNumber string            `json:"tracking_number,omitempty"`
	FinalAmount    *json.Number      `json:"final_amount,omitempty"`
}

func ParseCustomerCommand(raw []byte) (CustomerCommand, error) {
	var out CustomerCommand
	// Reject duplicates, nulls and unknown fields before selecting the union arm.
	if decode(raw, &out, "action") != nil {
		return out, ErrInput
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	switch out.Action {
	case "submit_photos":
		if len(fields) != 2 || fields["items"] == nil || len(out.Items) < 1 || len(out.Items) > 10 {
			return CustomerCommand{}, ErrInput
		}
		var items []json.RawMessage
		if json.Unmarshal(fields["items"], &items) != nil {
			return CustomerCommand{}, ErrInput
		}
		for i, r := range items {
			var item CommandIdentity
			if decode(r, &item, "item_id", "serial_number", "imei") != nil {
				return CustomerCommand{}, ErrInput
			}
			var e error
			item.ItemID, e = trimSubmission(item.ItemID, 1, 100)
			if e != nil {
				return CustomerCommand{}, e
			}
			item.SerialNumber = cases.Upper(language.Und).String(strings.Trim(item.SerialNumber, jsWhitespace))
			if _, e = trimSubmission(item.SerialNumber, 1, 100); e != nil || !commandIMEI.MatchString(item.IMEI) {
				return CustomerCommand{}, ErrInput
			}
			out.Items[i] = item
		}
	case "mark_shipped":
		if len(fields) != 2 || fields["tracking_number"] == nil {
			return CustomerCommand{}, ErrInput
		}
		var e error
		out.TrackingNumber, e = trimSubmission(out.TrackingNumber, 3, 150)
		if e != nil {
			return CustomerCommand{}, e
		}
	case "accept_offer":
		if len(fields) != 2 || fields["final_amount"] == nil || out.FinalAmount == nil || bytes.HasPrefix(bytes.TrimSpace(fields["final_amount"]), []byte{'"'}) {
			return CustomerCommand{}, ErrInput
		}
		// Zod observes a JS Number: normalize to binary64 before checking cents.
		value, e := strconv.ParseFloat(string(*out.FinalAmount), 64)
		if e != nil || math.IsInf(value, 0) || math.IsNaN(value) || value < 0 || value > 1000000 {
			return CustomerCommand{}, ErrInput
		}
		normalized := strconv.FormatFloat(value, 'f', -1, 64)
		r, ok := new(big.Rat).SetString(normalized)
		if !ok || !r.Mul(r, big.NewRat(100, 1)).IsInt() {
			return CustomerCommand{}, ErrInput
		}
		n := json.Number(normalized)
		out.FinalAmount = &n
	case "decline_offer", "cancel":
		if len(fields) != 1 {
			return CustomerCommand{}, ErrInput
		}
	default:
		return CustomerCommand{}, ErrInput
	}
	return out, nil
}

// StageCustomerCommand preserves original state guards, including rejection of
// repeats after a successful transition. It has no payment/provider side effects.
func StageCustomerCommand(ctx context.Context, tx *gorm.DB, storeID, accountID, id string, in CustomerCommand) (RequestDetail, error) {
	var out RequestDetail
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrInput
	}
	in, e = ParseCustomerCommand(raw)
	if e != nil {
		return out, e
	}
	if tx == nil || !validID(id, 191) {
		return out, ErrMissing
	}
	tx = tx.WithContext(ctx)
	var locked struct{ ID string }
	e = tx.Table("buyback_requests r").Select("r.id").Joins("JOIN buyback_account_access b ON b.request_id=r.id").Where("r.id=? AND r.deleted_at IS NULL AND b.customer_id=? AND b.store_id=?", id, accountID, storeID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&locked).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrMissing
	}
	if e != nil {
		return out, ErrUnavailable
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("buyback_request_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || !gate.Enabled {
		return out, ErrUnavailable
	}
	out, e = ReadRequest(ctx, tx, storeID, accountID, id)
	if e != nil {
		return RequestDetail{}, e
	}
	status := out.Status
	changes := map[string]any{}
	switch in.Action {
	case "submit_photos":
		if status != "awaiting_photos" {
			return RequestDetail{}, ErrCommandState
		}
		if len(in.Items) != len(out.Items) {
			return RequestDetail{}, ErrCommandIdentity
		}
		identities := map[string]CommandIdentity{}
		serials, imeis := map[string]bool{}, map[string]bool{}
		for _, v := range in.Items {
			if _, ok := identities[v.ItemID]; ok || serials[v.SerialNumber] || imeis[v.IMEI] {
				return RequestDetail{}, ErrCommandIdentity
			}
			identities[v.ItemID] = v
			serials[v.SerialNumber] = true
			imeis[v.IMEI] = true
		}
		for i, item := range out.Items {
			v, ok := identities[item.ID]
			if !ok {
				return RequestDetail{}, ErrCommandIdentity
			}
			out.Items[i].SerialNumber = v.SerialNumber
			out.Items[i].IMEI = v.IMEI
			for _, required := range item.Assessment.RequiredPhotos() {
				found := false
				for _, p := range out.Photos {
					if p.ItemID == item.ID && p.PhotoType == required && p.ReviewStatus != "expired" && p.ReviewStatus != "retake" {
						found = true
						break
					}
				}
				if !found {
					return RequestDetail{}, ErrCommandPhotos
				}
			}
		}
		status = "photo_review"
	case "mark_shipped":
		if status != "ready_to_ship" {
			return RequestDetail{}, ErrCommandState
		}
		status = "in_transit"
		changes["tracking_number"] = in.TrackingNumber
	case "accept_offer":
		if status != "final_offer" || out.FinalAmount == nil {
			return RequestDetail{}, ErrCommandOffer
		}
		current, e := strconv.ParseFloat(string(*out.FinalAmount), 64)
		wanted, err := strconv.ParseFloat(string(*in.FinalAmount), 64)
		if e != nil || err != nil || current != wanted {
			return RequestDetail{}, ErrCommandOffer
		}
		status = "accepted"
	case "decline_offer":
		if status != "final_offer" {
			return RequestDetail{}, ErrCommandState
		}
		status = "return_requested"
	case "cancel":
		if status != "awaiting_photos" && status != "photo_review" && status != "ready_to_ship" {
			return RequestDetail{}, ErrCommandState
		}
		status = "canceled"
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	history, _ := json.Marshal(map[string]any{"at": now, "action": in.Action, "actor": "customer", "status": status, "note": ""})
	out.History = append(out.History, history)
	savedItems, _ := json.Marshal(out.Items)
	savedHistory, _ := json.Marshal(out.History)
	changes["items"] = string(savedItems)
	changes["history"] = string(savedHistory)
	changes["status"] = status
	changes["updated_at"] = now
	if status == "canceled" {
		changes["closed_at"] = now
	}
	if tx.Table("buyback_requests").Where("id=? AND deleted_at IS NULL", id).Updates(changes).Error != nil {
		return RequestDetail{}, ErrUnavailable
	}
	return ReadRequest(ctx, tx, storeID, accountID, id)
}
