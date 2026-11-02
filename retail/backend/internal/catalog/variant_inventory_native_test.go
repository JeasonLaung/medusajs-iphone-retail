package catalog

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestNativeInventoryLinkTransactions(t *testing.T) {
	if os.Getenv("INVENTORY_LINKS_NATIVE_REHEARSAL") != "1" {
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
	h := Handler{DB: db}
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("owned admin required")
	}
	exec := func(q string, args ...any) {
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	dirs, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	in := createInputFixture()
	in.OperationID = uuid.NewString()
	in.Handle = "inventory-native-" + uuid.NewString()
	in.Options = []ProductCreateOption{}
	in.SalesChannelIDs = []string{dirs.Channels[0].ID}
	in.ShippingProfileID = dirs.ShippingProfiles[0].ID
	product, e := h.CreateProduct(ctx, actor, in)
	if e != nil {
		t.Fatal(e)
	}
	p := product.ID
	variants := []VariantCreateResult{}
	for i := 0; i < 3; i++ {
		c, e := h.ReadVariantCreate(ctx, actor, p)
		if e != nil {
			t.Fatal(e)
		}
		v, e := h.CreateVariant(ctx, actor, p, VariantCreateInput{OperationID: uuid.NewString(), ExpectedRevision: c.Revision, Title: "Owned inventory SKU", OptionValueIDs: []string{}})
		if e != nil {
			t.Fatal(e)
		}
		variants = append(variants, v)
	}
	first, second, tracked := variants[0], variants[1], variants[2]
	location := "sloc_" + uuid.NewString()
	exec("INSERT INTO inventory_locations(id,name,created_at,updated_at) VALUES(?,'Owned inventory links',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", location)
	exec("INSERT INTO inventory_levels(id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,raw_stocked_quantity,raw_reserved_quantity,raw_incoming_quantity,created_at,updated_at) VALUES(?,?,?,7.25,1.25,0,'{\"value\":\"7.25\",\"precision\":38}','{\"value\":\"1.25\",\"precision\":38}','{\"value\":\"0\",\"precision\":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "ilvl_"+uuid.NewString(), first.InventoryItemID, location)
	exec("INSERT INTO inventory_reservations(id,inventory_item_id,location_id,quantity,raw_quantity,created_at,updated_at) VALUES(?,?,?,1.25,'{\"value\":\"1.25\",\"precision\":38}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "res_"+uuid.NewString(), first.InventoryItemID, location)
	exec("INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "dvt_"+uuid.NewString(), tracked.ID, tracked.InventoryItemID)
	source := "source_" + uuid.NewString()
	exec("INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES(?,'Owned source','self',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", source, location)
	exec("INSERT INTO devices(id,serial_number,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) VALUES(?,?,?, ?,?,100,'eur','','','available','sale','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "dev_"+uuid.NewString(), "OWNED-"+uuid.NewString(), tracked.ID, source, location)
	priceCtx, e := h.ReadPriceCreate(ctx, actor, p, first.ID)
	if e != nil {
		t.Fatal(e)
	}
	price, e := h.CreateBasePrice(ctx, actor, p, first.ID, PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: priceCtx.Revision, CurrencyCode: "eur", Amount: "129.123456789012345678"})
	if e != nil {
		t.Fatal(e)
	}
	facts := func() string {
		data := map[string]any{}
		for table, where := range map[string]string{"inventory_items": "id IN ?", "inventory_levels": "inventory_item_id IN ?", "inventory_reservations": "inventory_item_id IN ?", "devices": "variant_id IN ?", "device_variant_tracking": "variant_id IN ?", "pricing_prices": "id IN ?"} {
			ids := []string{first.InventoryItemID, second.InventoryItemID, tracked.InventoryItemID}
			if strings.HasPrefix(table, "device") {
				ids = []string{first.ID, second.ID, tracked.ID}
			}
			if table == "pricing_prices" {
				ids = []string{price.ID}
			}
			var rows []map[string]any
			if e := db.Table(table).Where(where, ids).Order("id").Find(&rows).Error; e != nil {
				t.Fatal(e)
			}
			data[table] = rows
		}
		return imageDigest(data)
	}
	preserved := facts()
	state := func(v string) VariantInventoryContext {
		out, e := h.ReadInventoryLinks(ctx, p, v, actor)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	cmd := func(c VariantInventoryContext, items ...InventoryRequirement) VariantInventoryInput {
		if items == nil {
			items = []InventoryRequirement{}
		}
		return VariantInventoryInput{uuid.NewString(), c.Revision, items}
	}
	gate := func(on bool) {
		exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='variant.inventory'", on)
	}
	before := state(first.ID)
	beforeOther := state(second.ID)
	body := cmd(before, InventoryRequirement{first.InventoryItemID, "0.123456789012345678"}, InventoryRequirement{second.InventoryItemID, "2.5"})
	gate(false)
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, body); !errors.Is(e, ErrSchema) {
		t.Fatal("disabled save accepted", e)
	}
	gate(true)
	results := make([]ImageResult, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = h.EditInventoryLinks(ctx, p, first.ID, actor, body)
		}(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.Revision != results[0].Revision {
			t.Fatal("concurrent save failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("duplicate durable write")
	}
	after := state(first.ID)
	if len(after.Items) != 2 || after.Items[0].RequiredQuantity == "" || facts() != preserved || state(second.ID).Revision != beforeOther.Revision {
		t.Fatal("relationship save changed underlying facts or another SKU")
	}
	var exact string
	db.Table("catalog_variant_inventory_items").Select("CAST(required_quantity AS CHAR)").Where("variant_id=? AND inventory_item_id=?", first.ID, first.InventoryItemID).Scan(&exact)
	if exact != "0.123456789012345678" {
		t.Fatal("decimal rounded")
	}
	if _, e := h.EditInventoryLinks(ctx, p, second.ID, actor, body); !errors.Is(e, ErrEditConflict) {
		t.Fatal("cross-SKU receipt accepted", e)
	}
	stale := body
	stale.OperationID = uuid.NewString()
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale save accepted")
	}
	if _, e := h.ReadInventoryLinks(ctx, "prod_missing", first.ID, actor); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("foreign SKU readable")
	}
	// Actual search is literal, paginated and marks foreign device identities.
	choices, e := h.InventoryLinkChoices(ctx, p, first.ID, tracked.SKU, 0, actor)
	if e != nil || len(choices.Items) != 1 || !choices.Items[0].DeviceBound {
		t.Fatal("device directory marker missing", e)
	}
	choices, e = h.InventoryLinkChoices(ctx, p, first.ID, "%", 0, actor)
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range choices.Items {
		combined := i.ID
		if i.Title != nil {
			combined += *i.Title
		}
		if i.SKU != nil {
			combined += *i.SKU
		}
		if !strings.Contains(combined, "%") {
			t.Fatal("search interpreted wildcard")
		}
	}
	for i := 0; i < 52; i++ {
		exec("INSERT INTO inventory_items(id,sku,title,requires_shipping,created_at,updated_at) VALUES(?,?,?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", "iitem_"+uuid.NewString(), "OWNED-"+uuid.NewString(), "paged-"+p)
	}
	page, e := h.InventoryLinkChoices(ctx, p, first.ID, "paged-"+p, 0, actor)
	if e != nil || len(page.Items) != 50 || !page.HasMore {
		t.Fatal("first page wrong", e)
	}
	last, e := h.InventoryLinkChoices(ctx, p, first.ID, "paged-"+p, 50, actor)
	if e != nil || len(last.Items) != 2 || last.HasMore || page.Items[49].ID >= last.Items[0].ID {
		t.Fatal("last page wrong", e)
	}
	// A retired parent does not release an active device inventory identity.
	exec("UPDATE catalog_variants SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", tracked.ID)
	retiredChoices, retiredError := h.InventoryLinkChoices(ctx, p, first.ID, tracked.SKU, 0, actor)
	_, shareError := h.EditInventoryLinks(ctx, p, first.ID, actor, cmd(after, InventoryRequirement{tracked.InventoryItemID, "1"}))
	exec("UPDATE catalog_variants SET deleted_at=NULL WHERE id=?", tracked.ID)
	if retiredError != nil || len(retiredChoices.Items) != 1 || !retiredChoices.Items[0].DeviceBound || !errors.Is(shareError, ErrTrackedInventory) {
		t.Fatal("retired SKU released active device identity")
	}
	// Both the tracked SKU and an attempt to share its item are rejected.
	for _, c := range []struct {
		v  string
		in VariantInventoryInput
	}{{tracked.ID, cmd(state(tracked.ID))}, {tracked.ID, cmd(state(tracked.ID), InventoryRequirement{tracked.InventoryItemID, "2"})}, {first.ID, cmd(after, InventoryRequirement{tracked.InventoryItemID, "1"})}} {
		if _, e := h.EditInventoryLinks(ctx, p, c.v, actor, c.in); !errors.Is(e, ErrTrackedInventory) {
			t.Fatal("device identity changed", e)
		}
	}
	edit, e := h.ReadVariantEdit(ctx, p, tracked.ID, actor)
	if e != nil {
		t.Fatal(e)
	}
	off := VariantEditInput{VariantFields: edit.Variant, OperationID: uuid.NewString(), ExpectedRevision: edit.Revision, OptionValueIDs: []string{}}
	off.ManageInventory = false
	if _, e := h.EditVariant(ctx, p, tracked.ID, actor, off); !errors.Is(e, ErrTrackedInventory) {
		t.Fatal("SKU edit detached tracked identity", e)
	}
	// A final audit insert failure rolls back removals, inserts and variant revision.
	rollback := cmd(after, InventoryRequirement{first.InventoryItemID, "4"})
	db.Callback().Create().Before("gorm:create").Register("inventory_link_receipt_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_inventory_edits" {
			tx.AddError(errors.New("owned late failure"))
		}
	})
	_, e = h.EditInventoryLinks(ctx, p, first.ID, actor, rollback)
	db.Callback().Create().Remove("inventory_link_receipt_fail")
	if e == nil || state(first.ID).Revision != after.Revision || facts() != preserved {
		t.Fatal("partial relationship write escaped rollback")
	}
	oldIDs := map[string]string{}
	for _, i := range after.Items {
		oldIDs[i.InventoryItemID] = i.LinkID
	}
	unlink := cmd(after)
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, unlink); e != nil {
		t.Fatal(e)
	}
	empty := state(first.ID)
	if len(empty.Items) != 0 {
		t.Fatal("empty relationship state rejected")
	}
	restore := cmd(empty, InventoryRequirement{first.InventoryItemID, "1"}, InventoryRequirement{second.InventoryItemID, "2.5"})
	db.Callback().Create().Before("gorm:create").Register("inventory_restore_receipt_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_inventory_edits" {
			tx.AddError(errors.New("owned restore failure"))
		}
	})
	_, restoreError := h.EditInventoryLinks(ctx, p, first.ID, actor, restore)
	db.Callback().Create().Remove("inventory_restore_receipt_fail")
	if restoreError == nil || state(first.ID).Revision != empty.Revision {
		t.Fatal("restored links escaped rollback")
	}
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, restore); e != nil {
		t.Fatal("reattach failed", e)
	}
	restored := state(first.ID)
	for _, i := range restored.Items {
		if i.LinkID != oldIDs[i.InventoryItemID] {
			t.Fatal("reattach replaced source relationship ID")
		}
	}
	if facts() != preserved {
		t.Fatal("physical facts changed after reattach")
	}
	// Soft-deleted inventory is retained in context, and requires explicit removal.
	exec("UPDATE inventory_items SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", second.InventoryItemID)
	deleted := state(first.ID)
	unavailable := false
	for _, i := range deleted.Items {
		if i.InventoryItemID == second.InventoryItemID {
			unavailable = !i.Available
		}
	}
	if !unavailable {
		t.Fatal("unavailable current item silently lost")
	}
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, cmd(deleted, InventoryRequirement{second.InventoryItemID, "1"})); !errors.Is(e, errEditInput) {
		t.Fatal("unavailable item accepted")
	}
	exec("UPDATE inventory_items SET deleted_at=NULL WHERE id=?", second.InventoryItemID)
	gate(false)
	if r, e := h.EditInventoryLinks(ctx, p, first.ID, actor, restore); e != nil || !r.Replayed {
		t.Fatal("closed-gate replay failed", e)
	}
	gate(true)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	_, e = h.ReadInventoryLinks(ctx, p, first.ID, actor)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	if !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("scoped actor accessed global item directory", e)
	}
	exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID)
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.EditInventoryLinks(ctx, p, first.ID, actor, restore); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("revoked actor recovered receipt", e)
	}
}
