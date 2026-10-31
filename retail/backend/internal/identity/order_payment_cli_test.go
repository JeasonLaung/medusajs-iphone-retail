package identity_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/orderpayments"
	"gorm.io/gorm"
)

// The real separately compiled production CLI is the only caller of the owned
// HTTPS payment fixture. The container has no external network. SQL ownership,
// catalog and device receipt remain artificial; this is not customer browser or
// a real merchant acceptance test.
func checkOrderPaymentCLI(t *testing.T, ctx context.Context, db *gorm.DB, s orderpayments.Service, principal orderpayments.Principal, in orderpayments.Input, cartID string) {
	t.Helper()
	if os.Getenv("ORDER_PAYMENT_CLI_BINARY") != "/opt/order-payment-worker" {
		t.Fatal("owned production CLI required")
	}
	if _, e := s.Prepare(ctx, principal, in); e != nil {
		t.Fatal("CLI prepare", e)
	}
	var session string
	if db.Table("order_payment_operations").Select("session_id").Where("operation_id=?", in.OperationID).Scan(&session).Error != nil || session == "" {
		t.Fatal("CLI saved session")
	}
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Owned payment CLI fixture"}, DNSNames: []string{"api.mollie.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	directory := t.TempDir()
	ca := filepath.Join(directory, "ca.pem")
	if os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600) != nil {
		t.Fatal("CLI fixture CA")
	}
	var mu sync.Mutex
	phase, posts, lists, reads := 0, 0, 0, 0
	var payment map[string]any
	response := func(req *http.Request) (int, any, bool) {
		mu.Lock()
		defer mu.Unlock()
		if req.Host != "api.mollie.com" || req.Header.Get("Authorization") != "Bearer test_artificialPaymentCLI" || phase >= 2 {
			t.Error("unexpected payment CLI network access")
			return 403, nil, false
		}
		if req.Method == "POST" && req.URL.Path == "/v2/payments" {
			posts++
			var body struct {
				Amount      map[string]string
				Metadata    map[string]string
				RedirectURL string
			}
			var claimed int64
			if posts != 1 || json.NewDecoder(io.LimitReader(req.Body, 8192)).Decode(&body) != nil || body.Amount["value"] != "6.00" || body.Amount["currency"] != "EUR" || body.Metadata["session_id"] != session || body.Metadata["cart_id"] != cartID || req.Header.Get("Idempotency-Key") != session || db.Table("order_payment_operations o").Joins("JOIN payment_effect_journal e ON e.scope_id=o.order_id AND e.operation_id=o.operation_id AND e.resource_id=o.session_id").Where("o.operation_id=? AND o.dispatched_at IS NOT NULL AND e.state='inflight' AND e.attempts=1", in.OperationID).Count(&claimed).Error != nil || claimed != 1 {
				t.Error("CLI POST lacks exact immutable request and committed claim")
				return 409, nil, false
			}
			u, err := url.Parse(body.RedirectURL)
			if err != nil || u.Query().Get("order_id") != in.OrderID || u.Query().Get("order_payment_id") != in.OperationID || u.Query().Get("cart_id") != cartID {
				t.Error("CLI redirect scope differs")
				return 409, nil, false
			}
			payment = map[string]any{"id": "tr_ownedSupplementCLI", "resource": "payment", "mode": "test", "status": "paid", "amount": body.Amount, "metadata": body.Metadata, "redirectUrl": body.RedirectURL, "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/ownedSupplementCLI"}}}
			// Remote effect exists, but no response reaches the first CLI.
			return 201, nil, true
		}
		if req.Method != "GET" || req.Header.Get("Idempotency-Key") != "" {
			t.Error("CLI recovery used a mutating request")
			return 405, nil, false
		}
		if req.URL.Path == "/v2/payments" {
			lists++
			if phase == 0 {
				return 503, nil, false // First process cannot recover its lost POST.
			}
			return 200, map[string]any{"count": 1, "_embedded": map[string]any{"payments": []any{payment}}, "_links": map[string]any{"next": nil}}, false
		}
		if req.URL.Path == "/v2/payments/tr_ownedSupplementCLI" {
			reads++
			return 200, payment, false
		}
		t.Error("unexpected CLI provider route")
		return 404, nil, false
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "api.mollie.com:443" {
			http.Error(w, "Owned target only", 403)
			return
		}
		conn, rw, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
		defer secured.Close()
		_ = secured.SetDeadline(time.Now().Add(20 * time.Second))
		if secured.Handshake() != nil {
			return
		}
		reader := bufio.NewReader(secured)
		for {
			req, e := http.ReadRequest(reader)
			if e != nil {
				return
			}
			status, body, drop := response(req)
			_ = req.Body.Close()
			if drop {
				return
			}
			raw, _ := json.Marshal(body)
			result := http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/hal+json"}}, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw)), ProtoMajor: 1, ProtoMinor: 1}
			if result.Write(secured) != nil {
				return
			}
		}
	}))
	defer proxy.Close()
	var database string
	if db.Raw("SELECT DATABASE()").Scan(&database).Error != nil {
		t.Fatal("CLI database")
	}
	pids := map[int]bool{}
	run := func(success bool) {
		t.Helper()
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(runCtx, "/opt/order-payment-worker", "--once")
		cmd.Dir = directory // No repository .env can enter the explicit test config.
		cmd.Env = []string{"PATH=/usr/bin:/bin", "MYSQL_DSN=root:" + os.Getenv("MYSQL_PWD") + "@unix(/var/run/mysqld/mysqld.sock)/" + database + "?parseTime=true&loc=UTC", "CHECKOUT_PAYMENT_ENABLED=true", "ORDER_PAYMENTS_ENABLED=true", "MOLLIE_NOTIFICATIONS_ENABLED=true", "STOREFRONT_ORIGIN=" + s.ReturnOrigin, "MOLLIE_BACKEND_ORIGIN=http://127.0.0.1:9100", "STOREFRONT_STORE_ID=shop", "MOLLIE_API_KEY=test_artificialPaymentCLI", "PAYMENT_KEY_ACTIVE=initial", "PAYMENT_KEYRING_JSON={\"initial\":\"" + hex.EncodeToString(bytes.Repeat([]byte{0x71}, 32)) + "\"}", "HTTPS_PROXY=" + proxy.URL, "NO_PROXY=", "SSL_CERT_FILE=" + ca, "SSL_CERT_DIR=" + directory}
		out, e := cmd.CombinedOutput()
		if cmd.Process == nil || pids[cmd.Process.Pid] {
			t.Fatal("CLI did not start a distinct process")
		}
		pids[cmd.Process.Pid] = true
		if (e == nil) != success || (!success && !strings.Contains(string(out), "Order payment instructions=")) || runCtx.Err() != nil || strings.Contains(string(out), "test_artificial") || strings.Contains(string(out), "tr_owned") || strings.Contains(string(out), in.OperationID) {
			t.Fatalf("CLI exit or private logging differs (success=%v, exit=%v)", success, e)
		}
	}
	count := func(table, where string, args ...any) int64 {
		t.Helper()
		var n int64
		if db.Table(table).Where(where, args...).Count(&n).Error != nil {
			t.Fatal("CLI count", table)
		}
		return n
	}
	assertUnpaid := func() {
		t.Helper()
		if count("payments", "payment_collection_id=?", in.CollectionID) != 0 || count("payment_captures", "id=?", "cap_supp_"+in.OperationID) != 0 || count("order_transactions", "id=?", "ordtr_supp_"+in.OperationID) != 0 || count("order_payment_operations", "operation_id=? AND applied=TRUE", in.OperationID) != 0 {
			t.Fatal("CLI partial cash escaped rollback")
		}
	}
	run(false)
	assertUnpaid()
	if count("order_payment_operations", "operation_id=? AND dispatched_at IS NOT NULL", in.OperationID) != 1 || count("payment_effect_journal", "scope_id=? AND operation_id=? AND state='uncertain'", in.OrderID, in.OperationID) != 1 {
		t.Fatal("lost POST not durable after first CLI exit")
	}
	mu.Lock()
	phase = 1
	mu.Unlock()
	if db.Exec("CREATE TRIGGER owned_payment_cli_ack_failure BEFORE UPDATE ON mollie_notification_observations FOR EACH ROW BEGIN IF NEW.consumed_at IS NOT NULL AND NEW.remote_id='tr_ownedSupplementCLI' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned CLI final ACK failure'; END IF; END").Error != nil {
		t.Fatal("CLI ACK fault")
	}
	defer db.Exec("DROP TRIGGER IF EXISTS owned_payment_cli_ack_failure")
	run(false)
	assertUnpaid()
	if count("payment_effect_journal", "scope_id=? AND operation_id=? AND state='succeeded' AND result_envelope IS NOT NULL", in.OrderID, in.OperationID) != 1 || count("mollie_notification_observations", "remote_id=? AND consumed_at IS NULL AND result_envelope IS NOT NULL", "tr_ownedSupplementCLI") != 1 {
		t.Fatal("CLI protected paid evidence absent after ACK rollback")
	}
	if db.Exec("DROP TRIGGER owned_payment_cli_ack_failure").Error != nil {
		t.Fatal("CLI ACK fault cleanup")
	}
	for _, q := range []struct {
		sql string
		arg any
	}{
		{"UPDATE customer_accounts SET enabled=FALSE,auth_version=auth_version+1 WHERE id=?", principal.AccountID},
		{"UPDATE carts SET deleted_at=UTC_TIMESTAMP(6) WHERE id=?", cartID},
		{"UPDATE cart_ownership SET active=FALSE,checkout_attempt_id=NULL WHERE cart_id=?", cartID},
		{"UPDATE orders SET status='canceled',canceled_at=UTC_TIMESTAMP(6) WHERE id=?", in.OrderID},
	} {
		if db.Exec(q.sql, q.arg).Error != nil {
			t.Fatal("CLI retirement fixture")
		}
	}
	if db.Exec("UPDATE order_payment_runtime SET enabled=FALSE WHERE id=1").Error != nil {
		t.Fatal("CLI disable new dispatch")
	}
	mu.Lock()
	phase = 2
	beforeLists, beforeReads := lists, reads
	mu.Unlock()
	run(true)
	run(true)
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || lists < 1 || reads < 2 || lists != beforeLists || reads != beforeReads || len(pids) != 4 {
		t.Fatal("CLI recovery network/process counts", posts, lists, reads, len(pids))
	}
	if count("payments", "payment_collection_id=?", in.CollectionID) != 1 || count("payment_captures", "id=?", "cap_supp_"+in.OperationID) != 1 || count("order_transactions", "id=?", "ordtr_supp_"+in.OperationID) != 1 || count("order_payment_operations", "operation_id=? AND state='paid' AND applied=TRUE", in.OperationID) != 1 || count("orders", "id=? AND status='canceled'", in.OrderID) != 1 || count("mollie_notification_observations", "remote_id=? AND consumed_at IS NOT NULL", "tr_ownedSupplementCLI") != 1 {
		t.Fatal("CLI final cash/ACK not unique or canceled order reopened")
	}
	var due, cash string
	if db.Table("order_summaries").Select("JSON_UNQUOTE(JSON_EXTRACT(totals,'$.pending_difference'))").Where("order_id=? AND version=3", in.OrderID).Scan(&due).Error != nil || db.Table("order_summaries").Select("JSON_UNQUOTE(JSON_EXTRACT(totals,'$.transaction_total'))").Where("order_id=? AND version=3", in.OrderID).Scan(&cash).Error != nil || due != "0" || cash != "164.75" {
		t.Fatal("CLI exact cash summary", due, cash)
	}
	t.Log(`ORDER_PAYMENT_CLI_REHEARSAL {"production_binary":true,"distinct_processes":4,"owned_tls_proxy":true,"durable_claim_before_post":true,"single_post":true,"lost_response_process_exit":true,"new_process_get_discovery":true,"protected_creation_and_paid_observation":true,"last_ack_rolls_back_cash":true,"new_process_local_reapply_without_network":true,"revoked_account_retired_cart_disabled_feature_recovery":true,"canceled_order_not_reopened":true,"unique_payment_capture_transaction_ack":true,"cash_total":"164.75","pending_difference":"0","idle_process":true,"real_provider":false,"registration_browser":false,"payment_notice":false}`)
}
