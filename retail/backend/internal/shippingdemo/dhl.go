// Package shippingdemo preserves the development-only official DHL sample from
// medusa-store/apps/backend/src/lib/dhl-official-demo.ts. It never tracks orders.
package shippingdemo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const endpoint = "https://api-eu.dhl.com/track/shipments?trackingNumber=7777777770&service=express"

var errUnavailable = errors.New("DHL demo unavailable")

type result struct {
	body []byte
	err  error
	done chan struct{}
}

type Service struct {
	production bool
	client     *http.Client
	now        func() time.Time
	mu         sync.Mutex
	cached     []byte
	fetched    time.Time
	pending    *result
}

func New(production bool) *Service {
	return &Service{production: production, now: time.Now, client: &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *Service) HTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if s.production {
		c.JSON(http.StatusNotFound, gin.H{"code": "not_found"})
		return
	}
	body, err := s.get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": "DHL_DEMO_UNAVAILABLE"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func (s *Service) get(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	if s.cached != nil && s.now().Sub(s.fetched) < 30*time.Second {
		body := s.cached
		s.mu.Unlock()
		return body, nil
	}
	if p := s.pending; p != nil {
		s.mu.Unlock()
		select {
		case <-p.done:
			return p.body, p.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p := &result{done: make(chan struct{})}
	s.pending = p
	s.mu.Unlock()
	// The shared query has its own timeout: disconnecting one viewer must not
	// cancel the other viewers' request or poison the cache.
	body, fetched, err := s.fetch()
	s.mu.Lock()
	if err == nil {
		s.cached, s.fetched = body, fetched
	}
	p.body, p.err = body, err
	s.pending = nil
	close(p.done)
	s.mu.Unlock()
	return body, err
}

func (s *Service) fetch() ([]byte, time.Time, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("DHL-API-Key", "demo-key")
	req.Header.Set("Accept", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, time.Time{}, errUnavailable
	}
	var envelope struct {
		Shipments []json.RawMessage `json:"shipments"`
	}
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&envelope); err != nil {
		return nil, time.Time{}, errUnavailable
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, time.Time{}, errUnavailable
	}
	if len(envelope.Shipments) == 0 {
		return nil, time.Time{}, errUnavailable
	}
	var shipment struct {
		ID     string          `json:"id"`
		Events json.RawMessage `json:"events"`
	}
	if json.Unmarshal(envelope.Shipments[0], &shipment) != nil || shipment.ID == "" || len(shipment.Events) == 0 || shipment.Events[0] != '[' {
		return nil, time.Time{}, errUnavailable
	}
	fetched := s.now()
	body, err := json.Marshal(struct {
		Requested string          `json:"requested_tracking_number"`
		Fetched   string          `json:"fetched_at"`
		Source    string          `json:"source"`
		Shipment  json.RawMessage `json:"shipment"`
	}{"7777777770", fetched.UTC().Format("2006-01-02T15:04:05.000Z"), "https://developer.dhl.com/tracking", envelope.Shipments[0]})
	return body, fetched, err
}
