package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func checkBuybackCommands(t *testing.T, db *gorm.DB, customers *customer.Store, router *gin.Engine, token, csrf, store string, in buyback.Submission) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("command fixture SQL")
		}
	}
	in.SubmissionKey = "00000000-0000-4000-8000-000000007777"
	in.Items = append(in.Items, in.Items[0])
	in.Items[0].Assessment.Frame = "visible"
	request, e := customers.CreateBuyback(ctx, token, store, in)
	if e != nil {
		t.Fatal("command request fixture")
	}
	id := request.ID
	var noticesBefore, noticesAfter int64
	db.Table("customer_account_notices").Count(&noticesBefore)
	identities := []buyback.CommandIdentity{{ItemID: request.Items[0].ID, SerialNumber: "SN-FIRST", IMEI: "000000000000001"}, {ItemID: request.Items[1].ID, SerialNumber: "SN-SECOND", IMEI: "000000000000002"}}
	command := buyback.CustomerCommand{Action: "submit_photos", Items: identities}
	update := func(in buyback.CustomerCommand) (buyback.RequestDetail, error) {
		return customers.UpdateBuyback(ctx, token, store, id, in)
	}
	if _, e := update(command); !errors.Is(e, buyback.ErrCommandPhotos) {
		t.Fatal("missing photos accepted")
	}
	duplicate := command
	duplicate.Items = append([]buyback.CommandIdentity{}, identities...)
	duplicate.Items[1].SerialNumber = identities[0].SerialNumber
	if _, e := update(duplicate); !errors.Is(e, buyback.ErrCommandIdentity) {
		t.Fatal("duplicate SN accepted")
	}
	duplicate.Items[1] = identities[1]
	duplicate.Items[1].IMEI = identities[0].IMEI
	if _, e := update(duplicate); !errors.Is(e, buyback.ErrCommandIdentity) {
		t.Fatal("duplicate IMEI accepted")
	}
	var firstPhoto string
	for _, item := range request.Items {
		for _, kind := range []string{"front", "back"} {
			photo, e := customers.UploadBuybackPhoto(ctx, token, store, id, buyback.PhotoUpload{ItemID: item.ID, PhotoType: kind, MimeType: "image/jpeg", Content: "/9j/"})
			if e != nil {
				t.Fatal("command photo fixture")
			}
			if firstPhoto == "" {
				firstPhoto = photo.ID
			}
		}
	}
	if _, e := update(command); !errors.Is(e, buyback.ErrCommandPhotos) {
		t.Fatal("missing required edges accepted")
	}
	if _, e := customers.UploadBuybackPhoto(ctx, token, store, id, buyback.PhotoUpload{ItemID: request.Items[0].ID, PhotoType: "edges", MimeType: "image/jpeg", Content: "/9j/"}); e != nil {
		t.Fatal("required edges")
	}
	exec("UPDATE buyback_photos SET review_status='retake' WHERE id=?", firstPhoto)
	if _, e := update(command); !errors.Is(e, buyback.ErrCommandPhotos) {
		t.Fatal("retake accepted")
	}
	exec("UPDATE buyback_photos SET review_status='pending' WHERE id=?", firstPhoto)
	if db.Transaction(func(tx *gorm.DB) error {
		_, e := buyback.StageCustomerCommand(ctx, tx, store, "cust_buyback_second", id, command)
		if !errors.Is(e, buyback.ErrMissing) {
			t.Fatal("foreign command authorized")
		}
		return nil
	}) != nil {
		t.Fatal("cross account fixture")
	}
	// Original repeated transition conflicts, without appending a second history entry.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := update(command); results <- e }()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, buyback.ErrCommandState) {
			conflicts++
		} else {
			t.Fatal("submit race error")
		}
	}
	current, e := customers.Buyback(ctx, token, store, id)
	if e != nil || wins != 1 || conflicts != 1 || current.Status != "photo_review" || len(current.History) != 2 || current.Items[0].SerialNumber != "SN-FIRST" || current.Items[0].IMEI != "000000000000001" {
		t.Fatal("submit identity/state/history")
	}
	shipped := buyback.CustomerCommand{Action: "mark_shipped", TrackingNumber: " Track 1 "}
	if _, e := update(shipped); !errors.Is(e, buyback.ErrCommandState) {
		t.Fatal("shipment before approval")
	}
	exec("UPDATE buyback_requests SET status='ready_to_ship' WHERE id=?", id)
	current, e = update(shipped)
	if e != nil || current.Status != "in_transit" || current.TrackingNumber != "Track 1" || current.ClosedAt != nil {
		t.Fatal("shipping transition")
	}
	// Platform state below is an explicit fixture; no admin workflow/payment is claimed.
	exec(`UPDATE buyback_requests SET status='final_offer',final_amount=125,raw_final_amount='{"value":"125","precision":100}' WHERE id=?`, id)
	amount := json.Number("124")
	offer := buyback.CustomerCommand{Action: "accept_offer", FinalAmount: &amount}
	if _, e := update(offer); !errors.Is(e, buyback.ErrCommandOffer) {
		t.Fatal("stale offer accepted")
	}
	amount = "125"
	results = make(chan error, 2)
	for _, in := range []buyback.CustomerCommand{offer, {Action: "decline_offer"}} {
		wg.Add(1)
		go func(in buyback.CustomerCommand) { defer wg.Done(); _, e := update(in); results <- e }(in)
	}
	wg.Wait()
	close(results)
	wins = 0
	for e := range results {
		if e == nil {
			wins++
		} else if !errors.Is(e, buyback.ErrCommandOffer) && !errors.Is(e, buyback.ErrCommandState) {
			t.Fatal("offer race error")
		}
	}
	current, e = customers.Buyback(ctx, token, store, id)
	if e != nil || wins != 1 || len(current.History) != 4 || (current.Status != "accepted" && current.Status != "return_requested") {
		t.Fatal("offer race appended multiple actions")
	}
	exec("UPDATE buyback_requests SET status='final_offer' WHERE id=?", id)
	current, e = update(offer)
	if e != nil || current.Status != "accepted" || current.ClosedAt != nil {
		t.Fatal("accept transition")
	}
	exec("UPDATE buyback_requests SET status='final_offer' WHERE id=?", id)
	current, e = update(buyback.CustomerCommand{Action: "decline_offer"})
	if e != nil || current.Status != "return_requested" || current.ClosedAt != nil {
		t.Fatal("decline transition")
	}
	if _, e := update(buyback.CustomerCommand{Action: "cancel"}); !errors.Is(e, buyback.ErrCommandState) {
		t.Fatal("cancel after receiving")
	}
	exec("UPDATE buyback_requests SET status='awaiting_photos' WHERE id=?", id)
	before, _ := customers.Buyback(ctx, token, store, id)
	late := errors.New("late command failure")
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := buyback.StageCustomerCommand(ctx, tx, store, "cust_buyback_first", id, buyback.CustomerCommand{Action: "cancel"}); e != nil {
			return e
		}
		return late
	})
	after, _ := customers.Buyback(ctx, token, store, id)
	if !errors.Is(e, late) || after.Status != before.Status || len(after.History) != len(before.History) || after.ClosedAt != nil {
		t.Fatal("late command failure leaked")
	}
	exec("UPDATE buyback_request_runtime SET enabled=FALSE WHERE id=1")
	if _, e := update(buyback.CustomerCommand{Action: "cancel"}); e == nil {
		t.Fatal("disabled commands")
	}
	exec("UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1")
	for _, v := range []struct {
		body, session, proof, path string
		status                     int
	}{{`{"action":"cancel"}`, token, "", "", 403}, {`{"action":"cancel"}`, "", csrf, "", 401}, {`{"action":"cancel","action":"cancel"}`, token, csrf, "", 400}, {`{"action":"cancel"}`, token, csrf, "?customer_id=foreign", 404}, {`{"action":"cancel"}`, token, csrf, "", 200}, {`{"action":"cancel"}`, token, csrf, "", 409}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "https://shop.example/api/store/customer/buyback/requests/"+id+v.path, strings.NewReader(v.body))
		r.Header.Set("Origin", "https://shop.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", v.proof)
		if v.session != "" {
			r.Header.Set("Cookie", "__Host-retail_customer_session="+v.session)
		}
		router.ServeHTTP(w, r)
		if w.Code != v.status {
			t.Fatalf("command HTTP got %d want %d", w.Code, v.status)
		}
	}
	current, e = customers.Buyback(ctx, token, store, id)
	if e != nil || current.Status != "canceled" || current.ClosedAt == nil {
		t.Fatal("cancel close date")
	}
	// Exhaust original customer action/state matrix using complete identities,
	// available photos and a matching quote. States are fixture setup only.
	for _, status := range []string{"awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"} {
		for _, input := range []buyback.CustomerCommand{command, shipped, offer, {Action: "decline_offer"}, {Action: "cancel"}} {
			exec("UPDATE buyback_requests SET status=?,closed_at=NULL WHERE id=?", status, id)
			allowed := input.Action == "submit_photos" && status == "awaiting_photos" || input.Action == "mark_shipped" && status == "ready_to_ship" || (input.Action == "accept_offer" || input.Action == "decline_offer") && status == "final_offer" || input.Action == "cancel" && (status == "awaiting_photos" || status == "photo_review" || status == "ready_to_ship")
			_, e := update(input)
			if (e == nil) != allowed {
				t.Fatalf("state %s action %s allowed=%v error=%v", status, input.Action, allowed, e)
			}
		}
	}
	db.Table("customer_account_notices").Count(&noticesAfter)
	if noticesAfter != noticesBefore {
		t.Fatal("customer command emitted unexpected customer notice")
	}
	exec("UPDATE buyback_requests SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", id)
	t.Log(`BUYBACK_CUSTOMER_COMMAND_REHEARSAL {"state_action_matrix":60,"identities_exact_set":true,"sn_imei_unique":true,"front_back_conditional_edges":true,"retake_rejected_pending_allowed":true,"submit_race":[1,1],"offer_accept_decline_single_winner":true,"stale_offer_rejected":true,"tracking_trimmed":true,"cancel_closed_at":true,"late_failure_atomic":true,"cross_account_denied":true,"default_gate":true,"http_csrf_session_query_duplicates":true,"customer_notice_not_emitted":true}`)
}
