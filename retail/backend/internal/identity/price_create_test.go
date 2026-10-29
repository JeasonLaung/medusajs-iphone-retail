package identity_test

import (
	"encoding/json"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func checkPriceCreate(t *testing.T, db *gorm.DB, actor identity.User, call func(string, string, string, int) []byte) {
	t.Helper()
	ctx := t.Context()
	h := catalog.Handler{DB: db}
	exec := func(q string, args ...any) {
		t.Helper()
		if e := db.Exec(q, args...).Error; e != nil {
			t.Fatal("price create fixture", e)
		}
	}
	exec(`INSERT INTO catalog_variants(id,product_id,title,sku,allow_backorder,manage_inventory,created_at,updated_at) VALUES('price-create-v','price-edit-product','Unpriced SKU','UNPRICED',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('price-create-v2','price-edit-product','Another unpriced SKU','UNPRICED2',FALSE,FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	const path = "/api/admin/products/price-edit-product/variants/price-create-v"
	read := func() catalog.PriceCreateContext {
		t.Helper()
		var d catalog.PriceCreateContext
		if json.Unmarshal(call("GET", path+"/price-creation", "", 200), &d) != nil {
			t.Fatal("price create context")
		}
		return d
	}
	old := read()
	if old.Enabled || old.PriceSetID != nil || len(old.Currencies) == 0 {
		t.Fatal("unpriced context or default gate")
	}
	in := catalog.PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: old.Revision, CurrencyCode: "eur", Amount: "99.123456789012345678"}
	body := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	call("POST", path+"/prices", body(in), 503)
	exec("UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE catalog_price_edit_runtime SET enabled=FALSE WHERE id=1")
	// No set/link/price or revision survives a failure at the final audit write.
	exec("CREATE TRIGGER price_create_receipt_fault BEFORE INSERT ON catalog_price_edits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial price creation fault'")
	call("POST", path+"/prices", body(in), 503)
	exec("DROP TRIGGER price_create_receipt_fault")
	if now := read(); now.PriceSetID != nil || now.Revision != old.Revision {
		t.Fatal("price create rollback left set/link/revision")
	}
	var wins, replays atomic.Int64
	var failed atomic.Bool
	var wg sync.WaitGroup
	ids := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := h.CreateBasePrice(ctx, actor, "price-edit-product", "price-create-v", in)
			if e != nil {
				failed.Store(true)
			} else {
				ids <- out.ID
				if out.Replayed {
					replays.Add(1)
				} else {
					wins.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	close(ids)
	if failed.Load() || wins.Load() != 1 || replays.Load() != 3 {
		t.Fatal("price create concurrent replay")
	}
	id := ""
	for got := range ids {
		if id != "" && id != got {
			t.Fatal("replay price id changed")
		}
		id = got
	}
	call("POST", path+"/prices", body(in), 200)
	now := read()
	if now.PriceSetID == nil || len(now.ExistingBaseCurrencies) != 1 || now.ExistingBaseCurrencies[0] != "eur" || now.SharedVariants != 1 {
		t.Fatal("created price context")
	}
	duplicate := in
	duplicate.OperationID = uuid.NewString()
	duplicate.ExpectedRevision = now.Revision
	call("POST", path+"/prices", body(duplicate), 409)
	altered := in
	altered.Amount = "1.000000000000000000"
	call("POST", path+"/prices", body(altered), 409)
	call("POST", strings.Replace(path, "price-create-v", "price-create-v2", 1)+"/prices", body(in), 409)
	checkQuote := func(variant, amount string) {
		t.Helper()
		var quote *pricing.Quote
		e := db.Transaction(func(tx *gorm.DB) error {
			rows, e := pricing.Read(tx, []string{variant}, pricing.Context{Currency: "eur", Region: "price-edit-region", Channel: "price-edit-channel", Quantity: "1"}, time.Now().UTC())
			quote = rows[variant]
			return e
		})
		if e != nil || quote == nil || quote.Amount != amount {
			t.Fatal("created price quote differs", e)
		}
	}
	checkQuote("price-create-v", in.Amount)
	// Currency comes from real configuration; malformed/missing currencies cannot become quotes.
	missing := duplicate
	missing.CurrencyCode = "zzz"
	call("POST", path+"/prices", body(missing), 400)
	// Add to an existing shared set without replacing its regional/base prices.
	exec(`INSERT INTO commerce_currencies(code,symbol,symbol_native,decimal_digits,rounding,raw_rounding,name,created_at,updated_at) VALUES('usd','USD','USD',2,0,'{"value":"0","precision":20}','US Dollar',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	sharedPath := "/api/admin/products/price-edit-product/variants/price-edit-v"
	var shared catalog.PriceCreateContext
	if json.Unmarshal(call("GET", sharedPath+"/price-creation", "", 200), &shared) != nil || shared.SharedVariants != 2 {
		t.Fatal("shared creation context")
	}
	add := catalog.PriceCreateInput{OperationID: uuid.NewString(), ExpectedRevision: shared.Revision, CurrencyCode: "usd", Amount: "0.000000000000000000"}
	var sharedResult catalog.PriceCreateResult
	if json.Unmarshal(call("POST", sharedPath+"/prices", body(add), 200), &sharedResult) != nil || sharedResult.ID == "" {
		t.Fatal("shared base price creation")
	}
	var count int64
	if db.Table("pricing_prices").Where("price_set_id=? AND currency_code='usd' AND deleted_at IS NULL", *shared.PriceSetID).Count(&count).Error != nil || count != 1 {
		t.Fatal("shared price copied or missing")
	}
	exec(`INSERT INTO commerce_regions(id,name,currency_code,automatic_taxes,created_at,updated_at) VALUES('price-create-us-region','US','usd',TRUE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	var sharedQuotes map[string]*pricing.Quote
	if e := db.Transaction(func(tx *gorm.DB) error {
		var e error
		sharedQuotes, e = pricing.Read(tx, []string{"price-edit-v", "price-edit-shared"}, pricing.Context{Currency: "usd", Region: "price-create-us-region", Channel: "price-edit-channel", Quantity: "1"}, time.Now().UTC())
		return e
	}); e != nil {
		t.Fatal("new shared currency quote", e)
	}
	for _, v := range []string{"price-edit-v", "price-edit-shared"} {
		if sharedQuotes[v] == nil || sharedQuotes[v].Amount != "0" {
			t.Fatal("new shared currency zero quote differs")
		}
	}
	// Original same-currency price remains; creating a different currency must not delete it.
	checkQuote("price-edit-v", "101.000000000000000001")
	exec("UPDATE catalog_variant_price_sets SET deleted_at=UTC_TIMESTAMP(6) WHERE variant_id='price-create-v'")
	call("POST", path+"/prices", body(in), 404)
	exec("UPDATE catalog_variant_price_sets SET deleted_at=NULL WHERE variant_id='price-create-v'")
	var role string
	db.Table("authorization_memberships").Select("role_id").Where("user_id=?", actor.ID).Limit(1).Scan(&role)
	exec("DELETE FROM authorization_role_actions WHERE role_id=? AND action_code='price.write'", role)
	call("POST", path+"/prices", body(in), 403)
	exec("INSERT INTO authorization_role_actions(role_id,action_code) VALUES(?,'price.write')", role)
	exec("UPDATE authorization_principals SET kind='warehouse' WHERE user_id=?", actor.ID)
	call("POST", path+"/prices", body(in), 403)
	exec("UPDATE authorization_principals SET kind='employee' WHERE user_id=?", actor.ID)
	call("POST", path+"/prices", `{"currency_code":"eur","amount":1}`, 400)
	t.Log(`PRICE_CREATE_REHEARSAL {"default_disabled":true,"atomic_new_set_link_price":true,"late_receipt_failure_atomic":true,"concurrent_attempts":4,"single_price":true,"same_receipt_id":true,"duplicate_base_currency_denied":true,"missing_currency_denied":true,"exact_quote":true,"shared_set_preserved":true,"zero_price":true,"unlinked_replay_denied":true,"revoked_and_warehouse_replay_denied":true}`)
}
