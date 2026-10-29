package sessionstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

const CartScopeTTL = 12 * time.Hour

type CartScope struct {
	StoreID string
	Hash    [32]byte
}

type cartScopeRecord struct {
	Format   int    `json:"format"`
	StoreID  string `json:"store_id"`
	CSRFHash string `json:"csrf_hash"`
	Expires  int64  `json:"expires"`
}

// IssueCartScope establishes an anonymous creation/retry identity. It does not
// grant access to ANY cart and cannot be used as customer/admin authentication.
func (s *Store) IssueCartScope(ctx context.Context, store string) (Issued, error) {
	if !cartIDPattern.MatchString(store) {
		return Issued{}, ErrInvalid
	}
	token, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	expires := s.now().Add(CartScopeTTL).Unix()
	data, err := json.Marshal(cartScopeRecord{1, store, digest(csrf), expires})
	if err != nil {
		return Issued{}, ErrUnavailable
	}
	ok, err := s.client.SetNX(ctx, s.key(cartRealm, "creation-scope", token), data, CartScopeTTL).Result()
	if err != nil || !ok {
		return Issued{}, ErrUnavailable
	}
	return Issued{Token: token, CSRF: csrf, ExpiresAt: time.Unix(expires, 0).UTC()}, nil
}

// ResolveCartScope requires the cookie AND CSRF; its hash scopes durable
// idempotency receipts. Browser IDs/headers never select that hash or store.
func (s *Store) ResolveCartScope(ctx context.Context, token, csrf string) (CartScope, error) {
	if !tokenOK(token) {
		return CartScope{}, ErrMissing
	}
	data, err := s.client.Get(ctx, s.key(cartRealm, "creation-scope", token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return CartScope{}, ErrMissing
	}
	if err != nil {
		return CartScope{}, ErrUnavailable
	}
	var r cartScopeRecord
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if len(data) > 2048 || d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Format != 1 || !cartIDPattern.MatchString(r.StoreID) || !tokenOK(r.CSRFHash) || r.Expires <= s.now().Unix() {
		return CartScope{}, ErrMissing
	}
	if !csrfMatches(r.CSRFHash, csrf) {
		return CartScope{}, ErrCSRF
	}
	return CartScope{StoreID: r.StoreID, Hash: sha256.Sum256([]byte(token))}, nil
}

// Separate bounded keys for allocation by IP and create/retry traffic by
// authenticated scope. No raw scope credential is stored in a limiter key.
func (s *Store) AllowCartRequest(ctx context.Context, kind, identity string) (bool, error) {
	limit := 0
	switch kind {
	case "payment-status-possession", "confirmation-possession", "payment-options-possession":
		limit = 90
	case "scope-ip":
		limit = 8
	case "contact-possession", "shipping-possession", "checkout-possession", "completion-possession":
		limit = 32
	case "create-scope":
		limit = 32
	case "item-possession":
		limit = 128
	}
	if limit == 0 || len(identity) == 0 || len(identity) > 1024 {
		return false, ErrInvalid
	}
	n, err := s.client.Eval(ctx, attemptScript, []string{s.key(cartRealm, kind, identity)}, limit, LoginWindow.Milliseconds()).Int64()
	if err != nil {
		return false, ErrUnavailable
	}
	return n == 1, nil
}
