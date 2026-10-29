package cart

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Confirmation is private. No metadata, source account ID, provider payload,
// payment locator or raw envelope is returned. Email never grants ownership.
type Confirmation struct {
	ID              string              `json:"id"`
	DisplayID       string              `json:"display_id"`
	CustomDisplayID string              `json:"custom_display_id"`
	Status          string              `json:"status"`
	Email           string              `json:"email"`
	Locale          string              `json:"locale"`
	CreatedAt       time.Time           `json:"created_at"`
	Shipping        ContactAddress      `json:"shipping_address"`
	Billing         ContactAddress      `json:"billing_address"`
	Items           []ViewItem          `json:"items"`
	Amounts         orders.AmountDetail `json:"amounts"`
	Payment         ConfirmationPayment `json:"payment"`
}
type ConfirmationPayment struct {
	ProviderID   string    `json:"provider_id"`
	Amount       string    `json:"amount"`
	CurrencyCode string    `json:"currency_code"`
	CreatedAt    time.Time `json:"created_at"`
}

// ReadConfirmation authorizes a newly placed anonymous order via the current
// cart possession AND durable completion receipt, in one SQL transaction. It
// does not search by email or accept old account/order identifiers as authority.
func ReadConfirmation(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, orderID string) (Confirmation, error) {
	return readConfirmation(ctx, db, store, ref, orderID, nil)
}

