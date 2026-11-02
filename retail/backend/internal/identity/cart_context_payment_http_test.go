package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func checkContextPaymentHTTP(t *testing.T, ctx context.Context, db *gorm.DB, sessions *sessionstore.Store, makeCart func() (sessionstore.CartReference, cart.InitialPaymentCommand), options cart.InitialPaymentOptions) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE cart_context_runtime SET enabled=TRUE WHERE id=1")
	exec("UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1")
	defer exec("UPDATE cart_context_runtime SET enabled=FALSE WHERE id=1")
	defer exec("UPDATE cart_payment_refresh_runtime SET enabled=FALSE WHERE id=1")
	var price struct{ Amount, Raw string }
	if db.Table("pricing_prices").Select("CAST(amount AS CHAR) AS amount,raw_amount AS raw").Where("id='add-single'").Take(&price).Error != nil {
		t.Fatal("HTTP price")
	}
	defer exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
	for _, scenario := range []string{"unchanged", "changed", "restore", "restore-retained", "uncertain", "paid"} {
		exec("UPDATE pricing_prices SET amount=?,raw_amount=? WHERE id='add-single'", price.Amount, price.Raw)
		ref, initial := makeCart()
		if _, err := cart.StartInitialPayment(ctx, db, "shop", ref, initial, options); err != nil {
			t.Fatal(err)
		}
		issued, err := sessions.IssueCart(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		defer sessions.RevokeCart(ctx, issued.Token)
		deletesCalled := 0
		deletes, err := payments.NewDeleteAdapters(map[string]payments.DeleteSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionDelete(func(context.Context, payments.SessionRequest) (payments.DeleteOutcome, error) {
			deletesCalled++
			if scenario == "uncertain" || scenario == "paid" {
				return payments.DeleteUncertain, nil
			}
			return payments.DeleteSucceeded, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		configured := options
		createsCalled := 0
		observedStatus := "pending"
		observations := 0
		configured.Adapters, err = payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": executionObserver{create: func(_ context.Context, request payments.SessionRequest) (payments.CreateOutcome, payments.CreatedSession, error) {
			createsCalled++
			if request.Amount != "158.75" || request.Currency != "eur" {
				t.Error("restore changed original amount")
			}
			return payments.CreateSucceeded, payments.CreatedSession{Status: "pending", Data: ownedMollieCreation(request, "tr_context_http_restore", "https://www.mollie.com/checkout/contextrestore")}, nil
		}, observe: func(_ context.Context, request payments.SessionRequest, id string) (payments.SessionObservation, error) {
			observations++
			actions := map[string]string{"pending": "pending", "paid": "successful", "canceled": "canceled"}
			local := map[string]string{"pending": "pending", "paid": "authorized", "canceled": "canceled"}
			raw, _ := json.Marshal(map[string]any{"id": id, "status": observedStatus, "amount": map[string]string{"value": request.Amount, "currency": strings.ToUpper(request.Currency)}, "metadata": map[string]string{"session_id": request.SessionID, "cart_id": inScope(request)}})
			return payments.SessionObservation{PaymentID: id, ProviderStatus: observedStatus, Action: actions[observedStatus], Amount: request.Amount, Currency: request.Currency, Session: payments.CreatedSession{Status: local[observedStatus], Data: raw}}, nil
		}}})
		if err != nil {
			t.Fatal(err)
		}
		router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: "http://127.0.0.1:19001"}, cart.HTTPOptions{Sessions: sessions, Origin: options.ReturnOrigin, StoreID: "shop", ManualProviderIDs: options.ManualProviders, InitialPayment: &configured, ContextPaymentDeletes: deletes})
		if err != nil {
			t.Fatal(err)
		}
		command := cart.ContextCommand{OperationID: uuid.NewString(), Version: "5", RegionID: "region", Locale: "zh-CN"}
		bodyBytes, _ := json.Marshal(command)
		body := string(bodyBytes)
		path := "/api/store/carts/" + ref.CartID + "/context-payment"
		call := func(target, payload, origin, csrf string, cookie bool, want int) cart.ContextPaymentProgress {
			t.Helper()
			r := httptest.NewRequest("POST", target, strings.NewReader(payload))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", command.OperationID)
			r.Header.Set("Origin", origin)
			r.Header.Set("X-CSRF-Token", csrf)
			if cookie {
				r.AddCookie(&http.Cookie{Name: "retail_cart_possession", Value: issued.Token})
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != want {
				t.Fatalf("%s HTTP got %d want %d: %s", scenario, w.Code, want, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" || len(w.Result().Cookies()) != 0 {
				t.Fatal("HTTP private contract")
			}
			for _, private := range []string{"payses_", "request_envelope", "manifest", "AttemptID", "Digest", "tr_context"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("HTTP protected data leaked")
				}
			}
			var result cart.ContextPaymentProgress
			if want == 200 && json.Unmarshal(w.Body.Bytes(), &result) != nil {
				t.Fatal("HTTP progress")
			}
			return result
		}
		call(path, body, options.ReturnOrigin, issued.CSRF, false, 401)
		call(path, body, "https://foreign.example", issued.CSRF, true, 403)
		call(path, body, options.ReturnOrigin, "", true, 403)
		call(path+"?fields=*", body, options.ReturnOrigin, issued.CSRF, true, 400)
		call(path, strings.Replace(body, `"version":"5"`, `"version":5`, 1), options.ReturnOrigin, issued.CSRF, true, 400)
		call(path, strings.Replace(body, `"locale":"zh-CN"`, `"session_id":"injected","locale":"zh-CN"`, 1), options.ReturnOrigin, issued.CSRF, true, 400)
		if deletesCalled != 0 || createsCalled != 0 {
			t.Fatal("rejected HTTP called provider")
		}
		if scenario != "unchanged" {
			exec(`UPDATE pricing_prices SET amount=121,raw_amount='{"value":"121","precision":100}' WHERE id='add-single'`)
		}
		if scenario == "restore" || scenario == "restore-retained" {
			staged, err := cart.StageContextPayment(ctx, db, "shop", ref, command, configured)
			if err != nil || len(staged.Effects) != 1 {
				t.Fatal("restore HTTP staging", err)
			}
			if scenario == "restore" {
				if _, err = cart.ExecuteContextPaymentSession(ctx, db, "shop", ref, command, staged.Effects[0].ResourceID, configured, deletes); err != nil {
					t.Fatal(err)
				}
			}
			path += "/restore"
		}
		if scenario == "uncertain" || scenario == "paid" {
			if scenario == "paid" {
				observedStatus = "paid"
			}
			for retry := 0; retry < 2; retry++ {
				result := call(path, body, options.ReturnOrigin, issued.CSRF, true, 200)
				if result.State != "requires_recovery" || result.Receipt != nil {
					t.Fatal("unknown or paid delete released freeze", result)
				}
			}
			if deletesCalled != 1 || observations != 2 {
				t.Fatal("unknown delete repeated mutation", deletesCalled, observations)
			}
			if scenario == "paid" {
				call(path+"/restore", body, options.ReturnOrigin, issued.CSRF, true, 409)
				var owner struct {
					Version                                      uint64
					CheckoutAttemptID, PaymentRefreshOperationID *string
				}
				if db.Table("cart_ownership").Where("cart_id=?", ref.CartID).Take(&owner).Error != nil || owner.Version != 5 || owner.CheckoutAttemptID == nil || owner.PaymentRefreshOperationID == nil {
					t.Fatal("paid observation unfroze or saved context")
				}
				continue
			}
			observedStatus = "canceled"
		}
		for attempt := 0; attempt < 2; attempt++ {
			result := call(path, body, options.ReturnOrigin, issued.CSRF, true, 200)
			if result.Completion == nil || result.Completion.Cart.ID != ref.CartID || result.Completion.Cart.Version != "6" {
				t.Fatal("missing current HTTP completion", result)
			}
			if scenario == "changed" || scenario == "uncertain" {
				if result.Completion.Payment != nil {
					t.Fatal("retired payment still returned")
				}
			} else {
				wantOperation, wantVersion := initial.OperationID, "4"
				if scenario == "restore" {
					wantOperation, wantVersion = command.OperationID, "5"
				}
				binding := result.Completion.Payment
				if binding == nil || binding.OperationID != wantOperation || binding.Version != wantVersion {
					t.Fatal("wrong current HTTP payment binding", binding)
				}
			}
			if scenario == "restore" || scenario == "restore-retained" {
				if result.State != "restored" || result.Receipt != nil {
					t.Fatal("restore falsely saved target", result)
				}
			} else if result.State != "saved" || result.Receipt == nil || result.Receipt.Version != "6" || result.Receipt.Replayed != (attempt == 1) {
				t.Fatal("saved HTTP receipt", result)
			}
		}
		wantDeletes, wantCreates := 1, 0
		if scenario == "unchanged" || scenario == "restore-retained" {
			wantDeletes = 0
		}
		if scenario == "restore" {
			wantCreates = 1
		}
		if deletesCalled != wantDeletes || createsCalled != wantCreates {
			t.Fatal("HTTP retry repeated mutation", deletesCalled, createsCalled)
		}
		if scenario == "restore" {
			var receipts int64
			db.Table("cart_context_operations").Where("cart_id=? AND operation_id=?", ref.CartID, command.OperationID).Count(&receipts)
			if receipts != 0 {
				t.Fatal("restoration wrote target save receipt")
			}
		}
	}
	t.Log(`CART_CONTEXT_PAYMENT_HTTP {"real_gin_mysql_redis":true,"unchanged_and_changed_save":true,"restore_deleted_session":true,"lost_response_exact_retry":true,"no_repeated_mutation":true,"unknown_delete_get_recovery":true,"paid_observation_keeps_frozen":true,"possession_csrf_origin_strict_input":true,"private_response_no_protected_material":true,"no_false_target_receipt":true,"artificial_provider_only":true,"current_binding_retained_replaced_or_retired":true,"astro_connected":false}`)
}
