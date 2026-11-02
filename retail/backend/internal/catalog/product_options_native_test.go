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

func TestNativeProductOptionsTransactions(t *testing.T) {
	if os.Getenv("PRODUCT_OPTIONS_NATIVE_REHEARSAL") != "1" {
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
	directory, e := h.ReadProductCreate(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	product := func() string {
		in := createInputFixture()
		in.OperationID = uuid.NewString()
		in.Handle = "options-native-" + uuid.NewString()
		in.SalesChannelIDs = []string{directory.Channels[0].ID}
		in.ShippingProfileID = directory.ShippingProfiles[0].ID
		out, e := h.CreateProduct(ctx, actor, in)
		if e != nil {
			t.Fatal(e)
		}
		return out.ID
	}
	p, foreign := product(), product()
	state := func(id string) ProductOptionsContext {
		out, e := h.ReadProductOptions(ctx, id, actor)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	selected := func(c ProductOptionsContext) []OptionSelection {
		out := []OptionSelection{}
		for _, o := range c.Options {
			if o.Selected {
				vs := []OptionSelectionValue{}
				for _, v := range o.Values {
					if v.Selected {
						vid := v.ID
						vs = append(vs, OptionSelectionValue{&vid, v.Value})
					}
				}
				id := o.ID
				out = append(out, OptionSelection{&id, o.Title, vs})
			}
		}
		return out
	}
	before := state(p)
	initial := selected(before)
	if len(initial) != 1 {
		t.Fatal("fixture options missing")
	}
	oid := *initial[0].ID
	for _, o := range before.Options {
		if o.ID == oid && !o.IsExclusive {
			t.Fatal("created option polluted shared directory")
		}
	}
	shared, sv := "opt_"+uuid.NewString(), "optval_"+uuid.NewString()
	if db.Exec("INSERT INTO catalog_options(id,title,is_exclusive,created_at,updated_at) VALUES(?, ?,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", shared, "Native shared "+uuid.NewString()).Error != nil {
		t.Fatal("shared fixture failed")
	}
	if db.Exec("INSERT INTO catalog_option_values(id,option_id,value,`rank`,created_at,updated_at) VALUES(?,?,'Shared value',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", sv, shared).Error != nil {
		t.Fatal("shared value failed")
	}
	before = state(p)
	foreignBefore := state(foreign)
	var sharedTitle string
	for _, o := range before.Options {
		if o.ID == shared {
			sharedTitle = o.Title
		}
	}
	in := ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: before.Revision, Options: selected(before)}
	in.Options[0].Values = append(in.Options[0].Values, OptionSelectionValue{Value: "512 GB"})
	in.Options = append(in.Options, OptionSelection{ID: &shared, Title: sharedTitle, Values: []OptionSelectionValue{{ID: &sv, Value: "Shared value"}}}, OptionSelection{Title: "Native warranty", Values: []OptionSelectionValue{{Value: "12 months"}, {Value: "24 months"}}})
	gate := func(on bool) {
		if db.Exec("UPDATE catalog_command_capabilities SET enabled=? WHERE command='product.options'", on).Error != nil {
			t.Fatal("gate failed")
		}
	}
	gate(false)
	if _, e := h.EditProductOptions(ctx, p, actor, in); !errors.Is(e, ErrSchema) {
		t.Fatal("disabled write accepted")
	}
	gate(true)
	var wg sync.WaitGroup
	results := make([]ImageResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = h.EditProductOptions(ctx, p, actor, in) }(i)
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
		t.Fatal("duplicate option write")
	}
	after := state(p)
	if len(selected(after)) != 3 || len(selected(after)[0].Values)+len(selected(after)[1].Values)+len(selected(after)[2].Values) != 6 {
		t.Fatal("option membership mismatch")
	}
	if state(foreign).Revision != foreignBefore.Revision {
		t.Fatal("other product changed")
	}
	if _, e := h.EditProductOptions(ctx, foreign, actor, in); !errors.Is(e, ErrEditConflict) {
		t.Fatal("cross-product replay accepted")
	}
	stale := in
	stale.OperationID = uuid.NewString()
	if _, e := h.EditProductOptions(ctx, p, actor, stale); !errors.Is(e, ErrEditConflict) {
		t.Fatal("stale save accepted")
	}
	// Shared definitions cannot be extended or renamed through product membership editing.
	bad := ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, Options: selected(after)}
	for i, o := range bad.Options {
		if *o.ID == shared {
			bad.Options[i].Values = append(o.Values, OptionSelectionValue{Value: "Global mutation"})
		}
	}
	if _, e := h.EditProductOptions(ctx, p, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("global value creation accepted")
	}
	bad.Options = selected(after)
	for i, o := range bad.Options {
		if *o.ID == shared {
			bad.Options[i].Title = "Renamed"
		}
	}
	if _, e := h.EditProductOptions(ctx, p, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("shared rename accepted")
	}
	// A foreign exclusive option is absent from the directory and cannot be attached.
	foreignOption := selected(state(foreign))[0]
	bad.Options = []OptionSelection{foreignOption}
	if _, e := h.EditProductOptions(ctx, p, actor, bad); !errors.Is(e, errEditInput) {
		t.Fatal("foreign exclusive option accepted")
	}
	// An active SKU locks only its used value, preserving the established source rule.
	variant := "variant_" + uuid.NewString()
	used := *initial[0].Values[0].ID
	if db.Exec("INSERT INTO catalog_variants(id,product_id,title,manage_inventory,allow_backorder,created_at,updated_at) VALUES(?,?,'Existing SKU',TRUE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", variant, p).Error != nil || db.Exec("INSERT INTO catalog_variant_options(variant_id,option_value_id) VALUES(?,?)", variant, used).Error != nil {
		t.Fatal("fixture SKU failed")
	}
	after = state(p)
	blocked := ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, Options: []OptionSelection{}}
	if _, e := h.EditProductOptions(ctx, p, actor, blocked); !errors.Is(e, ErrOptionInUse) {
		t.Fatal("used option removed", e)
	}
	blocked.Options = selected(after)
	for i, o := range blocked.Options {
		if *o.ID == oid {
			vs := []OptionSelectionValue{}
			for _, v := range o.Values {
				if *v.ID != used {
					vs = append(vs, v)
				}
			}
			blocked.Options[i].Values = vs
		}
	}
	if _, e := h.EditProductOptions(ctx, p, actor, blocked); !errors.Is(e, ErrOptionInUse) {
		t.Fatal("used value removed", e)
	}
	// Fail the final receipt after definitions and relation writes; all must roll back.
	rollback := ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, Options: selected(after)}
	rollback.Options = append(rollback.Options, OptionSelection{Title: "Must roll back", Values: []OptionSelectionValue{{Value: "Never persists"}}})
	counts := map[string]int64{}
	for _, table := range []string{"catalog_options", "catalog_option_values", "catalog_product_options", "catalog_product_option_values", "catalog_option_edits"} {
		var n int64
		db.Table(table).Count(&n)
		counts[table] = n
	}
	db.Callback().Create().Before("gorm:create").Register("options_receipt_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "catalog_option_edits" {
			tx.AddError(errors.New("fixture final receipt failure"))
		}
	})
	_, e = h.EditProductOptions(ctx, p, actor, rollback)
	db.Callback().Create().Remove("options_receipt_fail")
	if e == nil {
		t.Fatal("late failure ignored")
	}
	for table, want := range counts {
		var n int64
		db.Table(table).Count(&n)
		if n != want {
			t.Fatal("row escaped rollback", table)
		}
	}
	if state(p).Revision != after.Revision {
		t.Fatal("revision escaped rollback")
	}
	// Keep the used value; remove unused values and groups without rewriting definitions.
	keep := initial[0]
	keep.Values = keep.Values[:1]
	removal := ProductOptionsInput{OperationID: uuid.NewString(), ExpectedRevision: after.Revision, Options: []OptionSelection{keep}}
	if _, e := h.EditProductOptions(ctx, p, actor, removal); e != nil {
		t.Fatal("unused membership removal failed", e)
	}
	remaining := state(p)
	if len(selected(remaining)) != 1 || len(selected(remaining)[0].Values) != 1 {
		t.Fatal("membership not detached")
	}
	var globalLive int64
	db.Table("catalog_options").Where("id=? AND deleted_at IS NULL", shared).Count(&globalLive)
	if globalLive != 1 {
		t.Fatal("shared definition deleted")
	}
	var warrantyLive int64
	db.Table("catalog_options").Where("title='Native warranty' AND deleted_at IS NULL AND is_exclusive=TRUE").Count(&warrantyLive)
	if warrantyLive != 0 {
		t.Fatal("orphan exclusive option remains active")
	}
	var skuLinks int64
	db.Table("catalog_variant_options").Where("variant_id=? AND option_value_id=?", variant, used).Count(&skuLinks)
	if skuLinks != 1 {
		t.Fatal("existing SKU changed")
	}
	gate(false)
	if r, e := h.EditProductOptions(ctx, p, actor, removal); e != nil || !r.Replayed {
		t.Fatal("closed-gate replay failed")
	}
	gate(true)
	if db.Exec("DELETE a FROM authorization_role_actions a JOIN authorization_memberships m ON m.role_id=a.role_id WHERE m.user_id=? AND a.action_code='product.write'", actor.ID).Error != nil {
		t.Fatal("revoke failed")
	}
	defer db.Exec("INSERT IGNORE INTO authorization_role_actions(role_id,action_code) SELECT role_id,'product.write' FROM authorization_memberships WHERE user_id=?", actor.ID)
	if _, e := h.EditProductOptions(ctx, p, actor, removal); e == nil {
		t.Fatal("revoked actor recovered receipt")
	}
}
