package cart

import (
	"context"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SelectedShippingGroup struct {
	Group ShippingGroup
	Quote fulfillment.ManualFlatQuote
}

// ManualShippingSelection is internal, current transaction evidence only.
// Stock assignments are not a reservation or permission to pay.
type ManualShippingSelection struct {
	Groups   []SelectedShippingGroup
	Stock    inventory.CartStockPlan
	Complete bool
}

// ReadSelectedManualShipping re-resolves every selected option, requires one
// available quote for each current profile, and observes shared stock across
// all lines in the cart. The browser may name option IDs, never profiles,
// warehouses, amounts or provider registrations. Caller owns the transaction
// and must revalidate/reserve and persist all other refresh phases atomically.
func ReadSelectedManualShipping(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, optionIDs, manualProviderIDs []string, at time.Time) (ManualShippingSelection, error) {
	return readManualShippingSelection(ctx, tx, store, ref, version, optionIDs, manualProviderIDs, at, true)
}

// In-progress selection may omit profiles. Such lines still compete for
// channel stock, but the result must not claim complete shipping coverage.
func readManualShippingSelection(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, optionIDs, manualProviderIDs []string, at time.Time, requireComplete bool) (ManualShippingSelection, error) {
	if len(optionIDs) > 1000 {
		return ManualShippingSelection{}, ErrInput
	}
	selected := map[string]bool{}
	for _, id := range optionIDs {
		if !idPattern.MatchString(id) || selected[id] {
			return ManualShippingSelection{}, ErrInput
		}
		selected[id] = true
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return ManualShippingSelection{}, err
	}
	quotes, err := ReadManualShippingQuotes(ctx, tx, store, ref, version, manualProviderIDs, at)
	if err != nil {
		return ManualShippingSelection{}, err
	}
	result := ManualShippingSelection{Groups: []SelectedShippingGroup{}}
	allowed := map[string][]string{}
	matched := 0
	for _, group := range quotes {
		var choice *fulfillment.ManualFlatQuote
		for i := range group.Quotes {
			if selected[group.Quotes[i].Option.ID] {
				if choice != nil {
					return ManualShippingSelection{}, ErrItemUnavailable
				}
				choice = &group.Quotes[i]
			}
		}
		if choice == nil {
			if requireComplete {
				return ManualShippingSelection{}, ErrItemUnavailable
			}
			continue
		}
		matched++
		for _, line := range group.Group.Lines {
			allowed[line.ID] = choice.Option.LocationIDs
		}
		result.Groups = append(result.Groups, SelectedShippingGroup{Group: group.Group, Quote: *choice})
	}
	if matched != len(selected) {
		return ManualShippingSelection{}, ErrItemUnavailable
	}
	result.Complete = matched == len(quotes)
	current := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
	var saved []struct {
		ID               string
		VariantID        *string
		Quantity         int64
		RequiresShipping bool
	}
	if current.Table("cart_line_items").Select("id,variant_id,quantity,requires_shipping").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Limit(1001).Scan(&saved).Error != nil || len(saved) > 1000 {
		return ManualShippingSelection{}, ErrSchema
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, line := range saved {
		if !idPattern.MatchString(line.ID) || line.Quantity < 1 || line.Quantity > 1000000000 {
			return ManualShippingSelection{}, ErrSchema
		}
		if line.VariantID == nil {
			if line.RequiresShipping {
				return ManualShippingSelection{}, ErrItemUnavailable
			}
			continue
		}
		if !seen[*line.VariantID] {
			ids = append(ids, *line.VariantID)
			seen[*line.VariantID] = true
		}
	}
	variants, err := catalog.ReadCartVariants(current, ids, cart.SalesChannelID)
	if err != nil {
		return ManualShippingSelection{}, ErrItemUnavailable
	}
	locations := []string{}
	if current.Table("inventory_locations l").Select("l.id").Joins("JOIN commerce_channel_locations cl ON cl.stock_location_id=l.id AND cl.deleted_at IS NULL").Where("cl.sales_channel_id=? AND l.deleted_at IS NULL", cart.SalesChannelID).Order("l.id").Limit(1001).Scan(&locations).Error != nil || len(locations) > 1000 {
		return ManualShippingSelection{}, ErrSchema
	}
	stockVariants := []inventory.AvailableVariant{}
	stockLines := []inventory.CartStockLine{}
	seen = map[string]bool{}
	for _, line := range saved {
		if line.VariantID == nil {
			continue
		}
		v := variants[*line.VariantID]
		// A digital/untracked line needs no warehouse. Tracked non-shipping
		// lines still compete for shared inventory at channel warehouses.
		if !line.RequiresShipping && !v.ManageInventory {
			continue
		}
		if !seen[v.ID] {
			stockVariants = append(stockVariants, inventory.AvailableVariant{ID: v.ID, ManageInventory: v.ManageInventory, AllowBackorder: v.AllowBackorder, Complete: v.Complete})
			seen[v.ID] = true
		}
		stockLines = append(stockLines, inventory.CartStockLine{ID: line.ID, VariantID: v.ID, Quantity: strconv.FormatInt(line.Quantity, 10)})
		if !line.RequiresShipping {
			allowed[line.ID] = locations
		} else if _, ok := allowed[line.ID]; !ok {
			if requireComplete {
				return ManualShippingSelection{}, ErrItemUnavailable
			}
			allowed[line.ID] = locations
		}
	}
	result.Stock, err = inventory.ReadCartStockAtLocations(current, stockVariants, stockLines, cart.SalesChannelID, allowed)
	if err != nil {
		return ManualShippingSelection{}, err
	}
	if !result.Stock.CanFulfill {
		return ManualShippingSelection{}, ErrItemStock
	}
	return result, nil
}
