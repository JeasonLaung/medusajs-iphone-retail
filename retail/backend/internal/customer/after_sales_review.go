package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReviewInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
}
type ReviewContext struct {
	OrderID  string              `json:"order_id"`
	Revision string              `json:"revision"`
	Enabled  bool                `json:"enabled"`
	Requests []AfterSalesRequest `json:"requests"`
}
type reviewAssignment struct {
	ID, DeviceID, Status string
	ReplacementOf        *string
	State                *string
}
type reviewFacts struct {
	Requests []struct {
		ID, CustomerID, Type, Status, Reason, Items string
		UpdatedAt                                   time.Time
	}
	Assignments map[string][]reviewAssignment
	Refunds     []struct {
		ID, Note, Amount, RawAmount string
		UpdatedAt                   time.Time
	}
}

func reviewGate(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) (bool, error) {
	actions := []authorization.Action{authorization.OrderRead, authorization.OrderPaymentRead, authorization.DeviceRead}
	if write {
		actions = append(actions, authorization.OrderAfterSalesWrite)
	}
	if e := authorization.CheckTx(ctx, tx, actor, actions...); e != nil {
		return false, e
	}
	var device struct {
		Version  int
		Revision uint64
	}
	if tx.Table("device_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&device).Error != nil || device.Version != 1 || device.Revision == 0 {
		return false, ErrUnavailable
	}
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("after_sales_review_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || orders.Ready(ctx, tx) != nil || payments.Ready(ctx, tx) != nil {
		return false, ErrUnavailable
	}
	return gate.Enabled, nil
}
func reviewLoad(tx *gorm.DB, orderID string, enabled bool) (ReviewContext, reviewFacts, error) {
	out := ReviewContext{OrderID: orderID, Enabled: enabled, Requests: []AfterSalesRequest{}}
	facts := reviewFacts{Assignments: map[string][]reviewAssignment{}}
	var order struct {
		ID        string
		Version   int
		UpdatedAt time.Time
	}
	if e := tx.Table("orders").Select("id,version,updated_at").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&order).Error; e != nil {
		return out, facts, e
	}
	if tx.Table("after_sales_requests").Select("id,customer_id,type,status,reason,items,updated_at").Where("order_id=? AND deleted_at IS NULL", orderID).Order("created_at DESC,id DESC").Limit(1001).Scan(&facts.Requests).Error != nil || len(facts.Requests) > 1000 {
		return out, facts, ErrUnavailable
	}
	for _, r := range facts.Requests {
		var saved struct {
			Lines []AfterSalesLine `json:"lines"`
		}
		if json.Unmarshal([]byte(r.Items), &saved) != nil || len(saved.Lines) < 1 || len(saved.Lines) > 100 {
			return out, facts, ErrUnavailable
		}
		var stamp struct{ CreatedAt time.Time }
		if tx.Table("after_sales_requests").Select("created_at").Where("id=?", r.ID).Take(&stamp).Error != nil {
			return out, facts, ErrUnavailable
		}
		out.Requests = append(out.Requests, AfterSalesRequest{ID: r.ID, Type: r.Type, Status: r.Status, Reason: r.Reason, Items: saved.Lines, CreatedAt: stamp.CreatedAt})
		rows := []reviewAssignment{}
		if tx.Table("device_assignments a").Select("a.id,a.device_id,a.status,a.replacement_of,d.state").Joins("LEFT JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.after_sales_request_id=? AND a.deleted_at IS NULL", r.ID).Order("a.id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
			return out, facts, ErrUnavailable
		}
		facts.Assignments[r.ID] = rows
	}
	if tx.Table("payment_refunds r").Select("r.id,r.note,CAST(r.amount AS CHAR) AS amount,r.raw_amount,r.updated_at").Joins("JOIN payments p ON p.id=r.payment_id AND p.deleted_at IS NULL").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.deleted_at IS NULL").Where("l.order_id=? AND r.deleted_at IS NULL", orderID).Order("r.id").Limit(1001).Scan(&facts.Refunds).Error != nil || len(facts.Refunds) > 1000 {
		return out, facts, ErrUnavailable
	}
	data, _ := json.Marshal(struct {
		Order any
		Facts reviewFacts
	}{order, facts})
	digest := sha256.Sum256(data)
	out.Revision = hex.EncodeToString(digest[:])
	return out, facts, nil
}
func checkReviewCompletion(request AfterSalesRequest, received []reviewAssignment, refunded bool, status string) error {
	if request.Status == "completed" || request.Status == "rejected" {
		return ErrAfterSalesConflict
	}
	if status == "rejected" && len(received) > 0 {
		return ErrAfterSalesConflict
	}
	if status != "completed" {
		return nil
	}
	count := 0
	for _, line := range request.Items {
		for _, deviceID := range line.DeviceIDs {
			count++
			var original *reviewAssignment
			for i := range received {
				if received[i].DeviceID == deviceID && received[i].ReplacementOf == nil {
					if original != nil {
						return ErrAfterSalesConflict
					}
					original = &received[i]
				}
			}
			if original == nil || original.State == nil || *original.State == "inspection" || *original.State == "repair" || *original.State == "repair_ready" {
				return ErrAfterSalesConflict
			}
			if request.Type == "repair" && original.Status != "shipped" {
				return ErrAfterSalesConflict
			}
			if request.Type == "exchange" {
				shipped := false
				for _, r := range received {
					if r.ReplacementOf != nil && *r.ReplacementOf == original.ID && r.Status == "shipped" {
						shipped = true
					}
				}
				if !shipped {
					return ErrAfterSalesConflict
				}
			}
		}
	}
	if count > 0 && request.Type == "return" && !refunded {
		return ErrAfterSalesConflict
	}
	return nil
}

