package customer

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAfterSalesConflict = errors.New("after-sales request conflicts with current order")

type AfterSalesLine struct {
	ItemID    string   `json:"item_id"`
	Quantity  int      `json:"quantity"`
	DeviceIDs []string `json:"device_ids,omitempty"`
	Title     string   `json:"title"`
}
type AfterSalesInput struct {
	OperationID string           `json:"operation_id"`
	Type        string           `json:"type"`
	Reason      string           `json:"reason"`
	Items       []AfterSalesLine `json:"items"`
}
type AfterSalesRequest struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	Status    string           `json:"status"`
	Reason    string           `json:"reason"`
	Items     []AfterSalesLine `json:"items" gorm:"-"`
	CreatedAt time.Time        `json:"created_at"`
}
type OrderDevice struct {
	ID             string  `json:"id"`
	AssignmentID   string  `json:"assignment_id"`
	ItemID         string  `json:"item_id"`
	SerialNumber   *string `json:"serial_number"`
	IMEI           *string `json:"imei"`
	Status         string  `json:"status"`
	ReplacementOf  *string `json:"replacement_of"`
	TrackingNumber *string `json:"tracking_number"`
}

func validAfterSales(in AfterSalesInput) bool {
	id, e := uuid.Parse(in.OperationID)
	if e != nil || id.Version() != 4 || id.String() != in.OperationID || !utf8.ValidString(in.Reason) || strings.TrimSpace(in.Reason) != in.Reason || utf8.RuneCountInString(in.Reason) < 10 || utf8.RuneCountInString(in.Reason) > 2000 || strings.ContainsFunc(in.Reason, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) || len(in.Items) < 1 || len(in.Items) > 100 || (in.Type != "return" && in.Type != "exchange" && in.Type != "repair") {
		return false
	}
	seen := map[string]bool{}
	for _, l := range in.Items {
		if !customerOrderID.MatchString(l.ItemID) || len(l.ItemID) > 191 || seen[l.ItemID] || l.Quantity < 1 || l.Quantity > 10000 || len(l.DeviceIDs) > 100 || l.Title != "" {
			return false
		}
		seen[l.ItemID] = true
		devices := map[string]bool{}
		for _, d := range l.DeviceIDs {
			if !customerOrderID.MatchString(d) || len(d) > 100 || devices[d] {
				return false
			}
			devices[d] = true
		}
	}
	return true
}

