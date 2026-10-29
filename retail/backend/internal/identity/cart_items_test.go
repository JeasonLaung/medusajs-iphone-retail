package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkCartItems(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.WithContext(ctx).Exec(q, args...).Error; err != nil {
			t.Fatalf("owned add-item fixture failed: %v", err)
		}
	}
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at,metadata) VALUES('add-product','Saved product','add-product',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),'{"private":"not-a-snapshot"}')`)
	exec(`INSERT INTO catalog_product_channels(product_id,sales_channel_id,id,created_at,updated_at) VALUES('add-product','channel','add-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,manage_inventory,allow_backorder,created_at,updated_at) VALUES('add-v','add-product','128 GB','00123456789012345678',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('add-shared','add-product','256 GB',NULL,TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_items(id,title,requires_shipping,created_at,updated_at) VALUES('add-item','Inventory',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES('add-location','Warehouse',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_channel_locations(sales_channel_id,stock_location_id,id,created_at,updated_at) VALUES('channel','add-location','add-location-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_inventory_items(variant_id,inventory_item_id,required_quantity,id,created_at,updated_at) VALUES('add-v','add-item',2.5,'add-v-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('add-shared','add-item',2.5,'add-shared-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,created_at,updated_at) VALUES('add-level','add-item','add-location',10,0,999,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('add-set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_price_sets(variant_id,price_set_id,id,created_at,updated_at) VALUES('add-v','add-set','add-v-price',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('add-shared','add-set','add-shared-price',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,min_quantity,max_quantity,created_at,updated_at) VALUES('add-single','add-set','eur',100,'{"value":"100.000000000000000000001","precision":38}',0,NULL,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('add-bulk','add-set','eur',80,'{"value":"80.000000000000000000001","precision":38}',0,2,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("add-item owned scope")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("owned item cart failed")
	}
	issued, err := sessions.IssueCart(ctx, created.Reference)
	if err != nil {
		t.Fatal("owned item possession failed")
	}
	ref, err := sessions.AuthorizeCartWrite(ctx, issued.Token, issued.CSRF)
	if err != nil {
		t.Fatal("owned item CSRF failed")
	}
	in := cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"}
	if _, err := cart.AddItem(ctx, db, "shop", ref, in); err != cart.ErrSchema {
		t.Fatal("disabled item capability allowed mutation")
	}
	exec(`UPDATE cart_item_runtime SET enabled=TRUE WHERE id=1`)
	var wg sync.WaitGroup
	results, errs := make([]cart.AddItemResult, 8), make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = cart.AddItem(ctx, db, "shop", ref, in) }(i)
	}
	wg.Wait()
	fresh := 0
	lineID := ""
	for i, r := range results {
		if errs[i] != nil || r.Version != "2" || r.Quantity != "1" {
			t.Fatalf("concurrent add failed: %v", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
		if lineID != "" && lineID != r.LineID {
			t.Fatal("replay changed item identity")
		}
		lineID = r.LineID
	}
	if fresh != 1 {
		t.Fatal("concurrent add applied more than once")
	}
	fingerprint := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"carts", "cart_ownership", "cart_line_items", "cart_item_operations", "inventory_levels", "inventory_reservations"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("add fingerprint failed")
			}
			b, _ := json.Marshal(rows)
			all = append(all, b)
		}
		b, _ := json.Marshal(all)
		return string(b)
	}
	reject := func(command cart.AddItemCommand, want error) {
		t.Helper()
		before := fingerprint()
		if _, err := cart.AddItem(ctx, db, "shop", ref, command); err != want {
			t.Fatalf("item command error differs: %v", err)
		}
		if before != fingerprint() {
			t.Fatal("rejected add changed durable state")
		}
	}
	changed := in
	changed.Quantity = "2"
	reject(changed, cart.ErrConflict)
	changed.OperationID = uuid.NewString()
	reject(changed, cart.ErrConflict)
	merge := cart.AddItemCommand{OperationID: uuid.NewString(), Version: "2", VariantID: "add-v", Quantity: "1"}
	merged, err := cart.AddItem(ctx, db, "shop", ref, merge)
	if err != nil || merged.LineID != lineID || merged.Quantity != "2" || merged.Version != "3" {
		t.Fatalf("quantity merge failed: %v", err)
	}
	amounts, err := cart.ReadAmounts(ctx, db, "shop", ref)
	if err != nil || len(amounts.Lines) != 1 || amounts.Lines[0].UnitPrice != "80.000000000000000000001" || amounts.Totals["total"] != "160.000000000000000000002" {
		t.Fatal("merged-quantity price selection lost raw precision")
	}
	addShared := cart.AddItemCommand{OperationID: uuid.NewString(), Version: "3", VariantID: "add-shared", Quantity: "1"}
	shared, err := cart.AddItem(ctx, db, "shop", ref, addShared)
	if err != nil || shared.Version != "4" {
		t.Fatalf("shared SKU add failed: %v", err)
	}
	reject(cart.AddItemCommand{OperationID: uuid.NewString(), Version: "4", VariantID: "add-v", Quantity: "2"}, cart.ErrItemStock)
	reject(cart.AddItemCommand{OperationID: uuid.NewString(), Version: "4", VariantID: "not-present", Quantity: "1"}, cart.ErrItemUnavailable)
	// Late SQL receipt rejection must roll back both the quantity and version.
	failed := cart.AddItemCommand{OperationID: "10000000-0000-4000-8000-000000000099", Version: "4", VariantID: "add-v", Quantity: "1"}
	exec(`ALTER TABLE cart_item_operations ADD CONSTRAINT owned_receipt_rejection CHECK(operation_id<>'10000000-0000-4000-8000-000000000099')`)
	reject(failed, cart.ErrStorage)
	exec(`ALTER TABLE cart_item_operations DROP CHECK owned_receipt_rejection`)
	// Different operations racing the same expected version cannot both apply.
	races := []cart.AddItemCommand{failed, failed}
	for i := range races {
		races[i].OperationID = uuid.NewString()
	}
	raceErrors := make([]error, 2)
	for i := range races {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, raceErrors[i] = cart.AddItem(ctx, db, "shop", ref, races[i]) }(i)
	}
	wg.Wait()
	winners, conflicts := 0, 0
	for _, err := range raceErrors {
		if err == nil {
			winners++
		} else if err == cart.ErrConflict {
			conflicts++
		} else {
			t.Fatalf("distinct item race failed: %v", err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal("different operations bypassed optimistic version")
	}
	failed.Version = "5"
	// Immutable replay after subsequent mutations returns its original receipt.
	replay, err := cart.AddItem(ctx, db, "shop", ref, in)
	if err != nil || !replay.Replayed || replay.Version != "2" || replay.Quantity != "1" {
		t.Fatal("historical add receipt reapplied")
	}
	exec(`UPDATE catalog_products SET status='draft' WHERE id='add-product'`)
	failed.OperationID = uuid.NewString()
	reject(failed, cart.ErrItemUnavailable)
	exec(`UPDATE catalog_products SET status='published' WHERE id='add-product'`)
	// Current durable generation is checked before replay, never bypassed by key.
	exec(`UPDATE cart_ownership SET generation=REPEAT('e',64) WHERE cart_id=?`, ref.CartID)
	reject(in, cart.ErrDenied)
	if err := sessions.RevokeCart(ctx, issued.Token); err != nil {
		t.Fatal("owned add cookie cleanup failed")
	}
	checkCartItemsHTTP(t, ctx, db, sessions)
	checkCartSetTransactions(t, ctx, db, sessions)
	exec(`UPDATE cart_item_runtime SET enabled=FALSE WHERE id=1`)
	var reserved string
	if db.Raw(`SELECT CAST(reserved_quantity AS CHAR) FROM inventory_levels WHERE id='add-level'`).Scan(&reserved).Error != nil || reserved != "0.000000000000000000" {
		t.Fatal("add reserved inventory")
	}
	var reservations int64
	if db.Table("inventory_reservations").Where("inventory_item_id='add-item'").Count(&reservations).Error != nil || reservations != 0 {
		t.Fatal("add wrote inventory reservations")
	}
	t.Log(`CART_ITEMS_REHEARSAL {"real_mysql_redis":true,"explicit_migrations":48,"concurrent_attempts":8,"single_fresh_add":1,"merge_quantity_price_tier":true,"exact_raw_beyond_sql_scale":true,"whole_cart_shared_stock":true,"late_receipt_failure_atomic":true,"version_conflict_unchanged":true,"different_operation_race_single_winner":true,"current_catalog_and_authority":true,"no_inventory_reservation":true,"absolute_quantity_and_removal":true,"set_concurrent_replay":true,"delete_late_failure_atomic":true,"delete_draft_product":true,"http_integrated":false,"rules_recalculated":false,"checkout_integrated":false}`)
}

func checkCartSetTransactions(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("owned set transactions")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal("set cart setup failed")
	}
	ref := created.Reference
	added, err := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"})
	if err != nil {
		t.Fatal("set line setup failed")
	}
	command := cart.AddItemCommand{OperationID: uuid.NewString(), Version: "2", Quantity: "2"}
	results := make([]cart.AddItemResult, 8)
	errors := make([]error, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errors[i] = cart.SetItem(ctx, db, "shop", ref, added.LineID, command)
		}(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errors[i] != nil || r.Version != "3" || r.Quantity != "2" {
			t.Fatal("set concurrent receipt failed")
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("set applied repeatedly")
	}
	fault := "20000000-0000-4000-8000-000000000099"
	if db.Exec("ALTER TABLE cart_item_operations ADD CONSTRAINT owned_set_failure CHECK(operation_id<>'"+fault+"')").Error != nil {
		t.Fatal("set fault setup failed")
	}
	_, err = cart.SetItem(ctx, db, "shop", ref, added.LineID, cart.AddItemCommand{OperationID: fault, Version: "3", Quantity: "0"})
	if db.Exec("ALTER TABLE cart_item_operations DROP CHECK owned_set_failure").Error != nil {
		t.Fatal("set fault cleanup failed")
	}
	amounts, readErr := cart.ReadAmounts(ctx, db, "shop", ref)
	if err != cart.ErrStorage || readErr != nil || amounts.Version != "3" || len(amounts.Lines) != 1 || amounts.Lines[0].Quantity != "2" {
		t.Fatal("failed removal did not roll back")
	}
	// Removal must work after the product disappears and preserve its row.
	if db.Exec("UPDATE catalog_products SET status='draft' WHERE id='add-product'").Error != nil {
		t.Fatal("set product fault failed")
	}
	remove := cart.AddItemCommand{OperationID: uuid.NewString(), Version: "3", Quantity: "0"}
	r, err := cart.SetItem(ctx, db, "shop", ref, added.LineID, remove)
	if db.Exec("UPDATE catalog_products SET status='published' WHERE id='add-product'").Error != nil {
		t.Fatal("set product restore failed")
	}
	if err != nil || r.Quantity != "0" || r.Version != "4" {
		t.Fatal("draft product removal failed")
	}
	replay, err := cart.SetItem(ctx, db, "shop", ref, added.LineID, remove)
	if err != nil || !replay.Replayed || replay.Version != "4" {
		t.Fatal("removal replay failed")
	}
	var n int64
	if db.Table("cart_line_items").Where("id=? AND deleted_at IS NOT NULL", added.LineID).Count(&n).Error != nil || n != 1 {
		t.Fatal("removal destroyed saved row")
	}
	// Add and set share the key namespace but cannot replay each other's digest.
	_, err = cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: remove.OperationID, Version: "3", VariantID: "add-v", Quantity: "1"})
	if err != cart.ErrConflict {
		t.Fatal("operation kind confused by same key")
	}
}
