package customer

import (
	"context"
	"crypto/subtle"
	"errors"
	"regexp"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrOrderMissing = errors.New("customer order unavailable")
var customerOrderID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,254}$`)

type orderPossession interface {
	ResolveCart(context.Context, string) (sessionstore.CartReference, error)
}

// AttachOrder uses two independent capabilities. Neither email, a legacy
// customer ID nor an order ID by itself grants account ownership.
func (s *Store) AttachOrder(ctx context.Context, token, possession, orderID string) error {
	if !tokenOK(token) || !tokenOK(possession) {
		return ErrUnauthorized
	}
	if !customerOrderID.MatchString(orderID) {
		return ErrInvalid
	}
	if !s.ready() {
		return ErrUnavailable
	}
	resolver, ok := s.sessions.(orderPossession)
	if !ok {
		return ErrUnavailable
	}
	principal, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
	if err != nil {
		return customerSessionError(err)
	}
	ref, err := resolver.ResolveCart(ctx, possession)
	if err != nil {
		return customerSessionError(err)
	}
	var commandErr error
	err = cart.WithConfirmation(ctx, s.db, ref.StoreID, ref, orderID, func(tx *gorm.DB) error {
		var account record
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", principal.UserID).Take(&account).Error; e != nil {
			commandErr = ErrUnavailable
			if errors.Is(e, gorm.ErrRecordNotFound) {
				commandErr = ErrUnauthorized
			}
			return commandErr
		}
		fresh, e := s.sessions.Resolve(ctx, sessionstore.Customer, token)
		if e != nil {
			commandErr = customerSessionError(e)
			return commandErr
		}
		current, e := resolver.ResolveCart(ctx, possession)
		if e != nil {
			commandErr = customerSessionError(e)
			return commandErr
		}
		if fresh.UserID != account.ID || current != ref || !account.Enabled || account.VerifiedAt == nil || account.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(fresh.Version), []byte(version(account))) != 1 {
			commandErr = ErrUnauthorized
			return commandErr
		}
		var existing struct{ CustomerID, CartID, StoreID string }
		e = tx.Table("customer_order_ownership").Where("order_id=?", orderID).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&existing).Error
		if e == nil {
			if existing.CustomerID != account.ID || existing.CartID != ref.CartID || existing.StoreID != ref.StoreID {
				commandErr = ErrOrderMissing
				return commandErr
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES(?,?,?,?,?)", orderID, account.ID, ref.CartID, ref.StoreID, time.Now().UTC()).Error != nil {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO audit_events(id,actor_id,action,resource_id,data,created_at) VALUES(?,?,'customer.order.attach',?,'{}',UTC_TIMESTAMP(6))", "customer-order_"+orderID, account.ID, orderID).Error != nil {
			return ErrUnavailable
		}
		return nil
	})
	if commandErr != nil {
		return commandErr
	}
	if errors.Is(err, cart.ErrDenied) {
		return ErrOrderMissing
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

// OrderSummary excludes contact addresses, source identities and payment data.
type OrderSummary struct {
	ID              string             `json:"id"`
	DisplayID       string             `json:"display_id"`
	CustomDisplayID *string            `json:"custom_display_id"`
	Status          string             `json:"status"`
	CreatedAt       time.Time          `json:"created_at"`
	CurrencyCode    string             `json:"currency_code"`
	Total           string             `json:"total"`
	Items           []OrderSummaryItem `json:"items" gorm:"-"`
}
type OrderSummaryItem struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Thumbnail *string `json:"thumbnail"`
	Quantity  string  `json:"quantity" gorm:"-"`
}
type OrderList struct {
	Orders []OrderSummary `json:"orders"`
	Count  int64          `json:"count"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func (s *Store) Orders(ctx context.Context, token string, offset int) (OrderList, error) {
	out := OrderList{Orders: []OrderSummary{}, Limit: 20, Offset: offset}
	if offset < 0 || offset > 100000 {
		return out, ErrInvalid
	}
	err := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		query := func() *gorm.DB {
			return tx.Table("customer_order_ownership a").Joins("JOIN orders o ON o.id=a.order_id AND o.deleted_at IS NULL AND o.is_draft_order=FALSE").Where("a.customer_id=?", account)
		}
		if query().Count(&out.Count).Error != nil {
			return ErrUnavailable
		}
		if query().Select("o.id,CAST(o.display_id AS CHAR) AS display_id,o.custom_display_id,o.status,o.created_at,o.currency_code").Order("o.created_at DESC,o.id DESC").Limit(20).Offset(offset).Scan(&out.Orders).Error != nil {
			return ErrUnavailable
		}
		for i := range out.Orders {
			item := &out.Orders[i]
			amount, e := orders.ReadAmountSnapshot(tx, item.ID)
			if e != nil || amount.CurrencyCode != item.CurrencyCode {
				return ErrUnavailable
			}
			item.Total = amount.Totals["total"]
			item.Items = []OrderSummaryItem{}
			if tx.Table("order_line_items l").Select("l.id,l.title,l.thumbnail").Joins("JOIN order_items i ON i.item_id=l.id AND i.deleted_at IS NULL").Where("i.order_id=? AND i.version=? AND l.deleted_at IS NULL", item.ID, amount.Version).Order("l.created_at DESC,l.id").Limit(1001).Scan(&item.Items).Error != nil || len(item.Items) != len(amount.Lines) {
				return ErrUnavailable
			}
			quantities := map[string]string{}
			for _, line := range amount.Lines {
				quantities[line.ItemID] = line.Quantity
			}
			for j := range item.Items {
				quantity, ok := quantities[item.Items[j].ID]
				if !ok {
					return ErrUnavailable
				}
				item.Items[j].Quantity = quantity
				delete(quantities, item.Items[j].ID)
			}
		}
		return nil
	})
	if err != nil {
		return OrderList{}, err
	}
	return out, nil
}
