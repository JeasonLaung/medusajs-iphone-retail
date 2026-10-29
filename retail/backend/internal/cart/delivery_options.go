package cart

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeliveryOption struct {
	ID                    string `json:"id"`
	ProfileID             string `json:"profile_id"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Kind                  string `json:"kind"`
	Amount                string `json:"amount"`
	TaxInclusive          bool   `json:"is_tax_inclusive"`
	InsufficientInventory bool   `json:"insufficient_inventory"`
}
type DeliveryOptions struct {
	CartID   string           `json:"cart_id"`
	Version  string           `json:"version"`
	Currency string           `json:"currency_code"`
	Options  []DeliveryOption `json:"options"`
	Selected []string         `json:"selected_option_ids"`
	Complete bool             `json:"complete"`
}

// Resolves and projects options in a single current transaction. Helpers use
// locking reads for the same cart/warehouse facts as saving; no data is changed
// and stock observations do not constitute a reservation or permission to pay.
func ReadDeliveryOptions(ctx context.Context, db *gorm.DB, store string, ref sessionstore.CartReference, manualProviders []string) (DeliveryOptions, error) {
	if db == nil || db.Statement == nil || !refOK(store, ref) {
		return DeliveryOptions{}, ErrInput
	}
	if _, nested := db.Statement.ConnPool.(gorm.TxCommitter); nested {
		return DeliveryOptions{}, ErrInput
	}
	var out DeliveryOptions
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := commandReady(ctx, tx, "cart.access"); err != nil {
			return err
		}
		channel, err := storeChannel(ctx, tx, store)
		if err != nil {
			return err
		}
		current, err := lockCurrent(ctx, tx, store, channel, ref, "UPDATE")
		if err != nil {
			return err
		}
		if err := shippingContactReady(tx, current); err != nil {
			return err
		}
		at := time.Now().UTC()
		groups, err := ReadManualShippingQuotes(ctx, tx, store, ref, current.Version, manualProviders, at)
		if err != nil {
			return err
		}
		out = DeliveryOptions{CartID: current.ID, Version: current.Version, Currency: current.CurrencyCode, Options: []DeliveryOption{}, Selected: []string{}}
		var saved []string
		if tx.Table("cart_shipping_methods").Select("shipping_option_id").Where("cart_id=? AND deleted_at IS NULL AND shipping_option_id IS NOT NULL", current.ID).Order("created_at,id").Clauses(clause.Locking{Strength: "SHARE"}).Limit(101).Scan(&saved).Error != nil || len(saved) > 100 {
			return ErrSchema
		}
		byProfile := map[string]string{}
		for _, group := range groups {
			for _, quote := range group.Quotes {
				for _, id := range saved {
					if id == quote.Option.ID {
						if byProfile[group.Group.ProfileID] != "" {
							return ErrSchema
						}
						byProfile[group.Group.ProfileID] = id
						out.Selected = append(out.Selected, id)
					}
				}
			}
		}
		for _, group := range groups {
			for _, quote := range group.Quotes {
				effective := []string{quote.Option.ID}
				for _, other := range groups {
					if other.Group.ProfileID != group.Group.ProfileID && byProfile[other.Group.ProfileID] != "" {
						effective = append(effective, byProfile[other.Group.ProfileID])
					}
				}
				_, err := readManualShippingSelection(ctx, tx, store, ref, current.Version, effective, manualProviders, at, false)
				insufficient := errors.Is(err, ErrItemStock)
				if err != nil && !insufficient {
					return err
				}
				var presentation struct{ Kind, Description string }
				result := tx.Table("fulfillment_shipping_options o").Select("f.type AS kind,COALESCE(t.description,'') AS description").Joins("JOIN fulfillment_service_zones z ON z.id=o.service_zone_id AND z.deleted_at IS NULL").Joins("JOIN fulfillment_sets f ON f.id=z.fulfillment_set_id AND f.deleted_at IS NULL").Joins("JOIN fulfillment_option_types t ON t.id=o.shipping_option_type_id AND t.deleted_at IS NULL").Where("o.id=? AND o.deleted_at IS NULL", quote.Option.ID).Clauses(clause.Locking{Strength: "SHARE"}).Scan(&presentation)
				if result.Error != nil || result.RowsAffected != 1 || !viewText(quote.Option.Name) || !viewText(presentation.Description) {
					return ErrSchema
				}
				kind := "shipping"
				if presentation.Kind == "pickup" {
					kind = "pickup"
				}
				out.Options = append(out.Options, DeliveryOption{ID: quote.Option.ID, ProfileID: group.Group.ProfileID, Name: quote.Option.Name, Description: presentation.Description, Kind: kind, Amount: quote.Price.Amount, TaxInclusive: quote.Price.CalculatedTaxInclusive, InsufficientInventory: insufficient})
				if len(out.Options) > 100 {
					return ErrSchema
				}
			}
		}
		selection, err := ReadSelectedManualShipping(ctx, tx, store, ref, current.Version, out.Selected, manualProviders, at)
		if err == nil {
			out.Complete = selection.Complete
		} else if !errors.Is(err, ErrItemStock) && !errors.Is(err, ErrItemUnavailable) {
			return err
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return DeliveryOptions{}, sanitize(err)
	}
	return out, nil
}
