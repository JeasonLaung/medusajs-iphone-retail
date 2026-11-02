package catalog

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestNativeVariantEditTransactions(t *testing.T) {
	if os.Getenv("VARIANT_EDIT_NATIVE_REHEARSAL") != "1" {
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
		t.Fatal("owned admin required")
	}
	h := Handler{DB: db}
	exec := func(q string, args ...any) {
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("fixture setup failed", e)
		}
	}
	dirs, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	createProduct := func() string {
		in := createInputFixture()
		in.OperationID = uuid.NewString()
		in.Handle = "swappie-iphone-edit-" + uuid.NewString()
		in.SalesChannelIDs = []string{dirs.Channels[0].ID}
		in.ShippingProfileID = dirs.ShippingProfiles[0].ID
		in.Options = []ProductCreateOption{{Title: "Color", Values: []string{"Product (RED)", "Black", "Blue"}}, {Title: "Storage", Values: []string{"128 GB", "256 GB"}}}
		base := "https://example.test/iphone-products/" + strings.TrimPrefix(in.Handle, "swappie-")
		in.ImageURLs = []string{base + "/red/02.webp", base + "/red/01.webp", base + "/black/02.webp", base + "/black/01.webp"}
		out, e := h.CreateProduct(ctx, actor, in)
		if e != nil {
			t.Fatal(e)
		}
		return out.ID
	}
	p, foreign := createProduct(), createProduct()
	value := func(id, label string) string {
		c, e := h.ReadVariantCreate(ctx, actor, id)
		if e != nil {
			t.Fatal(e)
		}
		for _, o := range c.Options {
			for _, v := range o.Values {
				if v.Value == label {
					return v.ID
				}
			}
		}
		t.Fatal("missing fixture value")
		return ""
	}
	createVariant := func(id, color, storage string) VariantCreateResult {
		c, e := h.ReadVariantCreate(ctx, actor, id)
		if e != nil {
			t.Fatal(e)
		}
		out, e := h.CreateVariant(ctx, actor, id, VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: c.Revision, Title: color + " " + storage, SKU: "", OptionValueIDs: []string{value(id, color), value(id, storage)}})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	first, second := createVariant(p, "Product (RED)", "128 GB"), createVariant(p, "Black", "128 GB")
	state := func(id string) VariantEditContext {
		out, e := h.ReadVariantEdit(ctx, p, id, actor)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	cmd := func(c VariantEditContext) VariantEditInput {
		return VariantEditInput{VariantFields: c.Variant, OperationID: uuid.NewString(), ExpectedRevision: c.Revision, OptionValueIDs: append([]string{}, c.SelectedIDs...)}
	}
	gate := func(on bool) {
		exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='variant.edit'", on)
	}
	// Real stock, reservation, device-tracking and exact-price facts must survive edits.
	location := "sloc_" + uuid.NewString()
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES(?,'Owned SKU edit fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", location)
	exec("INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,raw_stocked_quantity,raw_reserved_quantity,raw_incoming_quantity,created_at,updated_at) VALUES(?,?,?,7.25,1.25,0,'{\"value\":\"7.25\",\"precision\":38}','{\"value\":\"1.25\",\"precision\":38}','{\"value\":\"0\",\"precision\":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "ilvl_"+uuid.NewString(), first.InventoryItemID, location)
	exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,raw_quantity,created_at,updated_at) VALUES(?,?,?,1.25,'{\"value\":\"1.25\",\"precision\":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "res_"+uuid.NewString(), first.InventoryItemID, location)
	source, device := "source_"+uuid.NewString(), "device_"+uuid.NewString()
	exec("INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES(?,'Owned fixture','self',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", source, location)
	exec("INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "dvt_"+uuid.NewString(), first.ID, first.InventoryItemID)
	exec("INSERT INTO devices(id,serial_number,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,?, ?,?,100,'eur','','','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", device, "OWNED-"+uuid.NewString(), first.ID, source, location)
	priceContext, e := h.ReadPriceCreate(ctx, actor, p, first.ID)
	if e != nil {
		t.Fatal(e)
	}
	price, e := h.CreateBasePrice(ctx, actor, p, first.ID, PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: priceContext.Revision, CurrencyCode: "eur", Amount: "129.123456789012345678"})
	if e != nil {
		t.Fatal(e)
	}
	fingerprint := func() string {
		facts := map[string]any{}
		for table, where := range map[string]string{"inventory_items": "id", "inventory_levels": "inventory_item_id", "inventory_reservations": "inventory_item_id"} {
			var rows []map[string]any
			if db.Table(table).Where(where+"=?", first.InventoryItemID).Order("id").Find(&rows).Error != nil {
				t.Fatal("facts unreadable")
			}
			facts[table] = rows
		}
		for _, table := range []string{"device_variant_tracking", "devices"} {
			var rows []map[string]any
			if db.Table(table).Where("variant_id=?", first.ID).Order("id").Find(&rows).Error != nil {
				t.Fatal("device facts unreadable")
			}
			facts[table] = rows
		}
		var prices []map[string]any
		db.Table("pricing_prices").Where("id=?", price.ID).Find(&prices)
		facts["price"] = prices
		return imageDigest(facts)
	}
	preserved := fingerprint()
	before := state(first.ID)
	otherBefore := state(second.ID)
	in := cmd(before)
	in.Title = "Edited Black 256 GB"
	in.SKU = nil
	in.OptionValueIDs = []string{value(p, "Black"), value(p, "256 GB")}
	barcode, ean, upc, material, hs, mid, origin := "0000-"+uuid.NewString(), "0001-"+uuid.NewString(), "0002-"+uuid.NewString(), "钛金属", "851713", "MID-FIXTURE", "dk"
	in.Barcode = &barcode
	in.EAN = &ean
	in.UPC = &upc
	in.Material = &material
	in.HSCode = &hs
	in.MidCode = &mid
	in.OriginCountry = &origin
	weight, length, height, width := 187.25, 146.5, 0.0, 70.625
	in.Weight = &weight
	in.Length = &length
	in.Height = &height
	in.Width = &width
	gate(false)
	if _, e := h.EditVariant(ctx, p, first.ID, actor, in); !errors.Is(e, ErrSchema) {
		t.Fatal("disabled write accepted")
	}
	gate(true)
	var wg sync.WaitGroup
	results := make([]ImageResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.EditVariant(ctx, p, first.ID, actor, in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Revision != results[0].Revision {
			t.Fatal("concurrent SKU save failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("duplicated SKU write")
	}
	after := state(first.ID)
	if after.Variant.SKU == nil || *after.Variant.SKU != first.SKU || after.Variant.Title != in.Title || after.Variant.Barcode == nil || *after.Variant.Barcode != barcode || after.Variant.Height == nil || *after.Variant.Height != 0 || after.Variant.Weight == nil || *after.Variant.Weight != weight || after.Variant.Material == nil || *after.Variant.Material != material {
		t.Fatal("variant fields or generated identity changed")
	}
	if state(second.ID).Revision != otherBefore.Revision || fingerprint() != preserved {
		t.Fatal("another SKU, exact price, quantity or device facts changed")
	}
	images, e := h.ReadVariantImages(ctx, p, first.ID, actor)
	if e != nil || len(images.SelectedIDs) != 2 || images.Thumbnail == nil || !strings.Contains(*images.Thumbnail, "/black/01.webp") {
		t.Fatal("color images not synchronized", e)
	}
	if _, e := h.ReadVariantEdit(ctx, foreign, first.ID, actor); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("foreign SKU readable")
	}
	if _, e := h.EditVariant(ctx, p, second.ID, actor, in); !errors.Is(e, ErrEditConflict) {
		t.Fatal("cross-SKU receipt accepted")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := h.EditVariant(ctx, p, first.ID, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale save accepted")
	}
	bad := cmd(after)
	bad.OptionValueIDs = []string{value(p, "Black"), value(p, "128 GB")}
	if _, e := h.EditVariant(ctx, p, first.ID, actor, bad); !errors.Is(e, ErrEditConflict) {
		t.Fatal("duplicate combination accepted")
	}
	exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES(?,?)", second.ID, value(foreign, "Blue"))
	if _, e := h.EditVariant(ctx, p, first.ID, actor, bad); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale value bypassed source collision rule")
	}
	exec("DELETE FROM catalog_variant_options WHERE variant_id=? AND option_value_id=?", second.ID, value(foreign, "Blue"))
	bad = cmd(after)
	bad.OptionValueIDs = []string{value(p, "Black")}
	if _, e := h.EditVariant(ctx, p, first.ID, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("incomplete options accepted")
	}
	bad = cmd(after)
	bad.OptionValueIDs = []string{value(foreign, "Black"), value(p, "128 GB")}
	if _, e := h.EditVariant(ctx, p, first.ID, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("foreign value accepted")
	}
	taken := "taken-" + uuid.NewString()
	exec("UPDATE catalog_variants SET barcode=?,ean=?,upc=? WHERE id=?", taken, taken, taken, second.ID)
	for _, key := range []string{"sku", "barcode", "ean", "upc"} {
		bad = cmd(after)
		switch key {
		case "sku":
			bad.SKU = &second.SKU
		case "barcode":
			bad.Barcode = &taken
		case "ean":
			bad.EAN = &taken
		case "upc":
			bad.UPC = &taken
		}
		if _, e := h.EditVariant(ctx, p, first.ID, actor, bad); !errors.Is(e, ErrVariantCodeConflict) {
			t.Fatal("duplicate code accepted", key, e)
		}
	}
	// Fail after options, image, inventory and number-counter writes.
	rollback := cmd(after)
	rollback.Title = "Must roll back"
	rollback.OptionValueIDs = []string{value(p, "Product (RED)"), value(p, "256 GB")}
	newSKU := "ERP899999"
	rollback.SKU = &newSKU
	var counterBefore map[string]any
	db.Table("catalog_variant_create_runtime").Take(&counterBefore)
	db.Callback().Create().Before("gorm:create").Register("variant_edit_receipt_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_variant_edits" {
			tx.AddError(errors.New("fixture late receipt failure"))
		}
	})
	_, e = h.EditVariant(ctx, p, first.ID, actor, rollback)
	db.Callback().Create().Remove("variant_edit_receipt_fail")
	if e == nil {
		t.Fatal("late failure ignored")
	}
	var counterAfter map[string]any
	db.Table("catalog_variant_create_runtime").Take(&counterAfter)
	if state(first.ID).Revision != after.Revision || imageDigest(counterBefore) != imageDigest(counterAfter) || fingerprint() != preserved {
		t.Fatal("variant, options, images, counter or facts escaped rollback")
	}
	// Source subscriber deliberately keeps current images when no new color matches.
	blue := cmd(after)
	blue.OptionValueIDs = []string{value(p, "Blue"), value(p, "256 GB")}
	if _, e := h.EditVariant(ctx, p, first.ID, actor, blue); e != nil {
		t.Fatal(e)
	}
	noMatch, e := h.ReadVariantImages(ctx, p, first.ID, actor)
	if e != nil || noMatch.Revision != images.Revision {
		t.Fatal("unmatched color destroyed manual/current gallery")
	}
	// Native core inventory-toggle behavior is checked on a separate untracked SKU.
	toggleBefore := state(second.ID)
	off := cmd(toggleBefore)
	off.ManageInventory = false
	if _, e := h.EditVariant(ctx, p, second.ID, actor, off); e != nil {
		t.Fatal(e)
	}
	offState := state(second.ID)
	if offState.InventoryLinks != 0 || offState.Variant.ManageInventory {
		t.Fatal("inventory links not dismissed")
	}
	on := cmd(offState)
	on.ManageInventory = true
	if _, e := h.EditVariant(ctx, p, second.ID, actor, on); !errors.Is(e, ErrInventoryCodeConflict) {
		t.Fatal("existing merchant inventory taken on enable", e)
	}
	if state(second.ID).Revision != offState.Revision {
		t.Fatal("inventory conflict partly committed")
	}
	replacementSKU := "CUSTOM-" + uuid.NewString()
	on.SKU = &replacementSKU
	var itemCount, linkCount int64
	db.Table("inventory_items").Count(&itemCount)
	db.Table("catalog_variant_inventory_items").Count(&linkCount)
	db.Callback().Create().Before("gorm:create").Register("variant_enable_receipt_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_variant_edits" {
			tx.AddError(errors.New("fixture late inventory receipt failure"))
		}
	})
	_, e = h.EditVariant(ctx, p, second.ID, actor, on)
	db.Callback().Create().Remove("variant_enable_receipt_fail")
	if e == nil {
		t.Fatal("inventory late failure ignored")
	}
	var itemsAfter, linksAfter int64
	db.Table("inventory_items").Count(&itemsAfter)
	db.Table("catalog_variant_inventory_items").Count(&linksAfter)
	if itemsAfter != itemCount || linksAfter != linkCount || state(second.ID).Revision != offState.Revision {
		t.Fatal("default inventory escaped rollback")
	}
	if _, e := h.EditVariant(ctx, p, second.ID, actor, on); e != nil {
		t.Fatal("enable with unused SKU failed", e)
	}
	onState := state(second.ID)
	if !onState.Variant.ManageInventory || onState.InventoryLinks != 1 {
		t.Fatal("default inventory not created")
	}
	var oldActive int64
	db.Table("inventory_items").Where("id=? AND deleted_at IS NULL", second.InventoryItemID).Count(&oldActive)
	if oldActive != 1 {
		t.Fatal("old inventory record deleted")
	}
	plain := createInputFixture()
	plain.OperationID = uuid.NewString()
	plain.Handle = "plain-edit-" + uuid.NewString()
	plain.SalesChannelIDs = []string{dirs.Channels[0].ID}
	plain.ShippingProfileID = dirs.ShippingProfiles[0].ID
	plain.Options = []ProductCreateOption{}
	plainProduct, e := h.CreateProduct(ctx, actor, plain)
	if e != nil {
		t.Fatal(e)
	}
	plainVariants := []string{}
	for i := 0; i < 2; i++ {
		c, e := h.ReadVariantCreate(ctx, actor, plainProduct.ID)
		if e != nil {
			t.Fatal(e)
		}
		out, e := h.CreateVariant(ctx, actor, plainProduct.ID, VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: c.Revision, Title: "Plain SKU", OptionValueIDs: []string{}})
		if e != nil {
			t.Fatal("source permits multiple zero-dimensional SKUs", e)
		}
		plainVariants = append(plainVariants, out.ID)
	}
	plainContext, e := h.ReadVariantEdit(ctx, plainProduct.ID, plainVariants[0], actor)
	if e != nil {
		t.Fatal(e)
	}
	plainEdit := cmd(plainContext)
	plainEdit.Title = "Edited plain SKU"
	if _, e := h.EditVariant(ctx, plainProduct.ID, plainVariants[0], actor, plainEdit); e != nil {
		t.Fatal("plain SKU edit collided with another plain SKU", e)
	}
	gate(false)
	if r, e := h.EditVariant(ctx, p, second.ID, actor, on); e != nil || !r.Replayed {
		t.Fatal("closed-gate replay failed")
	}
	gate(true)
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID)
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.EditVariant(ctx, p, second.ID, actor, on); e == nil {
		t.Fatal("revoked actor recovered receipt")
	}
}
