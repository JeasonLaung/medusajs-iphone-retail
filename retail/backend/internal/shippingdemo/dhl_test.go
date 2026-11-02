package shippingdemo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

const sample = `{"shipments":[{"id":"official-sample","service":"express","events":[],"extra":{"preserved":true}}]}`

func TestFixedSampleCacheAndExpiry(t *testing.T) {
	s := New(false)
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	calls := 0
	s.client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != endpoint || r.Method != "GET" || r.Header.Get("DHL-API-Key") != "demo-key" || r.Header.Get("Accept") != "application/json" {
			t.Error("changed official fixed query")
		}
		return response(200, sample), nil
	})
	first, err := s.get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(first, &body) != nil || string(body["requested_tracking_number"]) != `"7777777770"` || !strings.Contains(string(body["shipment"]), `"extra"`) {
		t.Fatalf("sample envelope: %s", first)
	}
	now = now.Add(30*time.Second - time.Millisecond)
	again, err := s.get(context.Background())
	if err != nil || string(first) != string(again) || calls != 1 {
		t.Fatal("cache not retained for 30 seconds")
	}
	now = now.Add(time.Millisecond)
	again, err = s.get(context.Background())
	if err != nil || string(first) == string(again) || calls != 2 {
		t.Fatal("expired sample not refreshed")
	}
}

func TestConcurrentQueryIsShared(t *testing.T) {
	s := New(false)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	s.client.Transport = transport(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return response(200, sample), nil
	})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.get(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.get(ctx); !errors.Is(err, context.Canceled) {
		t.Error("waiting viewer cancellation not respected")
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent upstream calls: %d", calls.Load())
	}
}

func TestFailureRetryAndValidation(t *testing.T) {
	for _, bad := range []string{`{`, `{}`, sample + `{}`, `{"shipments":[]}`, `{"shipments":[{"events":[]}]}`, `{"shipments":[{"id":"x","events":null}]}`, `{"shipments":[{"id":"x","events":{}}]}`} {
		t.Run(bad, func(t *testing.T) {
			s := New(false)
			calls := 0
			s.client.Transport = transport(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return response(200, bad), nil
				}
				return response(200, sample), nil
			})
			if _, err := s.get(context.Background()); err == nil {
				t.Fatal("accepted malformed sample")
			}
			if _, err := s.get(context.Background()); err != nil || calls != 2 {
				t.Fatal("failed query poisoned retry")
			}
		})
	}
	for _, status := range []int{301, 401, 429, 500} {
		s := New(false)
		s.client.Transport = transport(func(*http.Request) (*http.Response, error) { return response(status, sample), nil })
		if _, err := s.get(context.Background()); err == nil {
			t.Errorf("accepted HTTP %d", status)
		}
	}
}

func TestHTTPProductionAndNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, production := range []bool{false, true} {
		s := New(production)
		calls := 0
		s.client.Transport = transport(func(*http.Request) (*http.Response, error) { calls++; return response(200, sample), nil })
		r := gin.New()
		r.GET("/api/store/shipping-demo/dhl", s.HTTP)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/store/shipping-demo/dhl?trackingNumber=real-order", nil))
		want := 200
		if production {
			want = 404
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status/cache: %d %v", w.Code, w.Header())
		}
		if production && calls != 0 {
			t.Fatal("production queried DHL")
		}
		if !production && !strings.Contains(w.Body.String(), "7777777770") {
			t.Fatal("query altered fixed sample")
		}
	}
	s := New(false)
	s.client.Transport = transport(func(*http.Request) (*http.Response, error) { return nil, errors.New("private upstream detail") })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	s.HTTP(c)
	if w.Code != 502 || strings.Contains(w.Body.String(), "private upstream detail") || !strings.Contains(w.Body.String(), "DHL_DEMO_UNAVAILABLE") {
		t.Fatal("failure boundary changed")
	}
}

func TestTimeoutAndNoRedirect(t *testing.T) {
	s := New(false)
	if s.client.Timeout != 15*time.Second {
		t.Fatal("changed timeout")
	}
	if !errors.Is(s.client.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("redirects allowed")
	}
	s.client.Timeout = 10 * time.Millisecond
	s.client.Transport = transport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	if _, err := s.get(context.Background()); err == nil {
		t.Fatal("timeout accepted")
	}
}

// Explicit opt-in: a read-only request for the same public official sample as
// the source application; never accepts a customer shipment or credentials.
func TestOfficialSampleLive(t *testing.T) {
	if os.Getenv("DHL_DEMO_LIVE_TEST") != "1" {
		t.Skip("official sample network check is opt-in")
	}
	s := New(false)
	body, err := s.get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Requested string `json:"requested_tracking_number"`
		Shipment  struct {
			ID     string            `json:"id"`
			Events []json.RawMessage `json:"events"`
		} `json:"shipment"`
	}
	if json.Unmarshal(body, &result) != nil || result.Requested != "7777777770" || result.Shipment.ID == "" {
		t.Fatal("invalid official sample")
	}
	t.Logf("official fixed sample received; events=%d", len(result.Shipment.Events))
}
