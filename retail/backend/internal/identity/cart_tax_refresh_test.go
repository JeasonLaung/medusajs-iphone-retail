package identity_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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

func TestOwnedCartTaxRefresh(t *testing.T) {
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
	if _, err := root.ExecContext(ctx, "CREATE DATABASE cart_tax_refresh_rehearsal"); err != nil {
		t.Fatal("owned cart database unavailable")
	}
	cfg.DBName = "cart_tax_refresh_rehearsal"
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
	if err != nil || r.Applied != 76 {
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
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES ('region','Europe','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO commerce_store_currencies(id,store_id,currency_code,is_default,created_at,updated_at) VALUES ('currency','shop','eur',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE cart_command_capabilities SET enabled=TRUE`)
	created, err := cart.Create(ctx, gdb, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("artificial tax cart")), StoreID: "shop", RegionID: "region"})
	if err != nil {
		t.Fatal("tax fixture cart creation failed")
	}
	ref := created.Reference
	exec(`INSERT INTO cart_addresses(id,country_code,province,created_at,updated_at) VALUES ('address','DK','SAMPLE',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`UPDATE carts SET shipping_address_id='address' WHERE id=?`, ref.CartID)
	for _, id := range []string{"item", "gift", "removed"} {
		exec(`INSERT INTO cart_line_items(id,cart_id,title,quantity,unit_price,raw_unit_price,requires_shipping,is_discountable,is_tax_inclusive,is_custom_price,is_giftcard,product_id,created_at,updated_at) VALUES(?,?,'Sample',1,100,'{"value":"100","precision":100}',TRUE,TRUE,FALSE,FALSE,?,'product',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, id, ref.CartID, id == "gift")
	}
	exec(`UPDATE cart_line_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id='removed'`)
	exec(`INSERT INTO carts(id,currency_code,created_at,updated_at) VALUES('unrelated','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,shipping_option_id,created_at,updated_at) VALUES('ship',?,'Shipping',10,'{"value":"10","precision":100}',FALSE,'option',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, ref.CartID)
	exec(`INSERT INTO cart_shipping_methods(id,cart_id,name,amount,raw_amount,is_tax_inclusive,created_at,updated_at) VALUES('unrelated-ship','unrelated','Unrelated',10,'{"value":"10","precision":100}',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	for _, id := range []string{"item", "gift", "removed"} {
		exec(`INSERT INTO cart_line_tax_lines(id,item_id,code,rate,created_at,updated_at) VALUES(?,?,'OLD',99,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, "old-"+id, id)
	}
	exec(`INSERT INTO cart_shipping_tax_lines(id,shipping_method_id,code,rate,created_at,updated_at) VALUES('old-ship','ship','OLD',99,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('unrelated-tax','unrelated-ship','KEEP',77,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_providers VALUES('tp_system',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),NULL)`)
	exec(`INSERT INTO tax_regions(id,provider_id,country_code,province_code,parent_id,created_at,updated_at) VALUES ('country','tp_system','dk',NULL,NULL,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('province',NULL,'dk','sample','country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES ('default',12.345678901234567891,'VAT','Default',TRUE,FALSE,'country',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('local',5,'LOCAL','Local',TRUE,TRUE,'province',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	fingerprint := func() string {
		t.Helper()
		var rows []struct{ Data string }
		if gdb.Raw(`SELECT CAST(JSON_ARRAY(id,item_id,description,tax_rate_id,code,CAST(rate AS CHAR),provider_id,created_at,updated_at,deleted_at) AS CHAR) AS data FROM cart_line_tax_lines ORDER BY id`).Scan(&rows).Error != nil {
			t.Fatal("tax evidence read unavailable")
		}
		var shipping []struct{ Data string }
		if gdb.Raw(`SELECT CAST(JSON_ARRAY(id,shipping_method_id,description,tax_rate_id,code,CAST(rate AS CHAR),provider_id,created_at,updated_at,deleted_at) AS CHAR) AS data FROM cart_shipping_tax_lines ORDER BY id`).Scan(&shipping).Error != nil {
			t.Fatal("shipping evidence read unavailable")
		}
		encoded, _ := json.Marshal(append(rows, shipping...))
		return fmt.Sprintf("%x", sha256.Sum256(encoded))
	}
	refresh := func(force bool, rollback bool) (cart.TaxRefreshResult, error) {
		var result cart.TaxRefreshResult
		err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = cart.RefreshSystemTaxes(ctx, tx, "shop", ref, "1", force)
			if err != nil {
				return err
			}
			if rollback {
				return errors.New("artificial subsequent refresh failure")
			}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		return result, err
	}
	before := fingerprint()
	if _, err := cart.RefreshSystemTaxes(ctx, gdb, "shop", ref, "1", false); err != cart.ErrInput {
		t.Fatal("autocommit tax refresh accepted")
	}
	if result, err := refresh(false, true); err == nil || result.ItemLines != 2 || result.ShippingLines != 2 || fingerprint() != before {
		t.Fatal("subsequent domain failure did not roll back taxes")
	}
	exec(`CREATE TRIGGER tax_late_failure BEFORE INSERT ON cart_shipping_tax_lines FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial late failure'`)
	if _, err := refresh(false, false); err != cart.ErrStorage || fingerprint() != before {
		t.Fatal("late shipping failure left partial item taxes")
	}
	exec(`DROP TRIGGER tax_late_failure`)
	if result, err := refresh(false, false); err != nil || result.ItemLines != 2 || result.ShippingLines != 2 {
		t.Fatal("cart tax refresh unavailable")
	}
	var exact string
	if gdb.Raw(`SELECT CAST(rate AS CHAR) FROM cart_line_tax_lines WHERE deleted_at IS NULL AND tax_rate_id='default'`).Scan(&exact).Error != nil || exact != "12.345678901234567891" {
		t.Fatal("persisted tax precision changed")
	}
	var unexpected int64
	gdb.Raw(`SELECT COUNT(*) FROM cart_line_tax_lines WHERE deleted_at IS NULL AND (item_id IN ('gift','removed') OR provider_id<>'system' OR description IS NULL)`).Scan(&unexpected)
	if unexpected != 0 {
		t.Fatal("giftcard removed line or provider differs")
	}
	gdb.Raw(`SELECT COUNT(*) FROM cart_shipping_tax_lines WHERE id='unrelated-tax' AND deleted_at IS NULL AND rate=77`).Scan(&unexpected)
	if unexpected != 1 {
		t.Fatal("refresh modified another cart")
	}
	// A fresh set replaces old IDs rather than accumulating active taxes.
	if _, err := refresh(false, false); err != nil {
		t.Fatal("repeat tax set failed")
	}
	gdb.Raw(`SELECT COUNT(*) FROM cart_line_tax_lines WHERE deleted_at IS NULL`).Scan(&unexpected)
	if unexpected != 2 {
		t.Fatal("tax set accumulated active lines")
	}
	before = fingerprint()
	bad := ref
	bad.Generation = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := cart.RefreshSystemTaxes(ctx, tx, "shop", bad, "1", false)
		return err
	}); err != cart.ErrDenied || fingerprint() != before {
		t.Fatal("foreign authority changed taxes")
	}
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := cart.RefreshSystemTaxes(ctx, tx, "shop", ref, "2", false)
		return err
	}); err != cart.ErrConflict || fingerprint() != before {
		t.Fatal("stale version changed taxes")
	}
	exec(`UPDATE tax_providers SET is_enabled=FALSE`)
	if _, err := refresh(false, false); err != cart.ErrSchema || fingerprint() != before {
		t.Fatal("invalid provider cleared saved taxes")
	}
	exec(`UPDATE tax_providers SET is_enabled=TRUE`)
	exec(`UPDATE tax_rates SET code=REPEAT('税',191) WHERE id='default'`)
	if _, err := refresh(false, false); err != nil {
		t.Fatal("valid multibyte tax code rejected")
	}
	var code string
	if gdb.Raw(`SELECT code FROM cart_line_tax_lines WHERE deleted_at IS NULL AND tax_rate_id='default'`).Scan(&code).Error != nil || code != strings.Repeat("税", 191) {
		t.Fatal("multibyte code changed")
	}
	exec(`UPDATE tax_rates SET code='VAT' WHERE id='default'`)
	// Establish an old RR snapshot, then change cart facts in a second connection.
	// The helper must use the currently locked cart and relations, not old facts.
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old string
		if tx.Raw(`SELECT country_code FROM cart_addresses WHERE id='address'`).Scan(&old).Error != nil {
			return errors.New("snapshot unavailable")
		}
		if gdb.Exec(`UPDATE cart_addresses SET country_code='fr' WHERE id='address'`).Error != nil {
			return errors.New("concurrent address unavailable")
		}
		result, err := cart.RefreshSystemTaxes(ctx, tx, "shop", ref, "1", false)
		if err != nil || result.ItemLines != 0 || result.ShippingLines != 0 {
			return errors.New("old snapshot address reused")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead}); err != nil {
		t.Fatal("current cart facts hidden by old snapshot")
	}
	exec(`UPDATE cart_addresses SET country_code='DK' WHERE id='address'`)
	if _, err := refresh(false, false); err != nil {
		t.Fatal("restored address tax calculation failed")
	}
	exec(`UPDATE commerce_regions SET automatic_taxes=FALSE`)
	if result, err := refresh(false, false); err != nil || result.ItemLines != 0 || result.ShippingLines != 0 {
		t.Fatal("disabled automatic taxes did not clear lines")
	}
	gdb.Raw(`SELECT COUNT(*) FROM cart_line_tax_lines WHERE deleted_at IS NULL`).Scan(&unexpected)
	if unexpected != 0 {
		t.Fatal("automatic-disabled refresh retained taxes")
	}
	if result, err := refresh(true, false); err != nil || result.ItemLines != 2 || result.ShippingLines != 2 {
		t.Fatal("force calculation ignored persisted context")
	}
	exec(`UPDATE carts SET shipping_address_id=NULL WHERE id=?`, ref.CartID)
	before = fingerprint()
	if _, err := refresh(true, false); err != cart.ErrSchema || fingerprint() != before {
		t.Fatal("force without address mutated saved taxes")
	}
	exec(`UPDATE commerce_regions SET automatic_taxes=TRUE`)
	if result, err := refresh(false, false); err != nil || result.ItemLines != 0 || result.ShippingLines != 0 {
		t.Fatal("missing address did not clear lines")
	}
	// This component deliberately leaves durable version and all other domains
	// to its caller; a committed test transaction is not a public cart command.
	current, err := cart.Inspect(ctx, gdb, "shop", ref)
	if err != nil || current.Version != "1" {
		t.Fatal("tax helper changed cart version")
	}
	t.Log(`CART_TAX_REFRESH_REHEARSAL {"migrations":61,"persisted_context":true,"item_and_shipping_lines":true,"exact_rate":true,"soft_delete_replace":true,"giftcard_and_removed_cleanup":true,"unrelated_cart_preserved":true,"whole_transaction_rollback":true,"late_insert_failure_rollback":true,"authority_version_rechecked":true,"provider_failure_preserves_taxes":true,"multibyte_code_preserved":true,"old_snapshot_current_address":true,"automatic_force_address_gates":true,"standalone_command":false,"checkout":false}`)
}
