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
	"sync/atomic"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/payments"
	"gorm.io/gorm"
)

// The API only prepares commands. The separately launched production binary
// must use its real configured Mollie adapter through the owned HTTPS fixture.
type checkoutRefundAdapter struct{ executionObserver }

func (checkoutRefundAdapter) CreateRefund(context.Context, payments.RefundRequest) (payments.RefundObservation, error) {
	return payments.RefundObservation{}, payments.ErrRefundExecution
}
func (checkoutRefundAdapter) ObserveRefund(context.Context, payments.RefundRequest, string) (payments.RefundObservation, error) {
	return payments.RefundObservation{}, payments.ErrRefundExecution
}
func (checkoutRefundAdapter) DiscoverRefund(context.Context, payments.RefundRequest) (payments.RefundObservation, error) {
	return payments.RefundObservation{}, payments.ErrRefundExecution
}

type checkoutRefundWorker struct {
	posts, gets, runs, lost, rollback, localRecovery atomic.Int32
	firstOperation                                   atomic.Value
	mu                                               sync.Mutex
	observations                                     map[string]map[string]any
}

func startCheckoutRefundWorker(t *testing.T, ctx context.Context, db *gorm.DB) (*checkoutRefundWorker, func()) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Owned refund HTTPS fixture"}, DNSNames: []string{"api.mollie.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
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
	state := &checkoutRefundWorker{observations: map[string]map[string]any{}}
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
			if req.Host != "api.mollie.com" || req.Header.Get("Authorization") != "Bearer test_artificialRefundJoint" {
				t.Error("refund transport target or credential differs")
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
	binary := os.Getenv("CHECKOUT_REFUND_WORKER_BINARY")
	if binary != "/opt/refund-worker" {
		t.Fatal("owned worker binary required")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			cmd := exec.CommandContext(workerCtx, binary, "--once")
			cmd.Env = []string{"PATH=/usr/bin:/bin", "MYSQL_DSN=root:" + os.Getenv("MYSQL_PWD") + "@unix(/var/run/mysqld/mysqld.sock)/" + database + "?parseTime=true&loc=UTC", "CHECKOUT_PAYMENT_ENABLED=true", "ORDER_REFUNDS_ENABLED=true", "STOREFRONT_ORIGIN=http://127.0.0.1:19379", "MOLLIE_BACKEND_ORIGIN=http://127.0.0.1:9100", "STOREFRONT_STORE_ID=shop", "MOLLIE_API_KEY=test_artificialRefundJoint", "PAYMENT_KEY_ACTIVE=initial", "PAYMENT_KEYRING_JSON={\"initial\":\"" + hex.EncodeToString(bytes.Repeat([]byte{0x71}, 32)) + "\"}", "HTTPS_PROXY=" + proxy.URL, "NO_PROXY=", "MOLLIE_CA_FILE=" + ca, "SSL_CERT_FILE=" + ca, "SSL_CERT_DIR=" + directory}
			state.runs.Add(1)
			output, e := cmd.CombinedOutput()
			if workerCtx.Err() != nil {
				return
			}
			if e != nil && !bytes.Contains(output, []byte("selected=")) {
				t.Errorf("owned refund CLI failed before processing: %s", strings.TrimSpace(string(output)))
				return
			}
			if first := state.firstOperation.Load(); first != nil {
				id := first.(string)
				var row struct {
					State   string
					Applied bool
				}
				if db.Table("order_refund_operations").Select("state,applied").Where("operation_id=?", id).Take(&row).Error != nil {
					t.Error("refund recovery receipt unavailable")
					return
				}
				if row.State == "refunded" && !row.Applied && state.rollback.Load() == 0 {
					for _, query := range []string{"SELECT COUNT(*) FROM payment_refunds WHERE id=CONCAT('ref_',?)", "SELECT COUNT(*) FROM order_transactions WHERE reference_id=CONCAT('ref_',?)", "SELECT COUNT(*) FROM customer_account_notices WHERE event_key=CONCAT('refund.created:ref_',?)"} {
						var n int64
						if db.Raw(query, id).Scan(&n).Error != nil || n != 0 {
							t.Error("late refund notice failure left a partial movement")
							return
						}
					}
					if db.Exec("DROP TRIGGER refund_notice_joint_failure").Error != nil {
						t.Error("owned notice fault cleanup failed")
						return
					}
					state.rollback.Store(1)
				} else if row.Applied && state.rollback.Load() == 1 {
					state.localRecovery.Store(1)
				}
			}
			select {
			case <-workerCtx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	return state, func() { cancel(); <-done; proxy.Close(); db.Exec("DROP TRIGGER IF EXISTS refund_notice_joint_failure") }
}
func (s *checkoutRefundWorker) response(t *testing.T, db *gorm.DB, r *http.Request) (int, any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v2" || parts[1] != "payments" {
		t.Error("unexpected refund API path")
		return 404, nil, false
	}
	payment := parts[2]
	if r.Method == "GET" && len(parts) == 3 {
		s.gets.Add(1)
		var row struct{ SessionID, CartID, Amount, ScopeID, OrderID, CheckoutOperationID string }
		if db.Table("order_refund_operations o").Select("o.session_id,c.cart_id,o.scope_id,p.amount,o.order_id,o.checkout_operation_id").Joins("JOIN cart_completion_receipts c ON c.order_id=o.order_id").Joins("JOIN payments p ON p.id=o.payment_id").Where("o.remote_payment_id=?", payment).Take(&row).Error != nil {
			t.Error("refund original session unavailable")
			return 404, nil, false
		}
		amount, err := payments.MollieSettlementAmount(row.Amount)
		if err != nil {
			t.Error("refund payment amount invalid")
			return 409, nil, false
		}
		if row.ScopeID != row.CartID && (os.Getenv("CHECKOUT_SUPPLEMENT_REFUND") != "1" || amount != "6.00") {
			t.Error("refund must target this supplemental capture")
			return 409, nil, false
		}
		redirect := "http://127.0.0.1:19379/api/mollie-return?cart_id=" + row.CartID
		if row.ScopeID != row.CartID {
			redirect = "http://127.0.0.1:19379/api/mollie-return?" + url.Values{"cart_id": {row.CartID}, "order_id": {row.OrderID}, "order_payment_id": {row.CheckoutOperationID}}.Encode()
		}
		return 200, map[string]any{"id": payment, "resource": "payment", "redirectUrl": redirect, "mode": "test", "status": "paid", "amount": map[string]string{"value": amount, "currency": "EUR"}, "amountRemaining": map[string]string{"value": amount, "currency": "EUR"}, "metadata": map[string]string{"session_id": row.SessionID, "cart_id": row.CartID}}, false
	}
	if len(parts) < 4 || parts[3] != "refunds" {
		t.Error("unexpected refund resource")
		return 404, nil, false
	}
	if r.Method == "POST" && len(parts) == 4 {
		var body struct {
			Amount   map[string]string
			Metadata map[string]string
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body) != nil {
			t.Error("invalid provider refund body")
			return 400, nil, false
		}
		id := body.Metadata["refund_operation_id"]
		var n int64
		if db.Table("order_refund_operations").Where("operation_id=? AND dispatched_at IS NOT NULL", id).Count(&n).Error != nil || n != 1 || r.Header.Get("Idempotency-Key") != id {
			t.Error("refund POST before durable claim")
			return 409, nil, false
		}
		if _, exists := s.observations[id]; exists {
			t.Error("refund was POSTed twice")
			return 409, nil, false
		}
		s.posts.Add(1)
		observation := map[string]any{"resource": "refund", "id": "re_" + strings.ReplaceAll(id, "-", ""), "paymentId": payment, "mode": "test", "status": "refunded", "amount": body.Amount, "metadata": body.Metadata, "createdAt": time.Now().UTC().Format(time.RFC3339Nano)}
		s.observations[id] = observation
		// Persist the provider-side result, then drop exactly one response. A fresh
		// CLI must discover the same refund after the original durable lease expires.
		if s.lost.CompareAndSwap(0, 1) {
			if db.Exec("CREATE TRIGGER refund_notice_joint_failure BEFORE INSERT ON customer_account_notices FOR EACH ROW BEGIN IF NEW.kind='refund.created' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned notice fault'; END IF; END").Error != nil {
				t.Error("owned refund notice fault unavailable")
				return 500, nil, false
			}
			s.firstOperation.Store(id)
			return 201, nil, true
		}
		return 201, observation, false
	}
	if r.Method == "GET" {
		s.gets.Add(1)
		rows := []any{}
		for _, v := range s.observations {
			if v["paymentId"] == payment {
				if len(parts) == 5 && parts[4] == v["id"] {
					return 200, v, false
				}
				rows = append(rows, v)
			}
		}
		if len(parts) == 4 {
			return 200, map[string]any{"count": len(rows), "_embedded": map[string]any{"refunds": rows}, "_links": map[string]any{"next": nil}}, false
		}
	}
	t.Error("unexpected provider refund operation")
	return 404, nil, false
}