// ReviewAfterSalesTx is called inside the authenticated handler transaction;
// it never performs a refund, device movement or notification as a side effect.
func ReviewAfterSalesTx(ctx context.Context, tx *gorm.DB, actor identity.User, orderID string, input *ReviewInput) (ReviewContext, error) {
	var out ReviewContext
	if !customerOrderID.MatchString(orderID) || len(orderID) > 191 {
		return out, ErrInvalid
	}
	enabled, e := reviewGate(ctx, tx, actor, input != nil)
	if e != nil {
		return out, e
	}
	before, facts, e := reviewLoad(tx, orderID, enabled)
	if e != nil {
		return out, e
	}
	if input == nil {
		return before, nil
	}
	id, e := uuid.Parse(input.OperationID)
	if e != nil || id.Version() != 4 || id.String() != input.OperationID || len(input.ExpectedRevision) != 64 || !customerOrderID.MatchString(input.RequestID) || (input.Status != "processing" && input.Status != "completed" && input.Status != "rejected") {
		return out, ErrInvalid
	}
	if _, e := hex.DecodeString(input.ExpectedRevision); e != nil {
		return out, ErrInvalid
	}
	var request *AfterSalesRequest
	for i := range before.Requests {
		if before.Requests[i].ID == input.RequestID {
			request = &before.Requests[i]
		}
	}
	if request == nil {
		return out, gorm.ErrRecordNotFound
	}
	raw, _ := json.Marshal(input)
	var receipt struct{ ActorID, OrderID, RequestJSON, ResultJSON string }
	e = tx.Table("after_sales_reviews").Where("operation_id=?", input.OperationID).Take(&receipt).Error
	if e == nil {
		if receipt.ActorID != actor.ID || receipt.OrderID != orderID || receipt.RequestJSON != string(raw) {
			return out, ErrAfterSalesConflict
		}
		if json.Unmarshal([]byte(receipt.ResultJSON), &out) != nil {
			return out, ErrUnavailable
		}
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, ErrUnavailable
	}
	if !enabled {
		return out, ErrUnavailable
	}
	if before.Revision != input.ExpectedRevision {
		return out, ErrAfterSalesConflict
	}
	refunded := false
	for _, refund := range facts.Refunds {
		if refund.Note == "after-sales:"+request.ID {
			positive, e := orders.PositiveAfterSalesRefund(refund.Amount, refund.RawAmount)
			if e != nil {
				return out, e
			}
			refunded = refunded || positive
		}
	}
	if e := checkReviewCompletion(*request, facts.Assignments[request.ID], refunded, input.Status); e != nil {
		return out, e
	}
	if tx.Table("after_sales_requests").Where("id=? AND deleted_at IS NULL", request.ID).Updates(map[string]any{"status": input.Status, "updated_at": time.Now().UTC()}).Error != nil {
		return out, ErrUnavailable
	}
	out, _, e = reviewLoad(tx, orderID, enabled)
	if e != nil {
		return out, e
	}
	saved, _ := json.Marshal(out)
	if tx.Table("after_sales_reviews").Create(map[string]any{"operation_id": input.OperationID, "actor_id": actor.ID, "order_id": orderID, "request_id": request.ID, "request_json": string(raw), "before_status": request.Status, "after_status": input.Status, "result_json": string(saved), "created_at": time.Now().UTC()}).Error != nil {
		return out, ErrUnavailable
	}
	return out, nil
}
