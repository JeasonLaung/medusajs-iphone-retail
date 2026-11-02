package catalog

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNativeVariantImageTransactions(t *testing.T) {
	if os.Getenv("VARIANT_IMAGES_NATIVE_REHEARSAL") != "1" {
		t.Skip("owned native MySQL required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("native fixture unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var name string
	db.Raw("SELECT DATABASE()").Scan(&name)
	if !strings.HasPrefix(name, "eurorefurb_product_create_") {
		t.Fatal("owned fixture required")
	}
	ctx := context.Background()
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("owned admin unavailable")
	}
	h := Handler{DB: db}
	directory, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	product := func() string {
		in := createInputFixture()
		in.OperationID = uuid.NewString()
		in.Handle = "variant-image-native-" + uuid.NewString()
		in.SalesChannelIDs = []string{directory.Channels[0].ID}
		in.ShippingProfileID = directory.ShippingProfiles[0].ID
		in.ImageURLs = []string{"https://example.test/a.png", "https://example.test/b.png"}
		out, e := h.CreateProduct(ctx, actor, in)
		if e != nil {
			t.Fatal("fixture product failed", e)
		}
		return out.ID
	}
	p, foreign := product(), product()
	variant := "variant_" + uuid.NewString()
	other := "variant_" + uuid.NewString()
	standalone := "https://example.test/standalone.png"
	for _, v := range []string{variant, other} {
		if db.Exec("INSERT INTO catalog_variants(id,product_id,title,thumbnail,manage_inventory,allow_backorder,created_at,updated_at) VALUES(?,?,'Fixture',?,TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", v, p, standalone).Error != nil {
			t.Fatal("fixture variant failed")
		}
	}
	state := func() VariantImageContext {
		out, e := h.ReadVariantImages(ctx, p, variant, actor)
		if e != nil {
			t.Fatal("SKU image context failed", e)
		}
		return out
	}
	gate := func(v bool) {
		if db.Exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='product.images'", v).Error != nil {
			t.Fatal("fixture gate failed")
		}
	}
	before := state()
	first, second := before.Images[0].ID, before.Images[1].ID
	in := variantImageFixture()
	in.OperationID = uuid.NewString()
	in.ExpectedRevision = before.Revision
	in.ImageIDs = []string{first}
	gate(false)
	if _, e := h.EditVariantImages(ctx, p, variant, actor, in); !errors.Is(e, ErrSchema) {
		t.Fatal("disabled first write accepted")
	}
	gate(true)
	var wg sync.WaitGroup
	results := make([]ImageResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.EditVariantImages(ctx, p, variant, actor, in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Revision != results[0].Revision {
			t.Fatal("concurrent SKU image operation failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("duplicated association")
	}
	after := state()
	if len(after.SelectedIDs) != 1 || after.SelectedIDs[0] != first || after.Thumbnail == nil || *after.Thumbnail != standalone {
		t.Fatal("association or independent thumbnail not preserved")
	}
	if _, e := h.ReadVariantImages(ctx, foreign, variant, actor); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("foreign product SKU readable")
	}
	if _, e := h.EditVariantImages(ctx, p, other, actor, in); !errors.Is(e, ErrEditConflict) {
		t.Fatal("operation reused across SKUs")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	stale.ImageIDs = []string{second}
	if _, e := h.EditVariantImages(ctx, p, variant, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale association accepted")
	}
	foreignImages, e := h.ReadImages(ctx, foreign, actor)
	if e != nil {
		t.Fatal(e)
	}
	bad := in
	bad.OperationID = uuid.NewString()
	bad.ExpectedRevision = after.Revision
	bad.ImageIDs = []string{foreignImages.Images[0].ID}
	if _, e := h.EditVariantImages(ctx, p, variant, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("foreign image accepted")
	}
	edit := VariantImageInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, ImageIDs: []string{first, second}, ThumbnailImageID: &second}
	if _, e := h.EditVariantImages(ctx, p, variant, actor, edit); e != nil {
		t.Fatal("thumbnail selection failed", e)
	}
	after = state()
	if *after.Thumbnail != before.Images[1].URL || len(after.SelectedIDs) != 2 {
		t.Fatal("thumbnail does not match selected image")
	}
	otherState, e := h.ReadVariantImages(ctx, p, other, actor)
	if e != nil || len(otherState.SelectedIDs) != 0 || *otherState.Thumbnail != standalone {
		t.Fatal("other SKU changed")
	}
	var countBefore int64
	db.Table("catalog_image_edits").Count(&countBefore)
	clear := VariantImageInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, ImageIDs: []string{}}
	db.Callback().Create().Before("gorm:create").Register("variant_image_test_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_image_edits" {
			tx.AddError(errors.New("fixture receipt failure"))
		}
	})
	_, e = h.EditVariantImages(ctx, p, variant, actor, clear)
	db.Callback().Create().Remove("variant_image_test_failure")
	var countAfter int64
	db.Table("catalog_image_edits").Count(&countAfter)
	if e == nil || state().Revision != after.Revision || countBefore != countAfter {
		t.Fatal("partial image unlink escaped rollback")
	}
	// Clear associations while explicitly keeping the current thumbnail. Product
	// deletion must remain blocked until that thumbnail is explicitly cleared.
	keep := clear
	keep.OperationID = uuid.NewString()
	keep.KeepThumbnail = true
	if _, e := h.EditVariantImages(ctx, p, variant, actor, keep); e != nil {
		t.Fatal("unlink preserving thumbnail failed", e)
	}
	gallery, e := h.ReadImages(ctx, p, actor)
	if e != nil {
		t.Fatal(e)
	}
	remove := ImageEditInput{OperationID: uuid.NewString(), ExpectedRevision: gallery.Revision, Images: []ImageEntry{}}
	if _, e := h.EditImages(ctx, p, actor, remove); !errors.Is(e, ErrImageInUse) {
		t.Fatal("SKU thumbnail failed to protect gallery")
	}
	clear.OperationID = uuid.NewString()
	clear.ExpectedRevision = state().Revision
	if _, e := h.EditVariantImages(ctx, p, variant, actor, clear); e != nil || state().Thumbnail != nil || len(state().SelectedIDs) != 0 {
		t.Fatal("explicit clear failed", e)
	}
	gallery, e = h.ReadImages(ctx, p, actor)
	if e != nil {
		t.Fatal(e)
	}
	remove.ExpectedRevision = gallery.Revision
	if _, e := h.EditImages(ctx, p, actor, remove); e != nil {
		t.Fatal("gallery remains blocked after explicit unlink and clear", e)
	}
	var active, historical int64
	db.Table("catalog_variant_images").Where("variant_id=? AND deleted_at IS NULL", variant).Count(&active)
	db.Table("catalog_variant_images").Where("variant_id=? AND deleted_at IS NOT NULL", variant).Count(&historical)
	if active != 0 || historical != 2 {
		t.Fatal("association history not preserved")
	}
	gate(false)
	if replay, e := h.EditVariantImages(ctx, p, variant, actor, clear); e != nil || !replay.Replayed {
		t.Fatal("closed-gate receipt recovery failed")
	}
	gate(true)
	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID).Error != nil {
		t.Fatal("fixture revoke failed")
	}
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.EditVariantImages(ctx, p, variant, actor, clear); e == nil {
		t.Fatal("revoked permission disclosed image receipt")
	}
}

