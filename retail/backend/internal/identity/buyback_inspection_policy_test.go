package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/identity"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVerifyBuybackInspectionPolicies(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned browser fixture required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("fixture SQL")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	ctx := context.Background()
	var actor identity.User
	if db.Where("id='new-browser-fixture'").Take(&actor).Error != nil {
		t.Fatal("actor")
	}
	var saved struct{ ID, Items, History, Status string }
	if db.Table("buyback_requests").Select("id,items,history,status").Where("status='received'").Take(&saved).Error != nil {
		t.Fatal("received request")
	}
	var items []buyback.RequestItem
	if json.Unmarshal([]byte(saved.Items), &items) != nil || len(items) != 1 {
		t.Fatal("one phone fixture required")
	}
	item := items[0]
	link := buyback.InspectionCommand{Action: "link_device", ItemID: item.ID, DeviceID: "inspection-fixture-device"}
	quote := buyback.InspectionCommand{Action: "final_offer", Items: []buyback.InspectionOffer{{ItemID: item.ID, Amount: json.Number("270.01"), Notes: "Screen inspected"}}}
	exec := func(tx *gorm.DB, q string, args ...any) {
		t.Helper()
		if tx.Exec(q, args...).Error != nil {
			t.Fatal("fixture SQL statement")
		}
	}
	setup := func(tx *gorm.DB) {
		exec(tx, "INSERT INTO commerce_channel_locations(id,sales_channel_id,stock_location_id,created_at,updated_at) VALUES('inspection-fixture-location-link','fixture-channel','fixture-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec(tx, "INSERT INTO catalog_variants(id,product_id,title,variant_rank,manage_inventory,allow_backorder,created_at,updated_at) VALUES('inspection-fixture-variant',?,'Inspection SKU',0,TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", item.ProductID)
		exec(tx, "INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES('inspection-fixture-variant','fixture-256')")
		exec(tx, "INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES('inspection-fixture-source','Inspection intake','self','fixture-location',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec(tx, "INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('inspection-fixture-device',?,?,'inspection-fixture-variant','inspection-fixture-source','fixture-location',0,'eur','','','inspection','recycled','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", item.SerialNumber, item.IMEI)
	}
	sentinel := errors.New("inspection fixture rollback")
	run := func(name string, fn func(*testing.T, *gorm.DB)) {
		t.Run(name, func(t *testing.T) {
			if e := db.Transaction(func(tx *gorm.DB) error { setup(tx); fn(t, tx); return sentinel }); !errors.Is(e, sentinel) {
				t.Fatal("rollback", e)
			}
		})
	}
	call := func(tx *gorm.DB, in buyback.InspectionCommand) (buyback.AdminDetail, error) {
		return buyback.InspectionTx(ctx, tx, actor, "fixture-store", saved.ID, in)
	}
	run("candidate-http-and-default-denials", func(t *testing.T, tx *gorm.DB) {
		candidates, e := buyback.InspectionCandidatesTx(ctx, tx, actor, "fixture-store", saved.ID, item.ID)
		if e != nil || len(candidates.Devices) != 1 || candidates.Devices[0] != link.DeviceID {
			t.Fatal("candidate", e)
		}
		for _, status := range []string{"awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"} {
			exec(tx, "UPDATE buyback_requests SET status=? WHERE id=?", status, saved.ID)
			for _, command := range []buyback.InspectionCommand{link, quote} {
				if _, e := call(tx, command); !errors.Is(e, buyback.ErrCommandState) {
					t.Fatal("state", status, e)
				}
			}
		}
		exec(tx, "UPDATE buyback_requests SET status='received' WHERE id=?", saved.ID)
		exec(tx, "UPDATE buyback_admin_runtime SET enabled=FALSE WHERE id=1")
		if _, e := call(tx, link); !errors.Is(e, buyback.ErrUnavailable) {
			t.Fatal("disabled", e)
		}
	})
	run("success-and-customer-choice", func(t *testing.T, tx *gorm.DB) {
		snapshot := func() string {
			all := []any{}
			for _, table := range []string{"inventory_levels", "inventory_reservations", "inventory_stock_edits"} {
				var rows []map[string]any
				order := "id"
				if table == "inventory_stock_edits" {
					order = "operation_id"
				}
				if tx.Table(table).Order(order).Find(&rows).Error != nil {
					t.Fatal("inventory snapshot")
				}
				all = append(all, rows)
			}
			raw, _ := json.Marshal(all)
			return string(raw)
		}
		stockBefore := snapshot()

		v, e := call(tx, link)
		if e != nil || v.Request.Items[0].DeviceID == nil || v.Request.Status != "received" {
			t.Fatal("link", e)
		}
		var n int64
		tx.Table("customer_account_notices").Where("resource_id=? AND action='link_device'", saved.ID).Count(&n)
		if n != 0 {
			t.Fatal("link emitted notice")
		}
		v, e = call(tx, quote)
		if e != nil || v.Request.FinalAmount == nil || string(*v.Request.FinalAmount) != "270.01" || v.Request.Items[0].InspectionNotes != "Screen inspected" || v.Request.Status != "final_offer" {
			t.Fatal("quote", e)
		}
		if snapshot() != stockBefore {
			t.Fatal("inspection changed stock")
		}
		tx.SavePoint("unique_links")
		if tx.Exec("INSERT INTO buyback_device_links(id,request_id,item_id,device_id,created_at,updated_at) VALUES('duplicate-fixture-link',?,'different-fixture-item','inspection-fixture-device',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", saved.ID).Error == nil {
			t.Fatal("same device linked to second item")
		}
		tx.RollbackTo("unique_links")
		amount := json.Number("270.01")
		accepted, e := buyback.StageCustomerCommand(ctx, tx, "fixture-store", "cust_buyback-browser", saved.ID, buyback.CustomerCommand{Action: "accept_offer", FinalAmount: &amount})
		if e != nil || accepted.Status != "accepted" {
			t.Fatal("customer accept", e)
		}
	})
	for _, mutation := range []string{"category='sale'", "state='available'", "serial_number='OTHER'", "imei='000000000000009'", "deleted_at=UTC_TIMESTAMP(6)", "location_id='other-location'"} {
		run("device-"+mutation, func(t *testing.T, tx *gorm.DB) {
			exec(tx, "UPDATE devices SET "+mutation+" WHERE id='inspection-fixture-device'")
			if _, e := call(tx, link); !errors.Is(e, buyback.ErrDeviceMismatch) {
				t.Fatal("invalid device linked", e)
			}
		})
	}
	run("wrong-product-capacity-source-channel", func(t *testing.T, tx *gorm.DB) {
		for _, q := range []string{"UPDATE catalog_variants SET product_id='fixture-a' WHERE id='inspection-fixture-variant'", "UPDATE catalog_variant_options SET option_value_id='fixture-128' WHERE variant_id='inspection-fixture-variant'", "UPDATE device_sources SET deleted_at=UTC_TIMESTAMP(6) WHERE id='inspection-fixture-source'", "UPDATE commerce_channel_locations SET deleted_at=UTC_TIMESTAMP(6) WHERE stock_location_id='fixture-location'"} {
			tx.SavePoint("bad_context")
			exec(tx, q)
			if _, e := call(tx, link); !errors.Is(e, buyback.ErrDeviceMismatch) {
				t.Fatal("stale device context", e)
			}
			tx.RollbackTo("bad_context")
		}
	})
	run("permissions-state-and-exact-coverage", func(t *testing.T, tx *gorm.DB) {
		if _, e := call(tx, quote); !errors.Is(e, buyback.ErrLinkDevices) {
			t.Fatal("quote without device", e)
		}
		exec(tx, "DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='buyback.write'", actor.ID)
		if _, e := call(tx, link); !errors.Is(e, authorization.ErrDenied) {
			t.Fatal("revoked", e)
		}
	})
	run("quote-coverage-and-state", func(t *testing.T, tx *gorm.DB) {
		if _, e := call(tx, link); e != nil {
			t.Fatal(e)
		}
		for _, bad := range []buyback.InspectionCommand{{Action: "final_offer", Items: append(append([]buyback.InspectionOffer{}, quote.Items...), quote.Items...)}, {Action: "final_offer", Items: []buyback.InspectionOffer{{ItemID: "foreign", Amount: json.Number("1"), Notes: "inspected"}}}} {
			if _, e := call(tx, bad); !errors.Is(e, buyback.ErrInput) {
				t.Fatal("coverage", e)
			}
		}
		zero := quote
		zero.Items = append([]buyback.InspectionOffer{}, quote.Items...)
		zero.Items[0].Amount = json.Number("0")
		v, e := call(tx, zero)
		if e != nil || v.Request.FinalAmount == nil || string(*v.Request.FinalAmount) != "0" {
			t.Fatal("zero quote", e)
		}
		if _, e := call(tx, quote); !errors.Is(e, buyback.ErrCommandState) {
			t.Fatal("quote repeated", e)
		}
	})
	run("two-phone-order-and-decline", func(t *testing.T, tx *gorm.DB) {
		second := item
		second.ID = item.ID + "-second"
		second.SerialNumber = "INSPECTION-SECOND"
		second.IMEI = "000000000000002"
		both, _ := json.Marshal([]buyback.RequestItem{item, second})
		exec(tx, "UPDATE buyback_requests SET items=? WHERE id=?", string(both), saved.ID)
		exec(tx, "INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES('inspection-fixture-second','INSPECTION-SECOND','000000000000002','inspection-fixture-variant','inspection-fixture-source','fixture-location',0,'eur','','','inspection','recycled','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		if _, e := call(tx, link); e != nil {
			t.Fatal(e)
		}
		if _, e := call(tx, buyback.InspectionCommand{Action: "link_device", ItemID: second.ID, DeviceID: "inspection-fixture-second"}); e != nil {
			t.Fatal(e)
		}
		// Input order need not match request order; each amount remains item-bound.
		in := buyback.InspectionCommand{Action: "final_offer", Items: []buyback.InspectionOffer{{ItemID: second.ID, Amount: json.Number("0.20"), Notes: "Second inspected"}, {ItemID: item.ID, Amount: json.Number("0.10"), Notes: "First inspected"}}}
		v, e := call(tx, in)
		if e != nil || v.Request.FinalAmount == nil || string(*v.Request.FinalAmount) != "0.3" || v.Request.Items[0].InspectionNotes != "First inspected" || v.Request.Items[1].InspectionNotes != "Second inspected" {
			t.Fatal("multi quote", e)
		}
		returned, e := buyback.StageCustomerCommand(ctx, tx, "fixture-store", "cust_buyback-browser", saved.ID, buyback.CustomerCommand{Action: "decline_offer"})
		if e != nil || returned.Status != "return_requested" {
			t.Fatal("decline", e)
		}
	})
	// Fail after all writes; no link, item snapshot, history, notice or audit survives.
	var notices, audits int64
	db.Table("customer_account_notices").Count(&notices)
	db.Table("audit_events").Count(&audits)
	run("late-failure", func(t *testing.T, tx *gorm.DB) {
		if _, e := call(tx, link); e != nil {
			t.Fatal(e)
		}
		if _, e := call(tx, quote); e != nil {
			t.Fatal(e)
		}
	})
	var after struct{ ID, Items, History, Status string }
	db.Table("buyback_requests").Select("id,items,history,status").Where("id=?", saved.ID).Take(&after)
	var n, a, l int64
	db.Table("customer_account_notices").Count(&n)
	db.Table("audit_events").Count(&a)
	db.Table("buyback_device_links").Count(&l)
	if saved != after || n != notices || a != audits || l != 0 {
		t.Fatal("rollback effects")
	}
	// Competing actual transactions on the same request/device: exactly one wins.
	if e := db.Transaction(func(tx *gorm.DB) error { setup(tx); return nil }); e != nil {
		t.Fatal(e)
	}
	defer func() {
		db.Exec("DELETE FROM buyback_device_links WHERE device_id='inspection-fixture-device'")
		db.Exec("UPDATE buyback_requests SET items=?,history=?,status=? WHERE id=?", saved.Items, saved.History, saved.Status, saved.ID)
		db.Exec("DELETE FROM devices WHERE id='inspection-fixture-device'")
		db.Exec("DELETE FROM device_sources WHERE id='inspection-fixture-source'")
		db.Exec("DELETE FROM catalog_variant_options WHERE variant_id='inspection-fixture-variant'")
		db.Exec("DELETE FROM catalog_variants WHERE id='inspection-fixture-variant'")
		db.Exec("DELETE FROM commerce_channel_locations WHERE id='inspection-fixture-location-link'")
	}()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- db.Transaction(func(tx *gorm.DB) error { _, e := call(tx, link); return e })
		}()
	}
	wg.Wait()
	close(results)
	win, denied := 0, 0
	for e := range results {
		if e == nil {
			win++
		} else if errors.Is(e, buyback.ErrDeviceMismatch) {
			denied++
		} else {
			t.Fatal(e)
		}
	}
	if win != 1 || denied != 1 {
		t.Fatal("duplicate device link")
	}
	var quoteWG sync.WaitGroup
	quoteResults := make(chan error, 2)
	for i := 0; i < 2; i++ {
		quoteWG.Add(1)
		go func() {
			defer quoteWG.Done()
			quoteResults <- db.Transaction(func(tx *gorm.DB) error { _, e := call(tx, quote); return e })
		}()
	}
	quoteWG.Wait()
	close(quoteResults)
	win, denied = 0, 0
	for e := range quoteResults {
		if e == nil {
			win++
		} else if errors.Is(e, buyback.ErrCommandState) {
			denied++
		} else {
			t.Fatal("quote race", e)
		}
	}
	if win != 1 || denied != 1 {
		t.Fatal("duplicate final offer")
	}
	exec(db, "UPDATE buyback_requests SET final_amount=NULL,raw_final_amount=NULL WHERE id=?", saved.ID)
	exec(db, "DELETE FROM customer_account_notices WHERE resource_id=? AND action='final_offer'", saved.ID)
	t.Log(`BUYBACK_INSPECTION_POLICIES {"same_device_single_winner":true,"device_identity_sku_capacity_scope_checked":true,"exact_item_coverage":true,"zero_quote":true,"customer_acceptance":true,"late_failure_atomic":true,"link_notice_suppressed":true,"competing_quote_single_winner":true,"multiple_phone_ordered_sum":true,"customer_decline":true,"inventory_unchanged":true}`)
}
