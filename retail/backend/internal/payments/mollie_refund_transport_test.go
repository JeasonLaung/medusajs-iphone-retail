package payments

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func localMollieTLS(t *testing.T, handler http.Handler) (*MollieCreateAdapter, func()) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	a, e := NewMollieCreateAdapter(mollieCreateConfig())
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(server.URL)
	transport := server.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = server.Certificate().DNSNames[0]
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "api.mollie.com:443" {
			t.Error("credentials could leave owned channel")
		}
		return (&net.Dialer{}).DialContext(ctx, network, u.Host)
	}
	a.transport.client.Transport = transport
	return a, func() { transport.CloseIdleConnections(); server.Close() }
}
func TestMollieRefundTransportDoesNotReplayCommittedPOST(t *testing.T) {
	for _, kind := range []string{"refund", "create"} {
		t.Run(kind, func(t *testing.T) {
			in := refundFixtureRequest()
			var posts atomic.Int32
			a, cleanup := localMollieTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Host != "api.mollie.com" {
					t.Error("provider host changed")
				}
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(refundParent())
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				if posts.Add(1) == 1 {
					c, _, e := w.(http.Hijacker).Hijack()
					if e != nil {
						t.Error(e)
						return
					}
					_ = c.Close()
					return
				}
				w.WriteHeader(201)
				if kind == "refund" {
					_ = json.NewEncoder(w).Encode(refundFixture(in, "pending"))
				} else {
					_ = json.NewEncoder(w).Encode(mollieCreateFixture())
				}
			}))
			defer cleanup()
			// Reuse a verified GET connection, exercising net/http's real retry path.
			if _, e := a.ObserveSession(context.Background(), in.Session, in.RemotePaymentID); e != nil {
				t.Fatal(e)
			}
			if kind == "refund" {
				out, e := a.CreateRefund(context.Background(), in)
				if e != ErrRefundExecution || out != (RefundObservation{}) {
					t.Error("lost refund POST was repeated or accepted", e)
				}
			} else {
				state, _, e := a.CreateSession(context.Background(), in.Session)
				if e == nil || state != CreateUncertain {
					t.Error("lost create POST was repeated or accepted", e)
				}
			}
			if posts.Load() != 1 {
				t.Fatalf("HTTP transport replayed committed %s POST: %d", kind, posts.Load())
			}
		})
	}
}
func TestMollieRefundTransportForbidsRedirect(t *testing.T) {
	in := refundFixtureRequest()
	var calls atomic.Int32
	a, cleanup := localMollieTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(refundParent())
			return
		}
		w.Header().Set("Location", "https://other.invalid/private")
		w.WriteHeader(307)
	}))
	defer cleanup()
	if _, e := a.CreateRefund(context.Background(), in); e != ErrRefundExecution || calls.Load() != 2 {
		t.Fatal("redirect followed or accepted", e, calls.Load())
	}
}
