package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestNativeProductImages(t *testing.T) {
	if os.Getenv("PRODUCT_IMAGES_NATIVE_REHEARSAL") != "1" {
		t.Skip("owned native database required")
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
		t.Fatal("owned database required")
	}
	ctx := context.Background()
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("owned admin unavailable")
	}
	h := Handler{DB: db}
	dirs, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	create := createInputFixture()
	create.OperationID = uuid.NewString()
	create.Handle = "image-native-" + uuid.NewString()
	create.SalesChannelIDs = []string{dirs.Channels[0].ID}
	create.ShippingProfileID = dirs.ShippingProfiles[0].ID
	result, e := h.CreateProduct(ctx, actor, create)
	if e != nil {
		t.Fatal("fixture product failed", e)
	}
	p := result.ID
	gate := func(v bool) {
		if db.Exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='product.images'", v).Error != nil {
			t.Fatal("fixture gate failed")
		}
	}
	state := func() ImageContext {
		out, e := h.ReadImages(ctx, p, actor)
		if e != nil {
			t.Fatal("image context failed", e)
		}
		return out
	}
	input := imageInputFixture()
	input.OperationID = uuid.NewString()
	input.ExpectedRevision = state().Revision
	gate(false)
	if _, e := h.EditImages(ctx, p, actor, input); !errors.Is(e, ErrSchema) {
		t.Fatal("disabled first write accepted")
	}
	gate(true)
	var wg sync.WaitGroup
	results := make([]ImageResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.EditImages(ctx, p, actor, input) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Revision != results[0].Revision {
			t.Fatal("concurrent image replay failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("upload duplicated")
	}
	after := state()
	if len(after.Images) != 1 || after.Thumbnail == nil || *after.Thumbnail != after.Images[0].URL {
		t.Fatal("upload/thumbnail not atomic")
	}
	imageID := after.Images[0].ID
	asset := func() *httptest.ResponseRecorder {
		r := gin.New()
		r.GET("/api/store/catalog-images/:image_id", h.ImageAssetHTTP)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", after.Images[0].URL, nil))
		return w
	}
	w := asset()
	expected, _ := base64.StdEncoding.DecodeString(input.Upload.Content)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("X-Content-Type-Options") != "nosniff" || !bytes.Equal(w.Body.Bytes(), expected) {
		t.Fatal("public bytes differ")
	}
	changed := input
	changed.Upload = &ImageUpload{Filename: "other.png", MimeType: input.Upload.MimeType, Content: input.Upload.Content}
	if _, e := h.EditImages(ctx, p, actor, changed); !errors.Is(e, ErrEditConflict) {
		t.Fatal("changed upload replay accepted")
	}
	stale := input
	stale.OperationID = uuid.NewString()
	if _, e := h.EditImages(ctx, p, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale revision accepted")
	}
	// Late receipt failure must undo the newly inserted asset and its image row.
	var beforeAssets, beforeImages int64
	db.Table("catalog_image_assets").Count(&beforeAssets)
	db.Table("catalog_images").Count(&beforeImages)
	late := input
	late.OperationID = uuid.NewString()
	late.ExpectedRevision = after.Revision
	late.Images = []ImageEntry{{ID: &imageID, URL: after.Images[0].URL}}
	db.Callback().Create().Before("gorm:create").Register("image_test_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_image_edits" {
			tx.AddError(errors.New("fixture late failure"))
		}
	})
	_, e = h.EditImages(ctx, p, actor, late)
	db.Callback().Create().Remove("image_test_failure")
	if e == nil {
		t.Fatal("late failure ignored")
	}
	var n int64
	db.Table("catalog_image_assets").Count(&n)
	if n != beforeAssets {
		t.Fatal("orphan blob survived rollback")
	}
	db.Table("catalog_images").Count(&n)
	if n != beforeImages || state().Revision != after.Revision {
		t.Fatal("partial gallery survived rollback")
	}
	// Existing independent thumbnails survive gallery additions unless explicitly cleared.
	standalone := "https://example.test/standalone.png"
	db.Table("catalog_products").Where("id=?", p).Update("thumbnail", standalone)
	preserve := -1
	edit := ImageEditInput{OperationID: uuid.NewString(), ExpectedRevision: state().Revision, Images: []ImageEntry{{ID: &imageID, URL: after.Images[0].URL}, {URL: "https://example.test/extra.png"}}, ThumbnailIndex: &preserve}
	if _, e := h.EditImages(ctx, p, actor, edit); e != nil || *state().Thumbnail != standalone {
		t.Fatal("standalone thumbnail lost", e)
	}
	// A catalog image associated with a SKU cannot silently disappear.
	variant := "variant_" + uuid.NewString()
	if db.Exec("INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES(?,?,'Fixture',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", variant, p).Error != nil || db.Exec("INSERT INTO catalog_variant_images(id,variant_id,image_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "vi_"+uuid.NewString(), variant, imageID).Error != nil {
		t.Fatal("fixture SKU image failed")
	}
	remove := ImageEditInput{OperationID: uuid.NewString(), ExpectedRevision: state().Revision, Images: []ImageEntry{}, ThumbnailIndex: nil}
	if _, e := h.EditImages(ctx, p, actor, remove); !errors.Is(e, ErrImageInUse) {
		t.Fatal("SKU image removed silently")
	}
	db.Table("catalog_variant_images").Where("variant_id=?", variant).Update("deleted_at", gorm.Expr("UTC_TIMESTAMP(6)"))
	db.Table("catalog_variants").Where("id=?", variant).Update("thumbnail", after.Images[0].URL)
	remove.ExpectedRevision = state().Revision
	if _, e := h.EditImages(ctx, p, actor, remove); !errors.Is(e, ErrImageInUse) {
		t.Fatal("SKU thumbnail removed silently")
	}
	db.Table("catalog_variants").Where("id=?", variant).Update("thumbnail", nil)
	remove.ExpectedRevision = state().Revision
	if _, e := h.EditImages(ctx, p, actor, remove); e != nil || len(state().Images) != 0 || state().Thumbnail != nil {
		t.Fatal("remove/clear failed", e)
	}
	if asset().Code != 404 {
		t.Fatal("removed image still public")
	}
	gate(false)
	if r, e := h.EditImages(ctx, p, actor, remove); e != nil || !r.Replayed {
		t.Fatal("disabled receipt recovery failed")
	}
	gate(true)
	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID).Error != nil {
		t.Fatal("fixture revoke failed")
	}
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.EditImages(ctx, p, actor, remove); e == nil {
		t.Fatal("revoked permission disclosed receipt")
	}
}