func TestNativeCreatedIphoneVariantImages(t *testing.T) {
	if os.Getenv("VARIANT_IMAGES_NATIVE_REHEARSAL") != "1" {
		t.Skip("owned native MySQL required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("fixture unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var name string
	db.Raw("SELECT DATABASE()").Scan(&name)
	if !strings.HasPrefix(name, "eurorefurb_product_create_") {
		t.Fatal("owned fixture required")
	}
	ctx := context.Background()
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("fixture admin unavailable")
	}
	h := Handler{DB: db}
	directory, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	var prior struct{ Enabled bool }
	if db.Table("catalog_command_capabilities").Where("command='product.images'").Take(&prior).Error != nil {
		t.Fatal("fixture image gate unavailable")
	}
	db.Exec("UPDATE catalog_command_capabilities SET enabled=FALSE WHERE command='product.images'")
	defer db.Exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='product.images'", prior.Enabled)
	in := createInputFixture()
	in.OperationID = uuid.NewString()
	in.Handle = "swappie-iphone-native-" + uuid.NewString()[:8]
	in.SalesChannelIDs = []string{directory.Channels[0].ID}
	in.ShippingProfileID = directory.ShippingProfiles[0].ID
	model := strings.TrimPrefix(in.Handle, "swappie-")
	base := "https://example.test/iphone-products/" + model
	in.ImageURLs = []string{base + "/red/02.webp", base + "/red/01.webp", base + "/black/01.webp"}
	in.Options = []ProductCreateOption{{Title: "Color", Values: []string{"Product (RED)", "Black"}}}
	p, e := h.CreateProduct(ctx, actor, in)
	if e != nil {
		t.Fatal("fixture product failed", e)
	}
	creation, e := h.ReadVariantCreate(ctx, actor, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	red, black := "", ""
	for _, o := range creation.Options {
		for _, v := range o.Values {
			if v.Value == "Product (RED)" {
				red = v.ID
			}
			if v.Value == "Black" {
				black = v.ID
			}
		}
	}
	if red == "" || black == "" {
		t.Fatal("fixture colors missing")
	}
	sku := VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: creation.Revision, Title: "Red", SKU: "", OptionValueIDs: []string{red}}
	created, e := h.CreateVariant(ctx, actor, p.ID, sku)
	if e != nil {
		t.Fatal("native source image assignment failed", e)
	}
	gallery, e := h.ReadVariantImages(ctx, p.ID, created.ID, actor)
	if e != nil || len(gallery.SelectedIDs) != 2 || gallery.Thumbnail == nil || *gallery.Thumbnail != base+"/red/01.webp" {
		t.Fatal("source color filter/thumbnail order differs", e)
	}
	allowed := map[string]bool{}
	for _, i := range gallery.Images {
		if strings.Contains(i.URL, "/red/") {
			allowed[i.ID] = true
		}
	}
	for _, id := range gallery.SelectedIDs {
		if !allowed[id] {
			t.Fatal("wrong color image attached")
		}
	}
	replay, e := h.CreateVariant(ctx, actor, p.ID, sku)
	if e != nil || !replay.Replayed || replay.ID != created.ID {
		t.Fatal("SKU replay duplicated auto images")
	}
	counts := map[string]int64{}
	for _, table := range []string{"catalog_variants", "catalog_variant_images", "inventory_items", "catalog_variant_inventory_items", "catalog_variant_creations"} {
		var n int64
		db.Table(table).Count(&n)
		counts[table] = n
	}
	creation, e = h.ReadVariantCreate(ctx, actor, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	blackSKU := VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: creation.Revision, Title: "Black", SKU: "", OptionValueIDs: []string{black}}
	db.Callback().Create().Before("gorm:create").Register("iphone_variant_late_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_variant_creations" {
			tx.AddError(errors.New("fixture final receipt failure"))
		}
	})
	_, e = h.CreateVariant(ctx, actor, p.ID, blackSKU)
	db.Callback().Create().Remove("iphone_variant_late_failure")
	if e == nil {
		t.Fatal("late SKU failure ignored")
	}
	for table, n := range counts {
		var now int64
		db.Table(table).Count(&now)
		if now != n {
			t.Fatal("auto image or inventory row escaped rollback", table)
		}
	}
}
