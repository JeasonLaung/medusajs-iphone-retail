package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkPromotionCommands(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("promotion command fixture failed")
		}
	}
	created, err := cart.Create(ctx, db, cart.CreateCommand{OperationID: uuid.NewString(), ScopeHash: sha256.Sum256([]byte("owned promotion command cart")), StoreID: "shop", RegionID: "region", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	ref := created.Reference
	added, err := cart.AddItem(ctx, db, "shop", ref, cart.AddItemCommand{OperationID: uuid.NewString(), Version: "1", VariantID: "add-v", Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	address := cart.ContactAddress{FirstName: "Promo", LastName: "Buyer", Address1: "Private fixture street", City: "Copenhagen", PostalCode: "0012", Country: "dk"}
	providers := []string{"delivery-provider"}
	contact, err := cart.SaveContacts(ctx, db, "shop", ref, cart.ContactCommand{OperationID: uuid.NewString(), Version: added.Version, Email: "promotion@example.invalid", Shipping: address, Billing: address}, providers)
	if err != nil {
		t.Fatal(err)
	}
	shipping, err := cart.SaveShipping(ctx, db, "shop", ref, cart.ShippingCommand{OperationID: uuid.NewString(), Version: contact.Version, OptionID: "delivery-standard"}, providers)
	if err != nil {
		t.Fatal(err)
	}
	amounts := func() cart.AmountDetail {
		t.Helper()
		a, e := cart.ReadAmounts(ctx, db, "shop", ref)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	baseline := amounts().Totals["total"]
	fingerprint := func() string {
		t.Helper()
		var all []json.RawMessage
		for _, table := range []string{"cart_ownership", "cart_promotion_command_operations", "cart_line_items", "cart_line_tax_lines", "cart_shipping_methods", "cart_shipping_tax_lines", "cart_line_adjustments", "cart_shipping_adjustments", "cart_promotions", "inventory_reservations", "promotions"} {
			var rows []map[string]any
			if db.Table(table).Order("1").Find(&rows).Error != nil {
				t.Fatal("promotion fingerprint")
			}
			raw, _ := json.Marshal(rows)
			all = append(all, raw)
		}
		raw, _ := json.Marshal(all)
		return string(raw)
	}
	in := cart.PromotionCommand{OperationID: uuid.NewString(), Version: shipping.Version, Codes: []string{"COMMAND10"}}
	reject := func(input cart.PromotionCommand, want error) {
		t.Helper()
		before := fingerprint()
		r, e := cart.SavePromotions(ctx, db, "shop", ref, input, providers)
		if e != want || r != (cart.PromotionResult{}) || before != fingerprint() {
			t.Fatalf("promotion rejection/rollback: %v want %v", e, want)
		}
	}
	reject(in, cart.ErrSchema)
	exec("UPDATE cart_promotion_command_runtime SET enabled=TRUE WHERE id=1")
	exec("INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('command-promo','COMMAND10',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('command-auto','COMMAND_AUTO',TRUE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,created_at,updated_at) VALUES('command-promo-method','command-promo','percentage','items','across',10,'{"value":"10","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('command-auto-method','command-auto','fixed','items','across',5,'{"value":"5","precision":100}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	defer exec("UPDATE promotions SET status='inactive' WHERE id IN ('command-promo','command-auto')")
	unknown := in
	unknown.Codes = []string{"UNKNOWN_COMMAND"}
	reject(unknown, cart.ErrInput)
	var wg sync.WaitGroup
	results := make([]cart.PromotionResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = cart.SavePromotions(ctx, db, "shop", ref, in, providers)
		}(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.CartID != ref.CartID || r.Version != "5" {
			t.Fatal("concurrent promotion command", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("duplicate promotion mutation")
	}
	detail, err := cart.ReadAppliedPromotions(ctx, db, "shop", ref)
	if err != nil || detail.Version != "5" || len(detail.Promotions) != 2 {
		t.Fatal("saved promotion projection", err)
	}
	discounted := amounts().Totals["total"]
	a, ok := new(big.Rat).SetString(discounted)
	b, ok2 := new(big.Rat).SetString(baseline)
	if !ok || !ok2 || a.Cmp(b) >= 0 {
		t.Fatal("discount did not change exact total")
	}
	changed := in
	changed.Codes = []string{}
	reject(changed, cart.ErrConflict)
	changed.OperationID = uuid.NewString()
	reject(changed, cart.ErrConflict)
	next := cart.PromotionCommand{OperationID: uuid.NewString(), Version: "5", Codes: []string{}}
	exec("CREATE TRIGGER fail_promotion_receipt BEFORE INSERT ON cart_promotion_command_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial promotion receipt failure'")
	reject(next, cart.ErrStorage)
	exec("DROP TRIGGER fail_promotion_receipt")
	result, err := cart.SavePromotions(ctx, db, "shop", ref, next, providers)
	if err != nil || result.Version != "6" {
		t.Fatal("promotion removal retry", err)
	}
	detail, err = cart.ReadAppliedPromotions(ctx, db, "shop", ref)
	if err != nil || len(detail.Promotions) != 1 || !detail.Promotions[0].Automatic {
		t.Fatal("automatic promotion removed by client", err)
	}
	// Exercise the real private HTTP registration and receipt replay.
	const origin = "http://127.0.0.1:18080"
	issued, err := sessions.IssueCart(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := cart.NewHTTP(db, cart.HTTPOptions{Sessions: sessions, Origin: origin, StoreID: "shop", ManualProviderIDs: providers})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	handler.Register(router.Group("/api/store"))
	path := "/api/store/carts/" + ref.CartID + "/promotions"
	call := func(method, path, body, requestOrigin, csrf string, cookie bool, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", next.OperationID)
		if requestOrigin != "" {
			req.Header.Set("Origin", requestOrigin)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("promotion HTTP %s %s %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "private, no-store" || len(w.Result().Cookies()) != 0 {
			t.Fatal("promotion privacy")
		}
	}
	raw, _ := json.Marshal(next)
	call("POST", path, string(raw), origin, issued.CSRF, false, 401)
	call("POST", path, string(raw), origin, "", true, 403)
	call("POST", path, string(raw), "https://foreign.example", issued.CSRF, true, 403)
	call("POST", path, `{"version":"5","codes":[],"amount":"0"}`, origin, issued.CSRF, true, 400)
	call("POST", path, `{"version":"5","codes":[],"codes":[]}`, origin, issued.CSRF, true, 400)
	call("POST", path, `{"version":"5","codes":null}`, origin, issued.CSRF, true, 400)
	call("POST", path, string(raw), origin, issued.CSRF, true, 200)
	call("GET", path, "", "", "", true, 200)
	call("GET", path+"?email=promotion@example.invalid", "", "", "", true, 400)
	for i := 0; i < 31; i++ {
		if allowed, err := sessions.AllowCartRequest(ctx, "promotion-possession", issued.Token); err != nil || !allowed {
			t.Fatal("promotion request limit exhausted early", err)
		}
	}
	call("POST", path, string(raw), origin, issued.CSRF, true, 429)
	wrong := ref
	wrong.Generation = strings.Repeat("0", 64)
	if _, e := cart.ReadAppliedPromotions(ctx, db, "shop", wrong); e != cart.ErrDenied {
		t.Fatal("wrong generation read", e)
	}
	exec("INSERT INTO cart_payment_collections(cart_id,payment_collection_id,id,created_at,updated_at) VALUES(?,'delivery-payment','promotion-payment-link',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", ref.CartID)
	frozen := cart.PromotionCommand{OperationID: uuid.NewString(), Version: "6", Codes: []string{}}
	reject(frozen, cart.ErrConflict)
	exec("DELETE FROM cart_payment_collections WHERE id='promotion-payment-link'")
	exec("UPDATE promotions SET status='inactive' WHERE id IN ('command-promo','command-auto')")
	restored, err := cart.SavePromotions(ctx, db, "shop", ref, frozen, providers)
	if err != nil || restored.Version != "7" || amounts().Totals["total"] != baseline {
		t.Fatal("promotion removal did not restore exact total", err)
	}
	exec("UPDATE cart_promotion_command_runtime SET enabled=FALSE WHERE id=1")
	reject(in, cart.ErrSchema)
	t.Log(`CART_PROMOTION_COMMAND_REHEARSAL {"real_mysql_redis":true,"concurrent_attempts":4,"single_receipt":true,"manual_and_automatic":true,"remove_preserves_automatic":true,"exact_total_restored":true,"unknown_code_atomic":true,"late_receipt_failure_atomic":true,"payment_attached_denied":true,"private_http_csrf_origin":true,"duplicate_unknown_null_fields_denied":true,"current_generation_required":true,"disabled_replay_denied":true}`)
}
