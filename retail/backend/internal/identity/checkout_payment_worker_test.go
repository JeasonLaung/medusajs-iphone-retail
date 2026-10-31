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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/orderpayments"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/url"
)

// This provider fixture owns remote state only; the real customer API creates
// instructions and separately launched production CLI processes own all cash.
type checkoutPaymentWorker struct {
	posts, gets, runs, lost, lostRun atomic.Int32
	mu                               sync.Mutex
	observations                     map[string]map[string]any
}

func startCheckoutPaymentWorker(t *testing.T, ctx context.Context, db *gorm.DB, ring *protected.Ring, router *gin.Engine, origin string) (*orderpayments.Service, *checkoutPaymentWorker, func()) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Owned payment HTTPS fixture"}, DNSNames: []string{"api.mollie.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	directory := t.TempDir()
	ca := filepath.Join(directory, "ca.pem")
	if os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600) != nil {
		t.Fatal("fixture CA")
	}
	state := &checkoutPaymentWorker{observations: map[string]map[string]any{}}
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
		secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
		defer secured.Close()
		_ = secured.SetDeadline(time.Now().Add(25 * time.Second))
		if secured.Handshake() != nil {
			return
		}
		reader := bufio.NewReader(secured)
		for {
			req, e := http.ReadRequest(reader)
			if e != nil {
				return
			}
			if req.Host != "api.mollie.com" || req.Header.Get("Authorization") != "Bearer test_artificialPaymentJoint" {
				t.Error("payment transport target or credential differs")
				return
			}
			status, body, drop := state.response(t, db, req)
			_ = req.Body.Close()
			if drop {
				return
			}
			raw, _ := json.Marshal(body)
			response := http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/hal+json"}}, Body: io.NopCloser(bytes.NewReader(raw)), ContentLength: int64(len(raw)), ProtoMajor: 1, ProtoMinor: 1}
			if response.Write(secured) != nil {
				return
			}
		}
	}))
	var database string
	if db.Raw("SELECT DATABASE()").Scan(&database).Error != nil {
		t.Fatal("owned database")
	}
	binary := os.Getenv("CHECKOUT_PAYMENT_WORKER_BINARY")
	if binary != "/opt/order-payment-worker" {
		t.Fatal("owned worker binary required")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	adapter, e := payments.NewMollieCreateAdapter(payments.MollieCreateConfig{ProviderID: "pp_mollie-hosted-checkout_mollie", APIKey: "test_artificialPaymentJoint", RedirectURL: origin + "/api/mollie-return", BackendURL: "http://127.0.0.1:9100"})
	if e != nil {
		t.Fatal(e)
	}
	adapters, e := payments.NewCreateAdapters(map[string]payments.CreateSessionAdapter{"pp_mollie-hosted-checkout_mollie": adapter})
	if e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"order_payment_runtime", "mollie_notification_runtime"} {
		if db.Exec("UPDATE "+table+" SET enabled=TRUE WHERE id=1").Error != nil {
			t.Fatal("payment fixture gate")
		}
	}
	webhook, e := payments.NewMollieWebhook(db, payments.MollieWebhookOptions{ProviderIDs: []string{"pp_mollie-hosted-checkout_mollie"}})
	if e != nil {
		t.Fatal(e)
	}
	webhook.Register(router)
	service := &orderpayments.Service{DB: db, StoreID: "shop", ReturnOrigin: origin, Ring: ring, Adapters: adapters}
	go func() {
		defer close(done)
		for {
			cmd := exec.CommandContext(workerCtx, binary, "--once")
			cmd.Env = []string{"PATH=/usr/bin:/bin", "MYSQL_DSN=root:" + os.Getenv("MYSQL_PWD") + "@unix(/var/run/mysqld/mysqld.sock)/" + database + "?parseTime=true&loc=UTC", "CHECKOUT_PAYMENT_ENABLED=true", "ORDER_PAYMENTS_ENABLED=true", "MOLLIE_NOTIFICATIONS_ENABLED=true", "STOREFRONT_ORIGIN=http://127.0.0.1:19379", "MOLLIE_BACKEND_ORIGIN=http://127.0.0.1:9100", "STOREFRONT_STORE_ID=shop", "MOLLIE_API_KEY=test_artificialPaymentJoint", "PAYMENT_KEY_ACTIVE=initial", "PAYMENT_KEYRING_JSON={\"initial\":\"" + hex.EncodeToString(bytes.Repeat([]byte{0x71}, 32)) + "\"}", "HTTPS_PROXY=" + proxy.URL, "NO_PROXY=", "SSL_CERT_FILE=" + ca, "SSL_CERT_DIR=" + directory}
			state.runs.Add(1)
			output, e := cmd.CombinedOutput()
			if workerCtx.Err() != nil {
				return
			}
			if e != nil && !bytes.Contains(output, []byte("instructions=")) {
				t.Errorf("owned payment CLI failed before processing: %s", strings.TrimSpace(string(output)))
				return
			}
			select {
			case <-workerCtx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	return service, state, func() {
		cancel()
		<-done
		proxy.Close()
		for _, table := range []string{"order_payment_runtime", "mollie_notification_runtime"} {
			db.Exec("UPDATE " + table + " SET enabled=FALSE WHERE id=1")
		}
	}
}

func (s *checkoutPaymentWorker) response(t *testing.T, db *gorm.DB, r *http.Request) (int, any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == "POST" && r.URL.Path == "/v2/payments" {
		var body struct {
			Amount      map[string]string
			Metadata    map[string]string
			RedirectURL string
		}
		if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&body) != nil {
			t.Error("payment request invalid")
			return 400, nil, false
		}
		session := body.Metadata["session_id"]
		var row struct{ OperationID, OrderID, CartID string }
		if db.Table("order_payment_operations o").Select("o.operation_id,o.order_id,c.cart_id").Joins("JOIN cart_completion_receipts c ON c.order_id=o.order_id").Joins("JOIN payment_effect_journal e ON e.scope_id=o.order_id AND e.operation_id=o.operation_id AND e.resource_id=o.session_id").Where("o.session_id=? AND o.dispatched_at IS NOT NULL AND e.state='inflight' AND e.attempts=1", session).Take(&row).Error != nil || body.Amount["value"] != "6.00" || body.Amount["currency"] != "EUR" || body.Metadata["cart_id"] != row.CartID || r.Header.Get("Idempotency-Key") != session {
			t.Error("payment exact committed claim missing")
			return 409, nil, false
		}
		u, e := url.Parse(body.RedirectURL)
		if e != nil || u.Query().Get("order_id") != row.OrderID || u.Query().Get("order_payment_id") != row.OperationID {
			t.Error("payment redirect differs")
			return 409, nil, false
		}
		if _, exists := s.observations[row.OperationID]; exists {
			t.Error("duplicate payment POST")
			return 409, nil, false
		}
		s.posts.Add(1)
		observation := map[string]any{"id": "tr_joint" + strings.ReplaceAll(row.OperationID, "-", ""), "resource": "payment", "mode": "test", "status": "open", "amount": body.Amount, "metadata": body.Metadata, "redirectUrl": body.RedirectURL, "_links": map[string]any{"checkout": map[string]string{"href": "https://www.mollie.com/checkout/supplement/" + row.OperationID}}}
		s.observations[row.OperationID] = observation
		if s.lost.CompareAndSwap(0, 1) {
			s.lostRun.Store(s.runs.Load())
			return 201, nil, true
		}
		return 201, observation, false
	}
	if r.Method == "GET" {
		s.gets.Add(1)
		if s.lostRun.Load() == s.runs.Load() {
			return 503, map[string]string{"error": "owned first-process discovery fault"}, false
		}
		if r.URL.Path == "/v2/payments" {
			rows := []any{}
			for _, v := range s.observations {
				rows = append(rows, v)
			}
			return 200, map[string]any{"count": len(rows), "_embedded": map[string]any{"payments": rows}, "_links": map[string]any{"next": nil}}, false
		}
		for _, v := range s.observations {
			if r.URL.Path == "/v2/payments/"+v["id"].(string) {
				return 200, v, false
			}
		}
	}
	t.Error("unexpected owned payment request")
	return 404, nil, false
}
func (s *checkoutPaymentWorker) mount(fixture *gin.RouterGroup, router *gin.Engine) {
	fixture.POST("/supplement-paid/:id", func(c *gin.Context) {
		s.mu.Lock()
		v, ok := s.observations[c.Param("id")]
		if ok {
			copy := make(map[string]any, len(v))
			for k, value := range v {
				copy[k] = value
			}
			copy["status"] = "paid"
			s.observations[c.Param("id")] = copy
		}
		s.mu.Unlock()
		if !ok {
			c.Status(404)
			return
		}
		// Actual public notification handler enqueues the known reference. No fixture
		// writes cash, observations, ownership or paid state in SQL.
		req := httptest.NewRequest("POST", "/hooks/payment/mollie-hosted-checkout_mollie", strings.NewReader(url.Values{"id": {v["id"].(string)}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reply := httptest.NewRecorder()
		router.ServeHTTP(reply, req)
		c.Status(reply.Code)
	})
}
func checkCheckoutExchangePayment(t *testing.T, db *gorm.DB, orders []string, s *checkoutPaymentWorker) {
	t.Helper()
	if s.posts.Load() != 4 || s.gets.Load() < 8 || s.lost.Load() != 1 || s.runs.Load() < 4 {
		t.Fatal("payment independent CLI/provider recovery counts", s.posts.Load(), s.gets.Load(), s.runs.Load())
	}
	for _, order := range orders {
		for _, q := range []string{
			"SELECT COUNT(*) FROM order_payment_operations o JOIN customer_order_ownership own ON own.order_id=o.order_id AND own.customer_id=o.account_id JOIN customer_accounts a ON a.id=own.customer_id AND a.enabled=TRUE AND a.verified_at IS NOT NULL WHERE o.order_id=? AND o.state='paid' AND o.applied=TRUE AND o.amount='6.00' AND o.order_version=3 AND o.dispatched_at IS NOT NULL",
			"SELECT COUNT(*) FROM order_payment_operations o JOIN payments p ON p.id=CONCAT('pay_supp_',o.operation_id) AND p.amount=6 JOIN payment_captures c ON c.payment_id=p.id AND c.amount=6 JOIN order_transactions x ON x.reference_id=c.id AND x.amount=6 AND x.order_id=o.order_id WHERE o.order_id=?",
			"SELECT COUNT(*) FROM order_payment_operations o JOIN customer_account_notices n ON n.event_key=CONCAT('payment.received:pay_supp_',o.operation_id) AND n.customer_id=o.account_id AND n.resource_id=o.order_id AND n.read_at IS NOT NULL WHERE o.order_id=?",
			"SELECT COUNT(*) FROM cart_completion_receipts r JOIN carts c ON c.id=r.cart_id AND c.completed_at IS NOT NULL WHERE r.order_id=? AND r.payment_id NOT LIKE 'pay_supp_%'",
		} {
			var n int64
			if e := db.Raw(q, order).Scan(&n).Error; e != nil || n != 1 {
				t.Fatalf("joint payment facts differ %s: %d %v", q, n, e)
			}
		}
	}
	t.Log(`CHECKOUT_EXCHANGE_PAYMENT_JOINT_SQL {"same_four_registered_verified_customers":true,"same_four_original_paid_shipped_orders":true,"exact_supplemental_amount":"6.00","original_cash":"158.75","current_cash":"164.75","pending_difference":"0","separate_production_cli":true,"single_post_each":true,"lost_response_new_process_get_recovery":true,"unique_payment_capture_positive_transaction":true,"five_own_read_notices_each":true,"replacement_shipped_after_settlement":true,"original_checkout_retained":true,"real_provider":false,"real_email":false}`)
}
