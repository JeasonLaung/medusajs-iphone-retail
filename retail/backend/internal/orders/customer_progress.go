package orders

import (
	"context"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

type CustomerProgress struct {
	OrderID      string                  `json:"order_id"`
	CurrencyCode string                  `json:"currency_code"`
	Events       []CustomerProgressEvent `json:"events"`
	Shipments    []CustomerShipment      `json:"shipments"`
}
type CustomerProgressEvent struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	OccurredAt time.Time `json:"occurred_at"`
	Amount     *string   `json:"amount,omitempty"`
}
type CustomerShipment struct {
	ID          string                  `json:"id"`
	ShippedAt   time.Time               `json:"shipped_at"`
	DeliveredAt *time.Time              `json:"delivered_at"`
	Labels      []CustomerTrackingLabel `json:"labels"`
}
type CustomerTrackingLabel struct {
	TrackingNumber string `json:"tracking_number"`
	TrackingURL    string `json:"tracking_url"`
}

// ReadCustomerProgressTx requires a caller-owned transaction AFTER explicit
// customer_order_ownership verification. It projects facts only: no provider
// payloads, shipping labels, warehouse IDs, actor IDs or device event data.
func ReadCustomerProgressTx(ctx context.Context, tx *gorm.DB, orderID string) (CustomerProgress, error) {
	out := CustomerProgress{OrderID: orderID, Events: []CustomerProgressEvent{}, Shipments: []CustomerShipment{}}
	if tx == nil || tx.Statement == nil {
		return out, ErrSchema
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, ErrSchema
	}
	if Ready(ctx, tx) != nil || fulfillment.Ready(ctx, tx) != nil || payments.Ready(ctx, tx) != nil {
		return out, ErrSchema
	}
	var order struct {
		CurrencyCode string
		CreatedAt    time.Time
		CanceledAt   *time.Time
	}
	if tx.Table("orders").Select("currency_code,created_at,canceled_at").Where("id=? AND deleted_at IS NULL", orderID).Take(&order).Error != nil {
		return out, ErrSchema
	}
	out.CurrencyCode = order.CurrencyCode
	add := func(id, kind string, at *time.Time, amount *string) {
		if at != nil && !at.IsZero() {
			out.Events = append(out.Events, CustomerProgressEvent{ID: id, Kind: kind, OccurredAt: at.UTC(), Amount: amount})
		}
	}
	add(orderID+":placed", "placed", &order.CreatedAt, nil)
	add(orderID+":canceled", "canceled", order.CanceledAt, nil)
	var parcels []struct {
		ID                                 string
		CreatedAt                          time.Time
		ShippedAt, DeliveredAt, CanceledAt *time.Time
	}
	if tx.Table("fulfillments f").Select("f.id,f.created_at,f.shipped_at,f.delivered_at,f.canceled_at").Joins("JOIN order_fulfillments l ON l.fulfillment_id=f.id AND l.deleted_at IS NULL").Where("l.order_id=? AND f.deleted_at IS NULL", orderID).Order("f.created_at,f.id").Limit(101).Scan(&parcels).Error != nil || len(parcels) > 100 {
		return out, ErrSchema
	}
	for _, p := range parcels {
		// A shared parcel would disclose another customer's shipment.
		var links int64
		if tx.Table("order_fulfillments").Where("fulfillment_id=? AND deleted_at IS NULL", p.ID).Count(&links).Error != nil || links != 1 {
			return out, ErrSchema
		}
		add(p.ID+":packed", "packed", &p.CreatedAt, nil)
		add(p.ID+":shipped", "shipped", p.ShippedAt, nil)
		add(p.ID+":delivered", "delivered", p.DeliveredAt, nil)
		add(p.ID+":canceled", "fulfillment_canceled", p.CanceledAt, nil)
		if p.CanceledAt != nil || p.ShippedAt == nil {
			continue
		}
		shipment := CustomerShipment{ID: p.ID, ShippedAt: p.ShippedAt.UTC(), DeliveredAt: p.DeliveredAt, Labels: []CustomerTrackingLabel{}}
		if tx.Table("fulfillment_labels").Select("tracking_number,tracking_url").Where("fulfillment_id=? AND deleted_at IS NULL", p.ID).Order("created_at,id").Limit(101).Scan(&shipment.Labels).Error != nil || len(shipment.Labels) > 100 {
			return out, ErrSchema
		}
		for i := range shipment.Labels {
			l := &shipment.Labels[i]
			if !utf8.ValidString(l.TrackingNumber) || utf8.RuneCountInString(l.TrackingNumber) > 200 || strings.ContainsFunc(l.TrackingNumber, unicode.IsControl) {
				return out, ErrSchema
			}
			l.TrackingURL = customerTrackingURL(l.TrackingURL)
		}
		out.Shipments = append(out.Shipments, shipment)
	}
	var events []struct {
		ID, Action string
		CreatedAt  time.Time
	}
	if tx.Table("device_events").Select("id,action,created_at").Where("order_id=? AND deleted_at IS NULL AND action IN ?", orderID, []string{"assign", "release", "replace_allocation"}).Order("created_at,id").Limit(1001).Scan(&events).Error != nil || len(events) > 1000 {
		return out, ErrSchema
	}
	for _, event := range events {
		add("device:"+event.ID, map[string]string{"assign": "allocated", "release": "released", "replace_allocation": "reallocated"}[event.Action], &event.CreatedAt, nil)
	}
	var paid []struct {
		ID, Amount, RawAmount string
		CapturedAt            *time.Time
	}
	if tx.Table("payments p").Select("p.id,CAST(p.amount AS CHAR) AS amount,p.raw_amount,p.captured_at").Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.deleted_at IS NULL").Joins("JOIN payment_collections pc ON pc.id=p.payment_collection_id AND pc.deleted_at IS NULL").Where("l.order_id=? AND p.deleted_at IS NULL AND p.currency_code=? AND pc.currency_code=?", orderID, out.CurrencyCode, out.CurrencyCode).Order("p.created_at,p.id").Limit(101).Scan(&paid).Error != nil || len(paid) > 100 {
		return out, ErrSchema
	}
	for _, payment := range paid {
		for _, movement := range []struct{ table, kind string }{{"payment_captures", "paid"}, {"payment_refunds", "refund"}} {
			var rows []struct {
				ID, Amount, RawAmount string
				CreatedAt             time.Time
			}
			if tx.Table(movement.table).Select("id,CAST(amount AS CHAR) AS amount,raw_amount,created_at").Where("payment_id=? AND deleted_at IS NULL", payment.ID).Order("created_at,id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
				return out, ErrSchema
			}
			for _, row := range rows {
				amount, e := requiredPaymentAmount(row.Amount, row.RawAmount)
				if e != nil {
					return out, e
				}
				add(movement.kind+":"+row.ID, movement.kind, &row.CreatedAt, &amount)
			}
			if movement.kind == "paid" && len(rows) == 0 && payment.CapturedAt != nil {
				amount, e := requiredPaymentAmount(payment.Amount, payment.RawAmount)
				if e != nil {
					return out, e
				}
				add("paid:"+payment.ID, "paid", payment.CapturedAt, &amount)
			}
		}
	}
	if len(out.Events) > 2000 {
		return CustomerProgress{}, ErrSchema
	}
	sort.SliceStable(out.Events, func(i, j int) bool { return out.Events[i].OccurredAt.Before(out.Events[j].OccurredAt) })
	return out, nil
}

// Historical tracking links are untrusted input. Keep the number visible while
// suppressing unsafe schemes, credentials and control characters.
func customerTrackingURL(raw string) string {
	if len(raw) > 2048 || !utf8.ValidString(raw) || strings.ContainsFunc(raw, unicode.IsControl) || strings.TrimSpace(raw) != raw {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return raw
}
