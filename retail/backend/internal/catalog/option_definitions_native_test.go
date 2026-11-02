package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNativeOptionDefinitionTransactions(t *testing.T) {
	if os.Getenv("OPTION_DEFINITIONS_NATIVE") != "1" {
		t.Skip("owned private-socket MySQL required")
	}
	name, socket := os.Getenv("OPTION_TEST_DATABASE"), os.Getenv("OPTION_TEST_SOCKET")
	if !regexp.MustCompile(`^eurorefurb_options_[a-f0-9]{12}$`).MatchString(name) || !regexp.MustCompile(`^/private/tmp/eroptions-[a-f0-9]{12}-[^/]+/mysql.sock$`).MatchString(socket) {
		t.Fatal("owned fixture identity required")
	}
	cfg := driver.NewConfig()
	cfg.User, cfg.Net, cfg.Addr, cfg.ParseTime, cfg.Loc = "root", "unix", socket, true, time.UTC
	control, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	defer control.Close()
	var exists int
	if control.QueryRow("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=?", name).Scan(&exists) != nil || exists != 0 {
		t.Fatal("fresh owned database required")
	}
	if _, e = control.Exec("CREATE DATABASE `" + name + "`"); e != nil {
		t.Fatal(e)
	}
	keepForBrowser := false
	defer func() {
		if keepForBrowser {
			return
		}
		if _, e := control.Exec("DROP DATABASE `" + name + "`"); e != nil {
			t.Error("owned database cleanup failed", e)
		}
	}()
	cfg.DBName = name
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(12)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	plan, e := schema.Load(migrations.Files)
	if e != nil {
		t.Fatal(e)
	}
	if applied, e := schema.Apply(ctx, pool, plan); e != nil || applied.Applied != 99 || schema.CheckApplied(ctx, pool, plan) != nil {
		t.Fatal("explicit migration plan failed", applied, e)
	}
	verifyDefinitionUniquenessUpgrade(t, control, cfg)
	verifyDefinitionConstraints(t, db)
	exec := func(query string, args ...any) {
		t.Helper()
		if e := db.Exec(query, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	now := time.Now().UTC()
	actor := identity.User{ID: "option-native-admin", Email: "option-native@example.test", Name: "Artificial option operator", Role: "admin", Enabled: true, AuthVersion: 1, PasswordHash: "unused-artificial-hash", CreatedAt: now, UpdatedAt: now}
	if e = db.Create(&actor).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error { return authorization.Bootstrap(ctx, tx, actor.ID) }); e != nil {
		t.Fatal(e)
	}
	verifyQuantityPrices(t, db, actor)
	verifyPriceLists(t, db, actor)
	verifyPriceListMembers(t, db, actor)
	verifyPriceListCreate(t, db, actor)
	verifyPriceListAdd(t, db, actor)
	verifyPriceListBatch(t, db, actor)
	verifyRegionPriceCommands(t, db, actor)
	verifyPolicyCommands(t, db, actor)
	verifySearchEnglishNative(t, db)
	h := Handler{DB: db}
	input := func(title string) OptionDefinitionInput {
		return OptionDefinitionInput{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("0", 64), Title: title, Values: []string{"128 GB", "256 GB"}, Ranks: map[string]int{"128 GB": 7, "256 GB": 2}}
	}
	first := input("容量")
	if _, e := h.CreateOptionDefinition(ctx, actor, first); !errors.Is(e, ErrSchema) {
		t.Fatal("default closed gate accepted write", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE catalog_command_capabilities SET enabled=TRUE WHERE command IN ('product.option-definitions','product.options')")
	results, errs := make([]OptionDefinitionResult, 6), make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.CreateOptionDefinition(ctx, actor, first) }(i)
	}
	wg.Wait()
	fresh, id := 0, results[0].ID
	for i, result := range results {
		if errs[i] != nil || result.ID != id || result.Revision != results[0].Revision {
			t.Fatal("concurrent receipt mismatch", errs[i])
		}
		if !result.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("concurrent create was not unique")
	}
	read := func() OptionDefinitionContext {
		t.Helper()
		d, e := h.ReadOptionDefinition(ctx, id, actor)
		if e != nil {
			t.Fatal(e)
		}
		return d
	}

	// List filters run against real MySQL collation, LIKE escaping and pagination.
	for i := 0; i < 23; i++ {
		filterID := fmt.Sprintf("option-filter-%02d", i)
		exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES(?,?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", filterID, fmt.Sprintf("Filter literal %%_! %02d", i))
	}
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('option-filter-exclusive','Filter literal %_! excluded',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at,deleted_at) VALUES('option-filter-deleted','Filter literal %_! deleted',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("UPDATE catalog_options SET created_at='2026-01-02 00:00:00.123456',updated_at='2026-02-03 00:00:00.654321' WHERE id LIKE 'option-filter-%'")
	exec("UPDATE catalog_options SET created_at='2026-01-02 00:00:00.123455',updated_at='2026-02-03 00:00:00.654320' WHERE id='option-filter-00'")
	exec("UPDATE catalog_options SET created_at='2026-01-02 00:00:00.123457',updated_at='2026-02-03 00:00:00.654322' WHERE id='option-filter-22'")
	for _, test := range []struct {
		raw   string
		count int64
		size  int
		first string
	}{
		{"q=%25_!&sort=title_asc", 23, 20, "option-filter-00"},
		{"q=%25_!&offset=20&sort=title_asc", 23, 3, "option-filter-20"},
		{"q=%25_!&sort=title_desc", 23, 20, "option-filter-22"},
		{"title=Filter+literal+%25_!+03", 1, 1, "option-filter-03"},
		{"id=option-filter-07&q=literal", 1, 1, "option-filter-07"},
		{"id=option-filter-07&title=wrong", 0, 0, ""},
		{"q=%E5%AE%B9%E9%87%8F", 1, 1, id},
		{"q=missing", 0, 0, ""},
		{"id[]=option-filter-00&id[]=option-filter-22", 2, 2, "option-filter-00"},
		{"title[]=Filter+literal+%25_!+00&title[]=Filter+literal+%25_!+22", 2, 2, "option-filter-00"},
		{"id[]=option-filter-00&id[]=option-filter-22&title[]=Filter+literal+%25_!+22", 1, 1, "option-filter-22"},
		{"q=literal&created_at[$eq]=2026-01-02T00:00:00.123456Z", 21, 20, "option-filter-01"},
		{"q=literal&created_at[$gte]=2026-01-02T08:00:00.123456%2B08:00&created_at[$lte]=2026-01-02T00:00:00.123456Z&offset=20", 21, 1, "option-filter-21"},
		{"q=literal&created_at[$gt]=2026-01-02T00:00:00.123456Z", 1, 1, "option-filter-22"},
		{"q=literal&created_at[$lt]=2026-01-02T00:00:00.123456Z", 1, 1, "option-filter-00"},
		{"q=literal&created_at[$ne]=2026-01-02T00:00:00.123456Z", 2, 2, "option-filter-00"},
		{"q=literal&updated_at[$gt]=2026-02-03T00:00:00.654321Z", 1, 1, "option-filter-22"},
		{"q=literal&updated_at[$lt]=2026-02-03T00:00:00.654321Z", 1, 1, "option-filter-00"},
		{"q=literal&updated_at[$gte]=2026-02-03T00:00:00.654321Z&updated_at[$lte]=2026-02-03T00:00:00.654321Z", 21, 20, "option-filter-01"},
		{"q=literal&created_at[$gt]=2026-01-03T00:00:00Z&created_at[$lt]=2026-01-01T00:00:00Z", 0, 0, ""},
		{"id=option-filter-exclusive", 0, 0, ""},
		{"id=option-filter-deleted", 0, 0, ""},
		{"is_exclusive=true&q=literal", 1, 1, "option-filter-exclusive"},
		{"with_deleted=true&q=literal", 24, 20, "option-filter-00"},
		{"with_deleted=true&deleted_at[$gte]=2020-01-01T00:00:00Z", 1, 1, "option-filter-deleted"},
		{"$or[0][id]=option-filter-00&$or[1][id]=option-filter-22", 2, 2, "option-filter-00"},
		{"$and[0][$or][0][id]=option-filter-00&$and[0][$or][1][id]=option-filter-22&$and[1][created_at][$gt]=2026-01-02T00:00:00.123456Z", 1, 1, "option-filter-22"},
		{"with_deleted=true&$or[0][id]=option-filter-exclusive&$or[1][id]=option-filter-deleted", 1, 1, "option-filter-deleted"},
	} {
		query, err := parseDefinitionQuery(test.raw)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := h.ListOptionDefinitions(ctx, actor, query)
		if err != nil || listed.Count != test.count || len(listed.Options) != test.size || test.size > 0 && listed.Options[0].ID != test.first {
			t.Fatalf("filtered list %q: %+v %v", test.raw, listed, err)
		}
	}
	for _, target := range []string{"option-filter-exclusive", "option-filter-deleted"} {
		if _, e := h.ReadOptionDefinition(ctx, target, actor); !errors.Is(e, gorm.ErrRecordNotFound) {
			t.Fatal("non-shared active detail escaped", target, e)
		}
		cmd := input("Must not write")
		if _, e := h.EditOptionDefinition(ctx, target, actor, cmd); !errors.Is(e, gorm.ErrRecordNotFound) {
			t.Fatal("non-shared active command escaped", target, e)
		}
	}
	t.Log(`OPTION_DEFINITION_SCOPE_NATIVE {"exclusive_filter":true,"deleted_date_filter":true,"count_page_same_scope":true,"nested_and_or":true,"default_scope_outside_or":true,"non_shared_commands_rejected":true}`)
	exec("DELETE FROM catalog_options WHERE id LIKE 'option-filter-%'")

	beforeMetadata := read()
	metadataInput := OptionDefinitionMetadataInput{OperationID: uuid.NewString(), ExpectedRevision: beforeMetadata.Revision, Metadata: json.RawMessage(`{"large":9007199254740993123,"object":{"keep":true},"remove":"old"}`)}
	metadataResult, e := h.EditOptionDefinitionMetadata(ctx, id, actor, metadataInput)
	if e != nil {
		t.Fatal(e)
	}
	metadataReplay, e := h.EditOptionDefinitionMetadata(ctx, id, actor, metadataInput)
	if e != nil || !metadataReplay.Replayed || metadataReplay.Revision != metadataResult.Revision {
		t.Fatal("metadata replay failed", e)
	}
	unchanged := read()
	if unchanged.MetadataJSON == nil || !strings.Contains(*unchanged.MetadataJSON, "9007199254740993123") || unchanged.Revision == beforeMetadata.Revision {
		t.Fatal("metadata precision/revision failed")
	}
	stale := metadataInput
	stale.OperationID = uuid.NewString()
	if _, e := h.EditOptionDefinitionMetadata(ctx, id, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale metadata accepted", e)
	}
	patch := OptionDefinitionMetadataInput{OperationID: uuid.NewString(), ExpectedRevision: unchanged.Revision, Metadata: json.RawMessage(`{"remove":"","flag":false,"null":null}`)}
	if _, e := h.EditOptionDefinitionMetadata(ctx, id, actor, patch); e != nil {
		t.Fatal(e)
	}
	merged := read()
	if *merged.MetadataJSON != `{"flag":false,"large":9007199254740993123,"null":null,"object":{"keep":true}}` {
		t.Fatal("native metadata shallow merge changed", *merged.MetadataJSON)
	}
	exec("CREATE TRIGGER metadata_receipt_fault BEFORE INSERT ON catalog_option_definition_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned metadata receipt fault'")
	faultMetadata := OptionDefinitionMetadataInput{OperationID: uuid.NewString(), ExpectedRevision: merged.Revision, Metadata: json.RawMessage(`null`)}
	if _, e := h.EditOptionDefinitionMetadata(ctx, id, actor, faultMetadata); e == nil {
		t.Fatal("metadata late SQL failure accepted")
	}
	if current := read(); current.Revision != merged.Revision || *current.MetadataJSON != *merged.MetadataJSON {
		t.Fatal("metadata write survived receipt rollback")
	}
	exec("DROP TRIGGER metadata_receipt_fault")
	if _, e := h.EditOptionDefinitionMetadata(ctx, id, actor, faultMetadata); e != nil {
		t.Fatal(e)
	}
	if read().MetadataJSON != nil {
		t.Fatal("native metadata null did not clear")
	}
	d := read()
	ids := map[string]string{}
	for _, v := range d.Values {
		ids[v.Value] = v.ID
	}
	if _, e := h.CreateOptionDefinition(ctx, actor, input("容量")); !errors.Is(e, ErrOptionDefinitionTitle) {
		t.Fatal("duplicate active shared title accepted", e)
	}
	edit := first
	edit.OperationID, edit.ExpectedRevision = uuid.NewString(), d.Revision
	edit.Title, edit.Values, edit.Ranks = "共享容量", []string{"256 GB", "128 GB", "512 GB"}, map[string]int{"256 GB": -1}
	if _, e := h.EditOptionDefinition(ctx, id, actor, edit); e != nil {
		t.Fatal(e)
	}
	d = read()
	if len(d.Values) != 3 || d.Values[0].ID != ids["256 GB"] || *d.Values[0].Rank != -1 || d.Values[1].ID != ids["128 GB"] || *d.Values[1].Rank != 7 || d.Values[2].Rank != nil {
		t.Fatal("existing identity/rank not preserved", d)
	}
	if _, e := h.EditOptionDefinition(ctx, id, actor, OptionDefinitionInput{OperationID: uuid.NewString(), ExpectedRevision: edit.ExpectedRevision, Title: edit.Title, Values: edit.Values}); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale revision accepted", e)
	}
	// The terminal receipt insert is real SQL: a failure must roll back the
	// title/rank/value writes performed earlier in the same transaction.
	exec("CREATE TRIGGER option_receipt_fault BEFORE INSERT ON catalog_option_definition_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned receipt fault'")
	fault := input("must-roll-back")
	if _, e := h.CreateOptionDefinition(ctx, actor, fault); e == nil {
		t.Fatal("late SQL failure accepted")
	}
	var n int64
	db.Table("catalog_options").Where("title=?", fault.Title).Count(&n)
	if n != 0 {
		t.Fatal("late SQL failure left a partial definition")
	}
	exec("DROP TRIGGER option_receipt_fault")
	// Membership alone protects a value even when the product has no SKU.
	product := "option-native-product"
	exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES(?,?,?,'draft',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", product, "Artificial", product)
	attach := func() (ImageResult, error) {
		state, e := h.ReadProductOptions(ctx, product, actor)
		if e != nil {
			return ImageResult{}, e
		}
		vid := ids["128 GB"]
		return h.EditProductOptions(ctx, product, actor, ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: state.Revision, Options: []OptionSelection{{ID: &id, Title: d.Title, Values: []OptionSelectionValue{{ID: &vid, Value: "128 GB"}}}}})
	}
	// Exercise the actual common mutex with a second SQL connection.
	blocker := db.Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	if _, e := lockOptionDefinitions(blocker, true); e != nil {
		blocker.Rollback()
		t.Fatal(e)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); _, e := attach(); done <- e }()
	<-started
	waitDeadline := time.Now().Add(5 * time.Second)
	for {
		var waits int64
		if e := db.Raw("SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=? AND l.OBJECT_NAME='catalog_command_capabilities'", name).Scan(&waits).Error; e != nil {
			blocker.Rollback()
			t.Fatal("lock wait inspection failed", e)
		}
		if waits > 0 {
			break
		}
		select {
		case e := <-done:
			blocker.Rollback()
			t.Fatal("product option command bypassed shared mutex", e)
		default:
		}
		if time.Now().After(waitDeadline) {
			blocker.Rollback()
			t.Fatal("native MySQL did not report the product membership lock wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e := blocker.Commit().Error; e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	d = read()
	remove := OptionDefinitionRemoveInput{OperationID: uuid.NewString(), ExpectedRevision: d.Revision}
	if _, e := h.RemoveOptionDefinition(ctx, id, ids["128 GB"], actor, remove); !errors.Is(e, ErrOptionDefinitionAssociated) {
		t.Fatal("product-associated value without SKU was deleted", e)
	}
	if _, e := h.RemoveOptionDefinition(ctx, id, "", actor, remove); !errors.Is(e, ErrOptionDefinitionAssociated) {
		t.Fatal("associated definition was deleted", e)
	}
	exec("UPDATE catalog_products SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", product)
	d = read()
	remove = OptionDefinitionRemoveInput{OperationID: uuid.NewString(), ExpectedRevision: d.Revision}
	if _, e := h.RemoveOptionDefinition(ctx, id, ids["128 GB"], actor, remove); !errors.Is(e, ErrOptionDefinitionAssociated) {
		t.Fatal("soft-deleted product membership must still guard single value", e)
	}
	deleted, e := h.RemoveOptionDefinition(ctx, id, "", actor, remove)
	if e != nil || deleted.Replayed {
		t.Fatal("whole option linked only to deleted product was not deleted", e)
	}
	exec("UPDATE catalog_command_capabilities SET enabled=FALSE WHERE command='product.option-definitions'")
	replay, e := h.RemoveOptionDefinition(ctx, id, "", actor, remove)
	if e != nil || !replay.Replayed || replay.Revision != deleted.Revision {
		t.Fatal("deleted definition receipt cannot recover after gate closes", e)
	}
	exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID)
	if _, e := h.RemoveOptionDefinition(ctx, id, "", actor, remove); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked actor recovered receipt", e)
	}
	if os.Getenv("OPTION_TEST_KEEP_DB") == "1" {
		// The controller owns this private instance and explicitly removes it
		// after the actual API/browser phase. No application test route is added.
		hash, e := bcrypt.GenerateFromPassword([]byte("artificial-option-browser-password"), bcrypt.DefaultCost)
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE admin_users SET enabled=TRUE,password_hash=? WHERE id=?", string(hash), actor.ID)
		exec("UPDATE catalog_command_capabilities SET enabled=TRUE WHERE command IN ('product.option-definitions','store.buy-policies')")
		exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES('scope-browser-exclusive','Scope browser product-specific',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at,deleted_at) VALUES('scope-browser-deleted','Scope browser deleted',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES('option-browser-product','Option browser product','option-browser-product','draft',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
		if os.Getenv("OPTION_TEST_ASTRO") == "1" {
			exec("INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('eur','€','€',2,0,'0','Euro',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			exec("INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('option-region','Option native region','eur',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			exec("INSERT INTO commerce_countries(iso_2,iso_3,num_code,name,display_name,region_id,created_at,updated_at) VALUES('dk','dnk','208','Denmark','Denmark','option-region',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			exec("INSERT INTO commerce_sales_channels(id,name,is_disabled,created_at,updated_at) VALUES('option-channel','Option native channel',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			exec("INSERT INTO commerce_stores(id,name,default_sales_channel_id,default_region_id,created_at,updated_at) VALUES('option-store','Option native store','option-channel','option-region',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			exec("INSERT INTO commerce_store_currencies(id,currency_code,is_default,store_id,created_at,updated_at) VALUES('option-store-eur','eur',TRUE,'option-store',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			for _, language := range []string{"en-GB", "zh-CN"} {
				exec("INSERT INTO commerce_store_locales(id,locale_code,store_id,created_at,updated_at) VALUES(?,?,'option-store',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "option-locale-"+language, language)
			}
			exec("UPDATE catalog_products SET status='published' WHERE id='option-browser-product'")
			exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES('option-browser-product-two','Second option browser product','option-browser-product-two','published',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
			for _, product := range []string{"option-browser-product", "option-browser-product-two"} {
				exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) VALUES(?,?,'option-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "channel-"+product, product)
			}
			exec("UPDATE catalog_variant_create_runtime SET enabled=TRUE WHERE id=1")
			exec("UPDATE product_review_runtime SET reads_enabled=TRUE WHERE id=1")
			seedPriceListBrowser(t, db)
			seedCollectionBrowser(t, db)
			// Reuse the native keyword fixture in the same trusted public store.
			exec("INSERT INTO catalog_product_channels(id,product_id,sales_channel_id,created_at,updated_at) SELECT CONCAT(id,'-astro'),id,'option-channel',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM catalog_products WHERE id LIKE 'search-english-%'")
			exec("UPDATE catalog_products SET thumbnail='/iphone-thumbnails/iphone-16e/white.webp' WHERE id LIKE 'search-english-%'")
		}
		keepForBrowser = true
	}
	verifyRelatedProductCandidates(t, db)
	t.Log(`OPTION_DEFINITIONS_NATIVE {"migrations":97,"concurrent_create":6,"fresh_create":1,"identity_and_rank_preserved":true,"stale_revision_rejected":true,"real_sql_late_failure_atomic":true,"product_membership_mutex_observed":true,"no_sku_membership_blocks_deletion":true,"source_soft_deleted_product_rules":true,"deleted_receipt_gate_off_replay":true,"current_actor_required_on_replay":true,"real_provider_called":false}`)
}