func afterSalesGate(tx *gorm.DB, write bool) error {
	var gate struct {
		Version int
		Enabled bool
	}
	if tx.Table("customer_after_sales_runtime").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || write && !gate.Enabled {
		return ErrUnavailable
	}
	return nil
}
func (s *Store) orderTransaction(ctx context.Context, token, orderID string, write bool, action func(*gorm.DB, string, cart.Confirmation) error) error {
	if !customerOrderID.MatchString(orderID) || len(orderID) > 191 {
		return ErrOrderMissing
	}
	var business error
	err := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		if write {
			// Same serialization point/order as existing device fulfillment commands.
			// Filing a request itself does not require enabling device mutations.
			var gate struct {
				Version  int
				Revision uint64
			}
			if tx.Table("device_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || gate.Revision == 0 {
				return ErrUnavailable
			}
			var order struct{ ID string }
			if tx.Table("orders").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", orderID).Take(&order).Error != nil {
				business = ErrOrderMissing
				return business
			}
		}
		order, e := cart.ReadAccountConfirmation(ctx, tx, account, orderID)
		if e != nil {
			if errors.Is(e, cart.ErrDenied) {
				business = ErrOrderMissing
			}
			return e
		}
		business = action(tx, account, order)
		return business
	})
	if business != nil && (errors.Is(business, ErrInvalid) || errors.Is(business, ErrOrderMissing) || errors.Is(business, ErrAfterSalesConflict)) {
		return business
	}
	return err
}
func readAfterSales(tx *gorm.DB, account, orderID string) ([]AfterSalesRequest, error) {
	var rows []struct {
		AfterSalesRequest
		SavedItems string `gorm:"column:items"`
	}
	if tx.Table("after_sales_requests").Select("id,type,status,reason,items,created_at").Where("order_id=? AND customer_id=? AND deleted_at IS NULL", orderID, account).Order("created_at DESC,id DESC").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return nil, ErrUnavailable
	}
	out := []AfterSalesRequest{}
	for _, row := range rows {
		var items struct {
			Lines []AfterSalesLine `json:"lines"`
		}
		if json.Unmarshal([]byte(row.SavedItems), &items) != nil || len(items.Lines) == 0 || len(items.Lines) > 100 {
			return nil, ErrUnavailable
		}
		row.AfterSalesRequest.Items = items.Lines
		out = append(out, row.AfterSalesRequest)
	}
	return out, nil
}
func (s *Store) AfterSales(ctx context.Context, token, orderID string) ([]AfterSalesRequest, error) {
	out := []AfterSalesRequest{}
	e := s.orderTransaction(ctx, token, orderID, false, func(tx *gorm.DB, account string, _ cart.Confirmation) error {
		if e := afterSalesGate(tx, false); e != nil {
			return e
		}
		var e error
		out, e = readAfterSales(tx, account, orderID)
		return e
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Store) OrderDevices(ctx context.Context, token, orderID string) ([]OrderDevice, error) {
	out := []OrderDevice{}
	e := s.orderTransaction(ctx, token, orderID, false, func(tx *gorm.DB, _ string, _ cart.Confirmation) error {
		// Preserve original endpoint: shipped/returned assignments only, and only
		// SN/primary IMEI of the assigned device. No EID, cost, stock or supplier data.
		if tx.Table("device_assignments a").Select("a.device_id AS id,a.id AS assignment_id,a.item_id,d.serial_number,d.imei,a.status,a.replacement_of,a.tracking_number").Joins("LEFT JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("a.order_id=? AND a.deleted_at IS NULL AND a.status IN ?", orderID, []string{"shipped", "returned"}).Order("a.created_at DESC,a.id").Limit(1001).Scan(&out).Error != nil || len(out) > 1000 {
			return ErrUnavailable
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Store) CreateAfterSales(ctx context.Context, token, orderID string, in AfterSalesInput) (AfterSalesRequest, error) {
	var out AfterSalesRequest
	if !validAfterSales(in) {
		return out, ErrInvalid
	}
	encoded, _ := json.Marshal(in)
	e := s.orderTransaction(ctx, token, orderID, true, func(tx *gorm.DB, account string, order cart.Confirmation) error {
		if e := afterSalesGate(tx, false); e != nil {
			return e
		}
		var receipt struct{ CustomerID, OrderID, RequestJSON, ResultJSON string }
		e := tx.Table("customer_after_sales_operations").Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.CustomerID != account || receipt.OrderID != orderID || receipt.RequestJSON != string(encoded) {
				return ErrAfterSalesConflict
			}
			if json.Unmarshal([]byte(receipt.ResultJSON), &out) != nil {
				return ErrUnavailable
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if e := afterSalesGate(tx, true); e != nil {
			return e
		}
		if order.Status == "canceled" || order.Status == "draft" {
			return ErrAfterSalesConflict
		}
		var count int64
		if tx.Table("after_sales_requests").Where("order_id=? AND deleted_at IS NULL AND status IN ?", orderID, []string{"pending", "processing"}).Count(&count).Error != nil {
			return ErrUnavailable
		}
		if count > 0 {
			return ErrAfterSalesConflict
		}
		quantities := map[string]*big.Rat{}
		titles := map[string]string{}
		for _, l := range order.Amounts.Lines {
			q, ok := new(big.Rat).SetString(l.Quantity)
			if !ok {
				return ErrUnavailable
			}
			quantities[l.ItemID] = q
		}
		for _, l := range order.Items {
			titles[l.ID] = l.Title
		}
		lines := append([]AfterSalesLine{}, in.Items...)
		for i := range lines {
			l := &lines[i]
			available, ok := quantities[l.ItemID]
			requested := big.NewRat(int64(l.Quantity), 1)
			if !ok || requested.Cmp(available) > 0 {
				return ErrInvalid
			}
			var item struct{ VariantID *string }
			if tx.Table("order_line_items").Select("variant_id").Where("id=? AND deleted_at IS NULL", l.ItemID).Take(&item).Error != nil {
				return ErrUnavailable
			}
			var managed int64
			if item.VariantID != nil && tx.Table("device_variant_tracking").Where("variant_id=? AND deleted_at IS NULL", *item.VariantID).Count(&managed).Error != nil {
				return ErrUnavailable
			}
			shipped, e := orders.AfterSalesShippedTx(ctx, tx, orderID, l.ItemID)
			if e != nil {
				return e
			}
			unshippedReturn := in.Type == "return" && len(l.DeviceIDs) == 0 && requested.Cmp(new(big.Rat).Sub(available, shipped)) <= 0
			if managed > 0 && !unshippedReturn {
				if len(l.DeviceIDs) != l.Quantity {
					return ErrInvalid
				}
				var assigned []struct{ DeviceID string }
				if tx.Table("device_assignments").Select("device_id").Where("order_id=? AND item_id=? AND status='shipped' AND deleted_at IS NULL AND device_id IN ?", orderID, l.ItemID, l.DeviceIDs).Scan(&assigned).Error != nil {
					return ErrUnavailable
				}
				present := map[string]bool{}
				for _, a := range assigned {
					present[a.DeviceID] = true
				}
				if len(present) != l.Quantity {
					return ErrInvalid
				}
			} else if managed == 0 && len(l.DeviceIDs) > 0 {
				return ErrInvalid
			}
			l.Title = titles[l.ItemID]
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		out = AfterSalesRequest{ID: "asr_" + uuid.NewString(), Type: in.Type, Status: "pending", Reason: in.Reason, Items: lines, CreatedAt: now}
		items, _ := json.Marshal(struct {
			Lines []AfterSalesLine `json:"lines"`
		}{lines})
		if tx.Table("after_sales_requests").Create(map[string]any{"id": out.ID, "order_id": orderID, "customer_id": account, "type": out.Type, "status": out.Status, "reason": out.Reason, "items": string(items), "created_at": now, "updated_at": now}).Error != nil {
			return ErrUnavailable
		}
		result, _ := json.Marshal(out)
		if tx.Table("customer_after_sales_operations").Create(map[string]any{"operation_id": in.OperationID, "customer_id": account, "order_id": orderID, "request_id": out.ID, "request_json": string(encoded), "result_json": string(result), "created_at": now}).Error != nil {
			return ErrUnavailable
		}
		return tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'customer.after_sales.create',?,'{}',?)", "after-sales_"+in.OperationID, account, out.ID, now).Error
	})
	if e != nil {
		return AfterSalesRequest{}, e
	}
	return out, nil
}
