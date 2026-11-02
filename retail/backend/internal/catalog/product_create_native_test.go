package catalog

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNativeProductCreationTransactions(t *testing.T) {
	if os.Getenv("PRODUCT_CREATE_NATIVE_REHEARSAL") != "1" {
		t.Skip("dedicated native MySQL required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("native connection failed")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var name string
	if db.Raw("SELECT DATABASE()").Scan(&name).Error != nil || !strings.HasPrefix(name, "eurorefurb_product_create_") {
		t.Fatal("owned fixture required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("fixture admin required")
	}
	h := Handler{DB: db}
	directory, e := h.ReadProductCreate(ctx, actor)
	if e != nil || len(directory.Channels) == 0 || len(directory.ShippingProfiles) == 0 {
		t.Fatal("directory read failed", e)
	}
	input := func() ProductCreateInput {
		v := createInputFixture()
		v.OperationID = uuid.NewString()
		v.Handle = "native-test-" + uuid.NewString()
		v.SalesChannelIDs = []string{directory.Channels[0].ID}
		v.ShippingProfileID = directory.ShippingProfiles[0].ID
		v.ImageURLs = []string{"https://example.test/image.webp"}
		return v
	}
	setEnabled := func(v bool) {
		if db.Exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='product.create'", v).Error != nil {
			t.Fatal("fixture gate failed")
		}
	}
	count := func(table string) int64 {
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("fixture count failed")
		}
		return n
	}
	setEnabled(false)
	in := input()
	before := count("catalog_products")
	if _, e := h.CreateProduct(ctx, actor, in); !errors.Is(e, ErrSchema) || count("catalog_products") != before {
		t.Fatal("disabled first write accepted")
	}
	setEnabled(true)
	results := make([]ProductCreateResult, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.CreateProduct(ctx, actor, in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.ID != results[0].ID || r.Revision != results[0].Revision {
			t.Fatal("concurrent idempotent create failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 || count("catalog_products") != before+1 {
		t.Fatal("concurrent creation duplicated product")
	}
	id := results[0].ID
	for table, want := range map[string]int64{"catalog_product_options": 1, "catalog_images": 1, "catalog_product_channels": 1, "catalog_product_shipping_profiles": 1} {
		var n int64
		if db.Table(table).Where("product_id=?", id).Count(&n).Error != nil || n != want {
			t.Fatal("atomic product child missing", table)
		}
	}
	var values int64
	if db.Raw("SELECT COUNT(*) FROM catalog_product_option_values v JOIN catalog_product_options o ON o.id=v.product_product_option_id WHERE o.product_id=?", id).Scan(&values).Error != nil || values != 2 {
		t.Fatal("option membership missing")
	}
	duplicate := input()
	duplicate.Handle = in.Handle
	if _, e := h.CreateProduct(ctx, actor, duplicate); !errors.Is(e, ErrEditConflict) || count("catalog_products") != before+1 {
		t.Fatal("duplicate handle accepted")
	}
	changed := in
	changed.Title = "different"
	if _, e := h.CreateProduct(ctx, actor, changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed replay accepted")
	}
	missing := input()
	missing.ShippingProfileID = "missing-profile"
	if _, e := h.CreateProduct(ctx, actor, missing); e == nil || count("catalog_products") != before+1 {
		t.Fatal("missing parent accepted")
	}
	setEnabled(false)
	if r, e := h.CreateProduct(ctx, actor, in); e != nil || !r.Replayed {
		t.Fatal("disabled gate prevented receipt recovery")
	}
	setEnabled(true)
	tables := []string{"catalog_products", "catalog_options", "catalog_option_values", "catalog_product_options", "catalog_product_option_values", "catalog_product_edits", "catalog_images"}
	counts := map[string]int64{}
	for _, table := range tables {
		counts[table] = count(table)
	}
	failName := "product_create_test_receipt_failure"
	if db.Callback().Create().Before("gorm:create").Register(failName, func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_product_edits" {
			tx.AddError(errors.New("owned fixture late failure"))
		}
	}) != nil {
		t.Fatal("failure injection unavailable")
	}
	_, e = h.CreateProduct(ctx, actor, input())
	db.Callback().Create().Remove(failName)
	if e == nil {
		t.Fatal("late failure ignored")
	}
	for _, table := range tables {
		if count(table) != counts[table] {
			t.Fatal("partial create escaped transaction", table)
		}
	}
	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID).Error != nil {
		t.Fatal("fixture revocation failed")
	}
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.CreateProduct(ctx, actor, in); e == nil {
		t.Fatal("revoked permission disclosed receipt")
	}
}
