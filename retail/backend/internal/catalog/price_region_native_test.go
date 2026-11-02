package catalog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func verifyRegionPriceCommands(t *testing.T, db *gorm.DB, actor identity.User) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("regional fixture", e)
		}
	}
	exec("INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('zar','R','R',2,0,'0','Artificial rand',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('regional-price-region','Regional price fixture','zar',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('regional-price-other','Other fixture','zar',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_products(id,title,handle,status,is_giftcard,discountable,created_at,updated_at) VALUES('regional-price-product','Regional fixture','regional-price-product','draft',FALSE,TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO catalog_variants(id,product_id,title,allow_backorder,manage_inventory,created_at,updated_at) VALUES('regional-price-v','regional-price-product','Regional fixture',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('regional-price-v2','regional-price-product','Shared fixture',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	h := Handler{DB: db}
	read := func() PriceCreateContext {
		t.Helper()
		d, e := h.ReadPriceCreate(ctx, actor, "regional-price-product", "regional-price-v")
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	current := read()
	in := PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: current.Revision, CurrencyCode: "zar", Amount: "12.123456789012345678", RegionID: "regional-price-region"}
	save := func(in PriceCreateInput) (PriceCreateResult, error) {
		return h.CreateBasePrice(ctx, actor, "regional-price-product", "regional-price-v", in)
	}
	if _, e := save(in); !errors.Is(e, ErrSchema) {
		t.Fatal("regional gate", e)
	}
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	// Base creation remains unchanged, and the region may override it.
	base := in
	base.RegionID = ""
	base.Amount = "9.000000000000000000"
	base.OperationID = uuid.NewString()
	if _, e := save(base); e != nil {
		t.Fatal("base compatibility", e)
	}
	current = read()
	in.ExpectedRevision = current.Revision
	exec("CREATE TRIGGER regional_receipt_failure BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial region receipt failure'")
	_, failure := save(in)
	exec("DROP TRIGGER regional_receipt_failure")
	if failure == nil || read().Revision != current.Revision {
		t.Fatal("region receipt rollback", failure)
	}
	var n int64
	db.Table("pricing_prices").Where("price_set_id=?", *current.PriceSetID).Count(&n)
	if n != 1 {
		t.Fatal("region rollback left price", n)
	}
	results := make([]PriceCreateResult, 6)
	errs := make([]error, 6)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = save(in) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.ID != results[0].ID {
			t.Fatal("region concurrent", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("regional duplicate submit", fresh)
	}
	current = read()
	if len(current.ExistingRegionPrices) != 1 {
		t.Fatal("regional context")
	}
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = current.Revision
	if _, e := save(duplicate); !errors.Is(e, ErrEditConflict) {
		t.Fatal("duplicate regional price", e)
	}
	invalid := duplicate
	invalid.RegionID = "unknown-region"
	if _, e := save(invalid); !errors.Is(e, ErrPriceInput) {
		t.Fatal("unknown region", e)
	}
	invalid.RegionID = "regional-price-region"
	invalid.CurrencyCode = "eur"
	if _, e := save(invalid); !errors.Is(e, ErrPriceInput) {
		t.Fatal("region currency mismatch", e)
	}
	exec("INSERT INTO catalog_variant_price_sets(id,variant_id,price_set_id,created_at,updated_at) VALUES('regional-shared-link','regional-price-v2',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", *current.PriceSetID)
	for _, region := range []string{"regional-price-region", "regional-price-other"} {
		e := db.Transaction(func(tx *gorm.DB) error {
			quotes, e := pricing.Read(tx, []string{"regional-price-v", "regional-price-v2"}, pricing.Context{Currency: "zar", Region: region, Quantity: "1"}, time.Now().UTC())
			if e != nil {
				return e
			}
			want := "9"
			if region == "regional-price-region" {
				want = "12.123456789012345678"
			}
			for _, id := range []string{"regional-price-v", "regional-price-v2"} {
				if quotes[id] == nil || quotes[id].Amount != want {
					t.Fatalf("region quote %s %s: %+v", region, id, quotes[id])
				}
			}
			return nil
		})
		if e != nil {
			t.Fatal("real regional quote", e)
		}
	}
	exec("UPDATE commerce_regions SET deleted_at=UTC_TIMESTAMP(6) WHERE id='regional-price-region'")
	if _, e := save(in); !errors.Is(e, ErrPriceInput) {
		t.Fatal("deleted region receipt replay", e)
	}
	exec("UPDATE commerce_regions SET deleted_at=NULL WHERE id='regional-price-region'")
	verifyPriceRemoval(t, db, actor, results[0].ID)
	t.Log(`REGIONAL_PRICE_NATIVE {"default_closed":true,"base_preserved":true,"late_receipt_rollback":true,"concurrent_attempts":6,"single_price":true,"duplicate_rejected":true,"unknown_region_rejected":true,"currency_mismatch_rejected":true,"shared_sku_regional_quote":true,"other_region_base_fallback":true,"deleted_region_replay_denied":true}`)
}
