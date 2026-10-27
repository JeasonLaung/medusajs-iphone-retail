package catalogimport

import (
	"context"
	"database/sql"
	"math/big"
	"testing"

	cataloghttp "github.com/eurorefurb/retail/internal/catalog"
)

// This oracle reads imported facts independently: SQL reconciles each location,
// and rational arithmetic checks pooled and single-warehouse capacity. It does
// not call the production inventory reader or its arithmetic helpers.
func checkActualAvailability(t *testing.T, ctx context.Context, db *sql.DB, channel string, v cataloghttp.StoreVariant, counts map[string]int64) {
	t.Helper()
	state, can := "unavailable", false
	var pooled, maximum *big.Int
	if !v.ConfigurationComplete {
		state = "incomplete"
	} else if !v.ManageInventory {
		state, can = "untracked", true
	} else {
		rows, err := db.QueryContext(ctx, `SELECT vi.inventory_item_id,vi.required_quantity,i.id,l.id,
 CASE WHEN lev.id IS NOT NULL AND lev.stocked_quantity>=0 AND lev.reserved_quantity>=0
 AND lev.reserved_quantity=COALESCE(r.total,0) AND COALESCE(r.bad,0)=0 AND COALESCE(r.n,0)<=10000
 AND NOT EXISTS(SELECT 1 FROM inventory_migration_holds h WHERE h.inventory_item_id=i.id AND h.location_id=l.id)
 THEN GREATEST(lev.stocked_quantity-lev.reserved_quantity,0) ELSE NULL END
 FROM catalog_variant_inventory_items vi
 LEFT JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL
 LEFT JOIN (commerce_channel_locations cl JOIN inventory_locations l ON l.id=cl.stock_location_id AND l.deleted_at IS NULL)
 ON cl.sales_channel_id=? AND cl.deleted_at IS NULL
 LEFT JOIN inventory_levels lev ON lev.inventory_item_id=i.id AND lev.location_id=l.id AND lev.deleted_at IS NULL
 LEFT JOIN (SELECT inventory_item_id,location_id,SUM(quantity) total,SUM(quantity<0) bad,COUNT(*) n
 FROM inventory_reservations WHERE deleted_at IS NULL GROUP BY inventory_item_id,location_id) r
 ON r.inventory_item_id=i.id AND r.location_id=l.id
 WHERE vi.variant_id=? AND vi.deleted_at IS NULL ORDER BY vi.inventory_item_id,l.id`, channel, v.ID)
		if err != nil {
			t.Fatal("actual availability oracle query failed")
		}
		type requirement struct {
			required  *big.Rat
			locations map[string]*big.Rat
			total     *big.Rat
		}
		reqs := map[string]*requirement{}
		valid := true
		for rows.Next() {
			var id, required string
			var item, location, amount sql.NullString
			if rows.Scan(&id, &required, &item, &location, &amount) != nil {
				rows.Close()
				t.Fatal("actual availability oracle scan failed")
			}
			r := reqs[id]
			if r == nil {
				n, ok := new(big.Rat).SetString(required)
				if !ok || n.Sign() <= 0 {
					valid = false
					n = new(big.Rat)
				}
				r = &requirement{n, map[string]*big.Rat{}, new(big.Rat)}
				reqs[id] = r
			}
			if !item.Valid || !location.Valid {
				valid = false
			}
			if amount.Valid {
				n, ok := new(big.Rat).SetString(amount.String)
				if !ok || n.Sign() < 0 {
					rows.Close()
					t.Fatal("actual availability oracle amount failed")
				}
				r.locations[location.String] = n
				r.total.Add(r.total, n)
			}
		}
		if rows.Err() != nil {
			rows.Close()
			t.Fatal("actual availability oracle rows failed")
		}
		rows.Close()
		if valid && len(reqs) > 0 {
			common := map[string]*big.Int{}
			first := true
			floor := func(a, b *big.Rat) *big.Int { q := new(big.Rat).Quo(a, b); return new(big.Int).Quo(q.Num(), q.Denom()) }
			for _, r := range reqs {
				n := floor(r.total, r.required)
				if pooled == nil || n.Cmp(pooled) < 0 {
					pooled = n
				}
				if first {
					for loc, a := range r.locations {
						common[loc] = floor(a, r.required)
					}
					first = false
				} else {
					for loc, current := range common {
						a, ok := r.locations[loc]
						if !ok {
							delete(common, loc)
						} else if n := floor(a, r.required); n.Cmp(current) < 0 {
							common[loc] = n
						}
					}
				}
			}
			maximum = new(big.Int)
			for _, n := range common {
				if n.Cmp(maximum) > 0 {
					maximum.Set(n)
				}
			}
			if len(common) > 0 {
				if maximum.Sign() > 0 {
					state, can = "available", true
				} else if v.AllowBackorder {
					state, can = "backorder", true
				} else {
					state = "out_of_stock"
				}
			}
		}
	}
	matches := func(value *string, expected *big.Int) bool {
		return value == nil && expected == nil || value != nil && expected != nil && *value == expected.String()
	}
	if v.Availability.State != state || v.Availability.CanFulfill != can || !matches(v.InventoryQuantity, pooled) || !matches(v.Availability.MaxFulfillableQuantity, maximum) {
		t.Fatal("actual public availability differs from independent relational oracle")
	}
	counts["http_public_actual_availability_variants"]++
	counts["http_public_actual_availability_"+state]++
}
