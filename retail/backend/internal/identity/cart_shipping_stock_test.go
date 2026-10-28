package identity_test

import (
	"database/sql"
	"testing"

	"github.com/eurorefurb/retail/internal/inventory"
	"gorm.io/gorm"
)

func checkShippingStockScopes(t *testing.T, db *gorm.DB, variants []inventory.AvailableVariant, lines []inventory.CartStockLine, fingerprint func() [32]byte) {
	t.Helper()
	read := func(scopes map[string][]string, want bool) {
		t.Helper()
		before := fingerprint()
		var out inventory.CartStockPlan
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			out, err = inventory.ReadCartStockAtLocations(tx, variants, lines, "public-main", scopes)
			return err
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil || out.CanFulfill != want {
			t.Fatalf("SQL shipping warehouses differ: %s %v", out.State, err)
		}
		if fingerprint() != before {
			t.Fatal("shipping observation changed stock")
		}
		for _, assignment := range out.Assignments {
			found := false
			for _, location := range scopes[assignment.LineItemID] {
				if location == assignment.LocationID {
					found = true
				}
			}
			if !found {
				t.Fatal("SQL assignment escaped permitted warehouse")
			}
		}
	}
	read(map[string][]string{"one": {"availability-a"}, "two": {"availability-b"}}, true)
	read(map[string][]string{"one": {"availability-a"}, "two": {"availability-a"}}, false)
	read(map[string][]string{"one": {}, "two": {"availability-b"}}, false)
	read(map[string][]string{"one": {"availability-foreign"}, "two": {"availability-b"}}, false)
	if _, err := inventory.ReadCartStockAtLocations(db, variants, lines, "public-main", map[string][]string{"one": {"availability-a"}, "two": {"availability-b"}}); err != inventory.ErrCartStockInput {
		t.Fatal("autocommit restricted stock accepted")
	}
	t.Log(`CART_SHIPPING_STOCK_REHEARSAL {"per_line_warehouse_restrictions":true,"shared_capacity_across_groups":true,"foreign_and_empty_denied":true,"assignment_cannot_escape":true,"readonly_fingerprint":true,"autocommit_rejected":true,"inventory_reserved":false,"cart_shipping_integrated":false,"checkout":false}`)
}
