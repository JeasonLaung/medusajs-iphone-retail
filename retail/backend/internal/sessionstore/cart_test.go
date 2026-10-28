package sessionstore

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestCartReferenceValidation(t *testing.T) {
	ref := CartReference{CartID: "cart_new-1", StoreID: "store_new-1", Generation: strings.Repeat("a", 64)}
	if !cartReferenceOK(ref) {
		t.Fatal("server binding rejected")
	}
	for _, bad := range []CartReference{
		{}, {ref.CartID, ref.StoreID, ""}, {ref.CartID, ref.StoreID, strings.Repeat("A", 64)},
		{"cart/id", ref.StoreID, ref.Generation}, {" cart", ref.StoreID, ref.Generation},
		{ref.CartID, "store.example", ref.Generation}, {strings.Repeat("a", 192), ref.StoreID, ref.Generation},
	} {
		if cartReferenceOK(bad) {
			t.Fatal("invalid cart reference accepted")
		}
		if issued, err := (&Store{}).IssueCart(t.Context(), bad); err != ErrInvalid || issued != (Issued{}) {
			t.Fatal("invalid binding reached dependency or returned capability")
		}
	}
	// Arbitrary IDs, old cookies and login-like strings are not capabilities;
	// invalid values must fail before even attempting a Redis connection.
	for _, token := range []string{"", ref.CartID, "Bearer " + ref.Generation, strings.Repeat("A", 64), strings.Repeat("a", 65)} {
		if r, err := (&Store{}).ResolveCart(t.Context(), token); err != ErrMissing || r != (CartReference{}) {
			t.Fatal("invalid token resolved")
		}
	}
}