// WithConfirmation authorizes the same current cart and paid receipt as the
// private confirmation view, then runs one local ownership write atomically.
// Callers must reauthorize the customer session inside action; no remote I/O.
func WithConfirmation(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, orderID string, action func(*gorm.DB) error) error {
	if action == nil {
		return ErrInput
	}
	_, err := readConfirmation(ctx, db, store, ref, orderID, action)
	return err
}
func readConfirmation(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, orderID string, action func(*gorm.DB) error) (Confirmation, error) {
	failed := Confirmation{}
	if db == nil || db.Statement == nil || !refOK(store, ref) || !idPattern.MatchString(orderID) {
		return failed, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return failed, ErrInput
	}
	var out Confirmation
	lock := "SHARE"
	if action != nil {
		lock = "UPDATE"
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		if orders.Ready(ctx, tx) != nil {
			return ErrSchema
		}
		var gate struct {
			Version int
			Enabled bool
		}
		r := tx.Raw("SELECT version,enabled FROM cart_completion_runtime WHERE id=1 FOR SHARE").Scan(&gate)
		if r.Error != nil || r.RowsAffected != 1 || gate.Version != 1 || !gate.Enabled {
			return ErrSchema
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		var owner struct {
			StoreID, Generation string
			Version             uint64
			Active              bool
			CheckoutAttemptID   *string
		}
		r = tx.Table("cart_ownership").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: lock}).Take(&owner)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrDenied
		}
		if r.Error != nil {
			return ErrStorage
		}
		if !owner.Active || owner.StoreID != store || subtle.ConstantTimeCompare([]byte(owner.Generation), []byte(ref.Generation)) != 1 || owner.CheckoutAttemptID == nil {
			return ErrDenied
		}
		var receipt struct {
			OrderID, OperationID, Generation, PaymentID, CaptureID string
			CompletedVersion                                       uint64
		}
		r = tx.Table("cart_completion_receipts").Where("cart_id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrDenied
		}
		if r.Error != nil {
			return ErrStorage
		}
		if receipt.OrderID != orderID || receipt.OperationID != *owner.CheckoutAttemptID || receipt.Generation != ref.Generation || receipt.CompletedVersion != owner.Version {
			return ErrDenied
		}
		var cart struct {
			SalesChannelID         string
			CompletedAt, DeletedAt *time.Time
			CustomerID             *string
		}
		if tx.Table("carts").Select("sales_channel_id,completed_at,deleted_at,customer_id").Where("id=?", ref.CartID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&cart).Error != nil {
			return ErrStorage
		}
		if cart.SalesChannelID != channel || cart.CompletedAt == nil || cart.DeletedAt != nil || cart.CustomerID != nil {
			return ErrDenied
		}
		var saved struct {
			ID, DisplayID, CustomDisplayID, Status, Email, CurrencyCode, Locale, SalesChannelID string
			ShippingAddressID, BillingAddressID, CustomerID                                     *string
			CreatedAt                                                                           time.Time
			IsDraftOrder                                                                        bool
		}
		r = tx.Table("orders").Select("id,CAST(display_id AS CHAR) AS display_id,custom_display_id,status,email,currency_code,locale,sales_channel_id,shipping_address_id,billing_address_id,customer_id,created_at,is_draft_order").Where("id=? AND deleted_at IS NULL", orderID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&saved)
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return ErrDenied
		}
		if r.Error != nil {
			return ErrStorage
		}
		if saved.CustomerID != nil || saved.IsDraftOrder || saved.SalesChannelID != channel || saved.ShippingAddressID == nil || saved.BillingAddressID == nil {
			return ErrDenied
		}
		if !viewText(saved.Email) || saved.Email == "" || len(saved.Email) > 254 {
			return ErrSchema
		}
		var linked int64
		if tx.Table("order_carts").Where("cart_id=? AND order_id=? AND deleted_at IS NULL", ref.CartID, orderID).Count(&linked).Error != nil || linked != 1 {
			return ErrSchema
		}
		out = Confirmation{ID: orderID, DisplayID: saved.DisplayID, CustomDisplayID: saved.CustomDisplayID, Status: saved.Status, Email: saved.Email, Locale: saved.Locale, CreatedAt: saved.CreatedAt, Items: []ViewItem{}}
		for _, address := range []struct {
			id     string
			target *ContactAddress
		}{{*saved.ShippingAddressID, &out.Shipping}, {*saved.BillingAddressID, &out.Billing}} {
			if tx.Table("order_addresses").Select("first_name,last_name,company,COALESCE(address_1,'') AS address1,COALESCE(address_2,'') AS address2,city,country_code AS country,province,postal_code,phone").Where("id=? AND deleted_at IS NULL", address.id).Take(address.target).Error != nil {
				return ErrSchema
			}
			if !validContactAddress(*address.target) {
				return ErrSchema
			}
		}
		out.Amounts, err = orders.ReadAmountSnapshot(tx, orderID)
		if err != nil {
			return err
		}
		var rows []struct {
			ViewItem
			VariantOptionValues *string
		}
		if tx.Table("order_line_items l").Select("l.id,l.title,l.product_id,l.product_title,l.product_handle,l.thumbnail,l.variant_title,l.variant_option_values").Joins("JOIN order_items i ON i.item_id=l.id AND i.deleted_at IS NULL").Where("i.order_id=? AND i.version=? AND l.deleted_at IS NULL", orderID, out.Amounts.Version).Order("l.created_at DESC,l.id").Limit(1001).Scan(&rows).Error != nil || len(rows) != len(out.Amounts.Lines) || len(rows) == 0 {
			return ErrSchema
		}
		wanted := map[string]bool{}
		for _, line := range out.Amounts.Lines {
			wanted[line.ItemID] = true
		}
		for _, row := range rows {
			if !wanted[row.ID] || !viewText(row.Title) {
				return ErrSchema
			}
			delete(wanted, row.ID)
			for _, v := range []*string{row.ProductTitle, row.ProductHandle, row.Thumbnail, row.VariantTitle} {
				if v != nil && !viewText(*v) {
					return ErrSchema
				}
			}
			row.OptionValues = map[string]string{}
			if row.VariantOptionValues != nil {
				if len(*row.VariantOptionValues) > 32768 || json.Unmarshal([]byte(*row.VariantOptionValues), &row.OptionValues) != nil || row.OptionValues == nil || len(row.OptionValues) > 64 {
					return ErrSchema
				}
				for k, v := range row.OptionValues {
					if !viewText(k) || !viewText(v) {
						return ErrSchema
					}
				}
			}
			out.Items = append(out.Items, row.ViewItem)
		}
		var pay struct {
			ProviderID, Amount, CurrencyCode string
			RawAmount                        *string
			CreatedAt                        time.Time
			CapturedAt                       *time.Time
		}
		if tx.Table("payments p").Select("p.provider_id,CAST(p.amount AS CHAR) AS amount,p.raw_amount,p.currency_code,p.created_at,p.captured_at").Joins("JOIN payment_captures c ON c.payment_id=p.id AND c.id=? AND c.deleted_at IS NULL", receipt.CaptureID).Joins("JOIN order_payment_collections l ON l.payment_collection_id=p.payment_collection_id AND l.order_id=? AND l.deleted_at IS NULL", orderID).Where("p.id=? AND p.deleted_at IS NULL", receipt.PaymentID).Take(&pay).Error != nil || pay.CapturedAt == nil || pay.CurrencyCode != saved.CurrencyCode {
			return ErrSchema
		}
		number, err := nonNegativeAmount(pay.Amount, pay.RawAmount)
		if err != nil {
			return err
		}
		amount, err := totals.Resolve(number)
		if err != nil {
			return ErrSchema
		}
		out.Payment = ConfirmationPayment{pay.ProviderID, amount, pay.CurrencyCode, pay.CreatedAt}
		encoded, err := json.Marshal(out)
		if err != nil || len(encoded) > 4*1024*1024 {
			return ErrSchema
		}
		if action != nil {
			return action(tx)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return failed, sanitize(err)
	}
	return out, nil
}
