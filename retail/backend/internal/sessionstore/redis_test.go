package sessionstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Runs only inside a disposable, no-network Redis container owned by the
// rehearsal script. No user/application Redis URL is ever consumed here.
func TestOwnedRedisSessions(t *testing.T) {
	if os.Getenv("SESSION_REDIS_REHEARSAL") != "1" {
		t.Skip("owned disposable Redis required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Network: "unix", Addr: "/tmp/redis.sock", MaxRetries: -1, Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true})
	defer client.Close()
	s, err := New(client, "eurorefurb:fixture:v1:")
	if err != nil {
		t.Fatal("fixture store invalid")
	}
	other, _ := New(client, "eurorefurb:other:v1:")
	p := Principal{UserID: "artificial-user", Version: strings.Repeat("a", 64)}
	issued, err := s.Issue(ctx, Admin, p)
	if err != nil {
		t.Fatal("session not issued")
	}
	if issued.Token == issued.CSRF || !tokenOK(issued.Token) || !tokenOK(issued.CSRF) {
		t.Fatal("weak capabilities")
	}
	loaded, err := s.Resolve(ctx, Admin, issued.Token)
	if err != nil || loaded != p {
		t.Fatal("session roundtrip failed")
	}
	for _, r := range []Realm{Customer, Realm("unknown")} {
		if _, err := s.Resolve(ctx, r, issued.Token); err != ErrMissing {
			t.Fatal("realm crossed")
		}
	}
	if _, err := other.Resolve(ctx, Admin, issued.Token); err != ErrMissing {
		t.Fatal("namespace crossed")
	}
	if s.VerifyCSRF(ctx, Admin, issued.Token, issued.CSRF) != nil || s.VerifyCSRF(ctx, Admin, issued.Token, strings.Repeat("b", 64)) != ErrCSRF {
		t.Fatal("CSRF not verified")
	}
	if s.VerifyCSRF(ctx, Customer, issued.Token, issued.CSRF) != ErrMissing {
		t.Fatal("CSRF realm crossed")
	}
	key := s.key(Admin, "session", issued.Token)
	raw, err := client.Get(ctx, key).Result()
	if err != nil || strings.Contains(raw, issued.Token) || strings.Contains(raw, issued.CSRF) || strings.Contains(key, issued.Token) {
		t.Fatal("raw credentials persisted")
	}
	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil || ttl <= SessionTTL-time.Minute || ttl > SessionTTL {
		t.Fatal("session TTL missing")
	}
	if _, err := s.Resolve(ctx, Admin, "A"+issued.Token[1:]); err != ErrMissing {
		t.Fatal("noncanonical token accepted")
	}
	if err := s.Revoke(ctx, Admin, issued.Token); err != nil {
		t.Fatal("revoke failed")
	}
	if _, err := s.Resolve(ctx, Admin, issued.Token); err != ErrMissing {
		t.Fatal("revoked login survived")
	}
	if err := s.Revoke(ctx, Admin, issued.Token); err != nil {
		t.Fatal("repeat revoke failed")
	}

	pre, err := s.Prelogin(ctx, Admin)
	if err != nil {
		t.Fatal("prelogin unavailable")
	}
	if s.ConsumePrelogin(ctx, Admin, pre.Token, strings.Repeat("c", 64)) != ErrCSRF {
		t.Fatal("wrong prelogin CSRF accepted")
	}
	if _, err := s.Resolve(ctx, Admin, pre.Token); err != ErrMissing {
		t.Fatal("prelogin became session")
	}
	var wg sync.WaitGroup
	var won, failed atomic.Int64
	for j := 0; j < 16; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.ConsumePrelogin(ctx, Admin, pre.Token, pre.CSRF)
			if err == nil {
				won.Add(1)
			} else if !errors.Is(err, ErrMissing) {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || failed.Load() != 0 {
		t.Fatal("prelogin consume not atomic")
	}
	won.Store(0)
	failed.Store(0)
	for j := 0; j < 24; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.AllowLogin(ctx, Admin, "artificial-ip:email")
			if err != nil {
				failed.Add(1)
			}
			if ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != LoginAttempts || failed.Load() != 0 {
		t.Fatal("login attempt race allowed excess")
	}
	limiterTTL, err := client.PTTL(ctx, s.key(Admin, "login-attempt", "artificial-ip:email")).Result()
	if err != nil || limiterTTL <= LoginWindow-time.Minute || limiterTTL > LoginWindow {
		t.Fatal("limiter TTL missing")
	}
	if ok, err := s.AllowLogin(ctx, Customer, "artificial-ip:email"); err != nil || !ok {
		t.Fatal("rate realm shared")
	}
	if ok, err := other.AllowLogin(ctx, Admin, "artificial-ip:email"); err != nil || !ok {
		t.Fatal("rate namespace shared")
	}

	expiring, err := s.Issue(ctx, Customer, p)
	if err != nil {
		t.Fatal("expiry fixture unavailable")
	}
	s.now = func() time.Time { return time.Now().Add(SessionTTL + time.Minute) }
	if _, err := s.Resolve(ctx, Customer, expiring.Token); err != ErrMissing {
		t.Fatal("absolute expiration ignored")
	}
	s.now = time.Now
	expiryKey := s.key(Customer, "session", expiring.Token)
	if client.PExpire(ctx, expiryKey, 20*time.Millisecond).Err() != nil {
		t.Fatal("fixture expiry unavailable")
	}
	deadline := time.Now().Add(time.Second)
	for {
		_, err := s.Resolve(ctx, Customer, expiring.Token)
		if err == ErrMissing {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatal("Redis expiration ignored")
		}
		time.Sleep(5 * time.Millisecond)
	}
	malformed, _ := s.Issue(ctx, Admin, p)
	for _, bad := range []string{`{}`, `{"format":99}`, strings.Repeat("x", 4096)} {
		if client.Set(ctx, s.key(Admin, "session", malformed.Token), bad, time.Minute).Err() != nil {
			t.Fatal("malformed fixture unavailable")
		}
		if _, err := s.Resolve(ctx, Admin, malformed.Token); err != ErrMissing {
			t.Fatal("malformed record granted login")
		}
	}
	// Simulate scoped eviction only. Never FLUSH an application database.
	survivor, err := s.Issue(ctx, Admin, p)
	if err != nil {
		t.Fatal("eviction fixture unavailable")
	}
	if client.Del(ctx, s.key(Admin, "session", survivor.Token)).Err() != nil {
		t.Fatal("eviction fixture failed")
	}
	if _, err := s.Resolve(ctx, Admin, survivor.Token); err != ErrMissing {
		t.Fatal("cache loss restored login")
	}
	if _, err := s.Issue(ctx, Admin, Principal{}); err != ErrInvalid {
		t.Fatal("invalid principal accepted")
	}
	if _, err := s.Prelogin(ctx, Realm("warehouse")); err != ErrInvalid {
		t.Fatal("unknown realm issued")
	}
	// Closed connection models a failed dependency: no fallback memory/SQL.
	_ = client.Close()
	if _, err := s.Resolve(ctx, Admin, survivor.Token); err != ErrUnavailable {
		t.Fatal("outage accepted session")
	}
	if _, err := s.Issue(ctx, Admin, p); err != ErrUnavailable {
		t.Fatal("outage issued session")
	}
	if ok, err := s.AllowLogin(ctx, Admin, "artificial-outage"); ok || err != ErrUnavailable {
		t.Fatal("outage bypassed limiter")
	}
	data, _ := json.Marshal(map[string]int{"realms": 2, "concurrent_prelogin_attempts": 16, "prelogin_winners": 1, "concurrent_login_attempts": 24, "allowed_login_attempts": 8, "namespace_isolation": 1, "ttl_expiry": 1, "outage_fail_closed": 1})
	t.Log("SESSION_REDIS_REHEARSAL " + string(data))
}