// Only the disposable Redis owned by test-redis-sessions.py is used. No source
// snapshot, customer account, application Redis or HTTP route is involved.
func TestOwnedRedisCartPossession(t *testing.T) {
	if os.Getenv("SESSION_REDIS_REHEARSAL") != "1" {
		t.Skip("owned disposable Redis required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: "/tmp/redis.sock", MaxRetries: -1, Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true})
	defer client.Close()
	s, err := New(client, "eurorefurb:cart-fixture:v1:")
	if err != nil {
		t.Fatal("owned cart store unavailable")
	}
	other, _ := New(client, "eurorefurb:other-cart:v1:")
	ref := CartReference{"cart_artificial", "store_artificial", strings.Repeat("c", 64)}
	issue := func() Issued {
		i, err := s.IssueCart(ctx, ref)
		if err != nil || !tokenOK(i.Token) || !tokenOK(i.CSRF) || i.Token == i.CSRF {
			t.Fatal("cart capability not issued")
		}
		return i
	}
	i := issue()
	if got, err := s.ResolveCart(ctx, i.Token); err != nil || got != ref {
		t.Fatal("cart reference roundtrip failed")
	}
	if got, err := s.AuthorizeCartWrite(ctx, i.Token, i.CSRF); err != nil || got != ref {
		t.Fatal("cart CSRF roundtrip failed")
	}
	key := s.key(cartRealm, cartKind, i.Token)
	raw, err := client.Get(ctx, key).Result()
	if err != nil || strings.Contains(key, i.Token) || strings.Contains(raw, i.Token) || strings.Contains(raw, i.CSRF) {
		t.Fatal("raw cart credentials persisted")
	}
	var stored cartRecord
	if json.Unmarshal([]byte(raw), &stored) != nil || stored.CSRFHash != digest(i.CSRF) {
		t.Fatal("CSRF not hashed")
	}
	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil || ttl <= CartTTL-time.Minute || ttl > CartTTL || !i.ExpiresAt.Equal(time.Unix(stored.Expires, 0).UTC()) {
		t.Fatal("cart expiration not bounded")
	}
	for _, bad := range []string{"", "wrong", strings.Repeat("d", 64), i.Token} {
		if r, err := s.AuthorizeCartWrite(ctx, i.Token, bad); err != ErrCSRF || r != (CartReference{}) {
			t.Fatal("invalid CSRF exposed binding")
		}
		if r, err := s.RotateCart(ctx, i.Token, bad); err != ErrCSRF || r != (Issued{}) {
			t.Fatal("invalid CSRF rotated capability")
		}
	}
	if _, err := s.ResolveCart(ctx, i.Token); err != nil {
		t.Fatal("invalid CSRF consumed legitimate capability")
	}
	if _, err := other.ResolveCart(ctx, i.Token); err != ErrMissing {
		t.Fatal("cart namespace crossed")
	}
	for _, realm := range []Realm{Admin, Customer, cartRealm} {
		if _, err := s.Resolve(ctx, realm, i.Token); err != ErrMissing {
			t.Fatal("cart became login")
		}
		if _, err := s.Prelogin(ctx, cartRealm); err != ErrInvalid {
			t.Fatal("cart realm became prelogin")
		}
	}
	for _, realm := range []Realm{Admin, Customer} {
		p := Principal{"artificial-user", strings.Repeat("e", 64)}
		login, err := s.Issue(ctx, realm, p)
		if err != nil {
			t.Fatal("login isolation fixture unavailable")
		}
		pre, err := s.Prelogin(ctx, realm)
		if err != nil {
			t.Fatal("prelogin isolation fixture unavailable")
		}
		for _, token := range []string{login.Token, pre.Token} {
			if r, err := s.ResolveCart(ctx, token); err != ErrMissing || r != (CartReference{}) {
				t.Fatal("login became cart possession")
			}
		}
	}
	// Artificial TTL shortening proves token rotation cannot revive/extend a
	// key whose remaining Redis lifetime is shorter than its absolute expiry.
	if client.PExpire(ctx, key, 10*time.Second).Err() != nil {
		t.Fatal("owned cart TTL fixture unavailable")
	}
	before, _ := client.PTTL(ctx, key).Result()
	const contenders = 16
	results := make(chan Issued, contenders)
	errors := make(chan error, contenders)
	var wg sync.WaitGroup
	for j := 0; j < contenders; j++ {
		wg.Go(func() {
			i, err := s.RotateCart(ctx, i.Token, i.CSRF)
			if err == nil {
				results <- i
			} else {
				errors <- err
			}
		})
	}
	wg.Wait()
	close(results)
	close(errors)
	if len(results) != 1 || len(errors) != contenders-1 {
		t.Fatal("concurrent rotation did not have one winner")
	}
	for err := range errors {
		if err != ErrMissing {
			t.Fatal("rotation race failed unexpectedly")
		}
	}
	next := <-results
	after, err := client.PTTL(ctx, s.key(cartRealm, cartKind, next.Token)).Result()
	if err != nil || after <= 0 || after > before || !next.ExpiresAt.Equal(i.ExpiresAt) || next.Token == i.Token || next.CSRF == i.CSRF {
		t.Fatal("rotation extended expiry or reused credential")
	}
	if _, err := s.ResolveCart(ctx, i.Token); err != ErrMissing {
		t.Fatal("old cart token replay survived")
	}
	if _, err := s.AuthorizeCartWrite(ctx, next.Token, i.CSRF); err != ErrCSRF {
		t.Fatal("old CSRF survived rotation")
	}
	if r, err := s.AuthorizeCartWrite(ctx, next.Token, next.CSRF); err != nil || r != ref {
		t.Fatal("rotation changed binding")
	}
	if s.RevokeCart(ctx, next.Token) != nil || s.RevokeCart(ctx, next.Token) != nil {
		t.Fatal("cart revoke is not idempotent")
	}
	if _, err := s.ResolveCart(ctx, next.Token); err != ErrMissing {
		t.Fatal("revoked possession survived")
	}
	// A loaded record cannot resurrect a capability revoked before the CAS.
	revoked := issue()
	_, observed, err := s.loadCart(ctx, revoked.Token)
	if err != nil || s.RevokeCart(ctx, revoked.Token) != nil {
		t.Fatal("revocation CAS fixture unavailable")
	}
	candidate, _ := randomToken()
	n, err := client.Eval(ctx, rotateCartScript, []string{s.key(cartRealm, cartKind, revoked.Token), s.key(cartRealm, cartKind, candidate)}, observed, observed).Int64()
	if err != nil || n != 0 || client.Exists(ctx, s.key(cartRealm, cartKind, candidate)).Val() != 0 {
		t.Fatal("rotation resurrected revoked token")
	}
	expiring := issue()
	s.now = func() time.Time { return time.Now().Add(CartTTL + time.Minute) }
	if _, err := s.ResolveCart(ctx, expiring.Token); err != ErrMissing {
		t.Fatal("absolute cart expiry ignored")
	}
	s.now = time.Now
	expiryKey := s.key(cartRealm, cartKind, expiring.Token)
	if client.PExpire(ctx, expiryKey, time.Millisecond).Err() != nil {
		t.Fatal("owned cart expiry fixture unavailable")
	}
	deadline := time.Now().Add(time.Second)
	for {
		_, err := s.ResolveCart(ctx, expiring.Token)
		if err == ErrMissing {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatal("Redis cart expiry ignored")
		}
		time.Sleep(5 * time.Millisecond)
	}
	corrupt := issue()
	for _, bad := range []string{
		`{}`, `{"format":99}`, raw + `{}`, strings.Repeat("x", 4096),
		strings.Replace(raw, `"realm":"cart"`, `"realm":"admin"`, 1),
		strings.Replace(raw, `"format":1`, `"extra":true,"format":1`, 1),
		strings.Replace(raw, ref.Generation, "invalid-generation", 1),
	} {
		if client.Set(ctx, s.key(cartRealm, cartKind, corrupt.Token), bad, time.Minute).Err() != nil {
			t.Fatal("owned corrupt cart fixture unavailable")
		}
		if r, err := s.ResolveCart(ctx, corrupt.Token); err != ErrMissing || r != (CartReference{}) {
			t.Fatal("corrupt record granted possession")
		}
	}
	evicted := issue()
	if client.Del(ctx, s.key(cartRealm, cartKind, evicted.Token)).Err() != nil {
		t.Fatal("owned eviction fixture unavailable")
	}
	if _, err := s.ResolveCart(ctx, evicted.Token); err != ErrMissing {
		t.Fatal("cache loss restored cart authority")
	}
	outage := issue()
	_ = client.Close()
	if r, err := s.ResolveCart(ctx, outage.Token); err != ErrUnavailable || r != (CartReference{}) {
		t.Fatal("outage resolved cart")
	}
	if r, err := s.AuthorizeCartWrite(ctx, outage.Token, outage.CSRF); err != ErrUnavailable || r != (CartReference{}) {
		t.Fatal("outage authorized cart write")
	}
	if r, err := s.RotateCart(ctx, outage.Token, outage.CSRF); err != ErrUnavailable || r != (Issued{}) {
		t.Fatal("outage rotated cart")
	}
	if r, err := s.IssueCart(ctx, ref); err != ErrUnavailable || r != (Issued{}) {
		t.Fatal("outage issued cart")
	}
	if err := s.RevokeCart(ctx, outage.Token); err != ErrUnavailable {
		t.Fatal("outage reported successful revocation")
	}
	t.Log(`CART_POSSESSION_REHEARSAL {"realm_namespace_isolation":true,"id_is_not_authority":true,"hashed_credentials":true,"csrf_required":true,"rotation_contenders":16,"rotation_winners":1,"rotation_no_ttl_extension":true,"replay_rejected":true,"revoke_cas_rejected":true,"absolute_and_redis_expiry":true,"corrupt_records_rejected":true,"eviction_and_outage_closed":true,"durable_binding_integrated":false,"http_integrated":false}`)
}
