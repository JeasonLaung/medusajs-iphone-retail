package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOwnedCartPriceRefresh(t *testing.T) {
	if os.Getenv("IDENTITY_REHEARSAL") != "1" {
		t.Skip("owned MySQL/Redis required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	cfg := driver.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned cart root connection unavailable")
	}
	defer root.Close()
	if _, err := root.ExecContext(ctx, "CREATE DATABASE cart_price_refresh_rehearsal"); err != nil {
		t.Fatal("owned cart database unavailable")
	}
	cfg.DBName = "cart_price_refresh_rehearsal"
	gdb, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("owned cart connection unavailable")
	}
	db, _ := gdb.DB()
	defer db.Close()
	db.SetMaxOpenConns(12)
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		t.Fatal("cart runtime migration plan unavailable")
	}
	r, err := schema.Apply(ctx, db, plan)
	if err != nil || r.Applied != 81 {
		t.Fatal("cart runtime migration failed")
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if gdb.WithContext(ctx).Exec(query, args...).Error != nil {
			t.Fatal("artificial cart fixture mutation failed")
		}
	}
	exec(`INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES ('channel','Channel',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_stores(id,name,default_sales_channel_id,created_at,updated_at) VALUES ('shop','Shop','channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES ('region','Europe','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('region-other','Other','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES ('currency','shop','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE cart_command_capabilities SET enabled=TRUE`)
	created, err := cart.Create(ctx, gdb, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial price cart")), StoreID: "shop", RegionID: "region"})
	if err != nil {
		t.Fatal("price fixture cart creation failed")
	}
	ref := created.Reference
	exec(`INSERT INTO catalog_products(id,title,handle,is_giftcard,status,discountable,created_at,updated_at) VALUES('product','Current product','current-product',FALSE,'published',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_channels(product_id,sales_channel_id,id,created_at,updated_at) VALUES('product','channel','link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES('v','product','Variant',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('no-price','product','Custom variant',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_sets(id,created_at,updated_at) VALUES('set',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_variant_price_sets(variant_id,price_set_id,id,created_at,updated_at) VALUES('v','set','price-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,min_quantity,max_quantity,created_at,updated_at) VALUES('single','set','eur',100,'{"value":"100.000000000000000000001","precision":38}',0,NULL,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('bulk','set','eur',80,'{"value":"80.000000000000000000001","precision":38}',0,2,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_preferences(id,attribute,value,is_tax_inclusive,created_at,updated_at) VALUES('inclusive','currency_code','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for i, id := range []string{"a", "b", "c", "manual", "no-price-custom", "removed"} {
		var variant any = "v"
		if id == "manual" {
			variant = nil
		}
		if id == "no-price-custom" {
			variant = "no-price"
		}
		exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,unit_price,raw_unit_price,requires_shipping,is_discountable,is_tax_inclusive,is_custom_price,is_giftcard,variant_id,metadata,created_at,updated_at) VALUES(?,?,'Saved title',?,7,'{"value":"7.000000000000000000003","precision":38}',TRUE,FALSE,FALSE,?,FALSE,?,'{"saved":true}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, ref.CartID, i+1, id == "c" || id == "manual" || id == "no-price-custom", variant)
	}
	exec(`UPDATE cart_line_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='removed'`)
	exec(`UPDATE cart_line_items SET compare_at_unit_price=95,raw_compare_at_unit_price='{"value":"95.000000000000000000009","precision":38}' WHERE id='b'`)
	fingerprint := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"cart_line_items", "carts", "cart_ownership", "cart_item_operations", "cart_line_tax_lines", "cart_line_adjustments", "inventory_reservations"} {
			var rows []map[string]any
			if gdb.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("price fingerprint unavailable")
			}
			encoded, _ := json.Marshal(rows)
			all = append(all, encoded)
		}
		b, _ := json.Marshal(all)
		return fmt.Sprintf("%x", sha256.Sum256(b))
	}
	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	refresh := func(rollback bool) (cart.PriceRefreshResult, error) {
		var result cart.PriceRefreshResult
		err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.RefreshItemPrices(ctx, tx, "shop", ref, "1", at)
			if err != nil {
				return err
			}
			if rollback {
				return errors.New("artificial later refresh failure")
			}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		return result, err
	}
	before := fingerprint()
	if _, err := cart.RefreshItemPrices(ctx, gdb, "shop", ref, "1", at); err != cart.ErrInput {
		t.Fatal("autocommit price refresh accepted")
	}
	if _, err := refresh(true); err == nil || before != fingerprint() {
		t.Fatal("later refresh failure retained prices")
	}
	exec(`CREATE TRIGGER price_late_failure BEFORE UPDATE ON cart_line_items FOR EACH ROW BEGIN IF NEW.id='b' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial late failure'; END IF; END`)
	if _, err := refresh(false); err != cart.ErrStorage || before != fingerprint() {
		t.Fatal("late item update left partial prices")
	}
	exec(`DROP TRIGGER price_late_failure`)
	if result, err := refresh(false); err != nil || result.Repriced != 2 || result.Retained != 3 {
		t.Fatal("per-line price refresh unavailable")
	}
	read := func(id string, price string, inclusive bool, compare *string) {
		t.Helper()
		var row struct {
			RawUnitPrice          string
			IsTaxInclusive        bool
			RawCompareAtUnitPrice *string
			Title, Metadata       string
		}
		if gdb.Table("cart_line_items").Select("raw_unit_price,is_tax_inclusive,raw_compare_at_unit_price,title,metadata").Where("id=?", id).Take(&row).Error != nil {
			t.Fatal("price result unavailable")
		}
		var raw struct{ Value string }
		if json.Unmarshal([]byte(row.RawUnitPrice), &raw) != nil || raw.Value != price || row.IsTaxInclusive != inclusive || row.Title != "Saved title" || row.Metadata != "{\"saved\":true}" {
			t.Fatal("price exactness, flags or saved content changed")
		}
		if compare == nil {
			if row.RawCompareAtUnitPrice != nil {
				t.Fatal("compare-at fabricated")
			}
			return
		}
		if row.RawCompareAtUnitPrice == nil || json.Unmarshal([]byte(*row.RawCompareAtUnitPrice), &raw) != nil || raw.Value != *compare {
			t.Fatal("compare-at exactness changed")
		}
	}
	ptr := func(s string) *string { return &s }
	read("a", "100.000000000000000000001", true, nil)
	read("b", "80.000000000000000000001", true, ptr("95.000000000000000000009"))
	for _, id := range []string{"c", "manual", "no-price-custom", "removed"} {
		read(id, "7.000000000000000000003", false, nil)
	}
	// Sale fallback fills only null compare-at; even a custom price keeps its
	// own unit price and tax flag. Existing exact compare-at is never cleared.
	exec(`INSERT INTO pricing_price_lists(id,title,description,type,status,rules_count,created_at,updated_at) VALUES('sale-list','Sale','Sample','sale','active',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('sale','set','eur',60,'{"value":"60.000000000000000000006","precision":38}',0,'sale-list',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if _, err := refresh(false); err != nil {
		t.Fatal("sale refresh unavailable")
	}
	read("a", "60.000000000000000000006", true, ptr("100.000000000000000000001"))
	read("b", "60.000000000000000000006", true, ptr("95.000000000000000000009"))
	read("c", "7.000000000000000000003", false, ptr("80.000000000000000000001"))
	exec(`UPDATE pricing_price_lists SET status='draft' WHERE id='sale-list'`)
	if _, err := refresh(false); err != nil {
		t.Fatal("non-sale refresh unavailable")
	}
	read("a", "100.000000000000000000001", true, ptr("100.000000000000000000001"))
	read("b", "80.000000000000000000001", true, ptr("95.000000000000000000009"))
	// A deleted published variant or missing configured price rejects the
	// entire price phase without removing saved lines or guessing zero.
	before = fingerprint()
	exec(`UPDATE catalog_products SET status='draft' WHERE id='product'`)
	if _, err := refresh(false); err != cart.ErrItemUnavailable || before != fingerprint() {
		t.Fatal("unpublished variant refreshed prices")
	}
	exec(`UPDATE catalog_products SET status='published' WHERE id='product'`)
	exec(`UPDATE pricing_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE id IN ('single','bulk')`)
	if _, err := refresh(false); err != cart.ErrItemUnavailable || before != fingerprint() {
		t.Fatal("missing price mutated cart")
	}
	exec(`UPDATE pricing_prices SET deleted_at=NULL WHERE id IN ('single','bulk')`)
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := cart.RefreshItemPrices(ctx, tx, "shop", ref, "2", at); return err }); err != cart.ErrConflict || before != fingerprint() {
		t.Fatal("stale version changed prices")
	}
	// Explicit target region comes from the stored cart, never a caller DTO.
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,created_at,updated_at) VALUES('regional','set','eur',30,'{"value":"30","precision":38}',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_price_rules(id,price_id,attribute,operator,value,priority,created_at,updated_at) VALUES('region-rule','regional','region_id','eq','region-other',0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE carts SET region_id='region-other' WHERE id=?`, ref.CartID)
	if _, err := refresh(false); err != nil {
		t.Fatal("stored region price refresh unavailable")
	}
	read("a", "30", true, ptr("100.000000000000000000001"))
	read("b", "30", true, ptr("95.000000000000000000009"))
	current, err := cart.Inspect(ctx, gdb, "shop", ref)
	if err != nil || current.Version != "1" {
		t.Fatal("price phase changed cart version")
	}
	checkCartItemSnapshots(t, ctx, gdb, ref)
	checkFlatShippingPrices(t, ctx, gdb)
	checkShippingCandidates(t, ctx, gdb)
	checkManualShippingQuotes(t, ctx, gdb)
	checkCartShippingProfiles(t, ctx, gdb, ref)
	t.Log(`CART_PRICE_REFRESH_REHEARSAL {"migrations":61,"per_line_quantity_context":true,"shared_set":true,"raw_beyond_scale_18":true,"custom_and_manual_prices_retained":true,"saved_compare_at_retained":true,"sale_compare_fallback":true,"tax_inclusive_preference":true,"soft_deleted_lines_preserved":true,"later_failure_rollback":true,"late_update_failure_rollback":true,"unpublished_and_missing_price_rejected":true,"stale_version_rejected":true,"stored_region_context":true,"full_refresh":false,"checkout":false}`)
}
