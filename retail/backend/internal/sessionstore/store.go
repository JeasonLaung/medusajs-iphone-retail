// Package sessionstore owns ephemeral login capabilities in an explicit Redis
// namespace. It never owns durable business data, roles or authorization.
package sessionstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalid     = errors.New("invalid session request")
	ErrMissing     = errors.New("session absent or expired")
	ErrUnavailable = errors.New("session dependency unavailable")
	ErrCSRF        = errors.New("session CSRF mismatch")
)

type Realm string

const (
	Admin         Realm = "admin"
	Customer      Realm = "customer"
	SessionTTL          = 12 * time.Hour
	PreloginTTL         = 10 * time.Minute
	LoginWindow         = 15 * time.Minute
	LoginAttempts       = 8
)

// Raw capabilities never appear in keys, records or errors. Only returned to
// the caller for HttpOnly cookie / explicit CSRF response delivery.
type Issued struct {
	Token, CSRF string
	ExpiresAt   time.Time
}
type Principal struct{ UserID, Version string }

func (s *Store) Ping(ctx context.Context) error {
	if s.client.Ping(ctx).Err() != nil {
		return ErrUnavailable
	}
	return nil
}

type record struct {
	Format   int    `json:"format"`
	Realm    Realm  `json:"realm"`
	UserID   string `json:"user_id"`
	Version  string `json:"version"`
	CSRFHash string `json:"csrf_hash"`
	Expires  int64  `json:"expires"`
}

type Store struct {
	client *redis.Client
	prefix string
	now    func() time.Time
}

var prefixPattern = regexp.MustCompile(`^eurorefurb:[a-z][a-z0-9_-]{2,39}:v1:$`)

func New(client *redis.Client, prefix string) (*Store, error) {
	if client == nil || !prefixPattern.MatchString(prefix) {
		return nil, ErrInvalid
	}
	return &Store{client: client, prefix: prefix, now: time.Now}, nil
}
func realmOK(r Realm) bool   { return r == Admin || r == Customer }
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func tokenOK(s string) bool {
	if len(s) != 64 {
		return false
	}
	v, err := hex.DecodeString(s)
	return err == nil && hex.EncodeToString(v) == s
}
func randomToken() (string, error) {
	var v [32]byte
	if _, err := rand.Read(v[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(v[:]), nil
}
func principalOK(p Principal) bool {
	return len(p.UserID) > 0 && len(p.UserID) <= 191 && tokenOK(p.Version)
}
func (s *Store) key(realm Realm, kind, token string) string {
	return s.prefix + string(realm) + ":" + kind + ":" + digest(token)
}

// Issue never refreshes an existing capability or accepts a caller-chosen
// token. Redis SET NX and TTL happen atomically. A failed/uncertain SET yields
// no token; the unreachable orphan expires without granting login.
func (s *Store) Issue(ctx context.Context, realm Realm, p Principal) (Issued, error) {
	if !realmOK(realm) || !principalOK(p) {
		return Issued{}, ErrInvalid
	}
	return s.issue(ctx, realm, "session", p, SessionTTL)
}
func (s *Store) Prelogin(ctx context.Context, realm Realm) (Issued, error) {
	if !realmOK(realm) {
		return Issued{}, ErrInvalid
	}
	return s.issue(ctx, realm, "prelogin", Principal{}, PreloginTTL)
}
func (s *Store) issue(ctx context.Context, realm Realm, kind string, p Principal, ttl time.Duration) (Issued, error) {
	token, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	expires := s.now().Add(ttl).Unix()
	data, err := json.Marshal(record{Format: 1, Realm: realm, UserID: p.UserID, Version: p.Version, CSRFHash: digest(csrf), Expires: expires})
	if err != nil {
		return Issued{}, ErrUnavailable
	}
	ok, err := s.client.SetNX(ctx, s.key(realm, kind, token), data, ttl).Result()
	if err != nil || !ok {
		return Issued{}, ErrUnavailable
	}
	return Issued{Token: token, CSRF: csrf, ExpiresAt: time.Unix(expires, 0).UTC()}, nil
}

func (s *Store) load(ctx context.Context, realm Realm, kind, token string) (record, error) {
	if !realmOK(realm) || !tokenOK(token) {
		return record{}, ErrMissing
	}
	data, err := s.client.Get(ctx, s.key(realm, kind, token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return record{}, ErrMissing
	}
	if err != nil {
		return record{}, ErrUnavailable
	}
	var r record
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if len(data) > 2048 || d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Format != 1 || r.Realm != realm || !tokenOK(r.CSRFHash) || r.Expires <= s.now().Unix() {
		return record{}, ErrMissing
	}
	if kind == "session" && !principalOK(Principal{r.UserID, r.Version}) {
		return record{}, ErrMissing
	}
	if kind == "prelogin" && (r.UserID != "" || r.Version != "") {
		return record{}, ErrMissing
	}
	return r, nil
}

// Resolve returns a reference only. Caller MUST fetch current MySQL account,
// enabled state and version; this record contains no role/grants/cache bypass.
func (s *Store) Resolve(ctx context.Context, realm Realm, token string) (Principal, error) {
	r, err := s.load(ctx, realm, "session", token)
	if err != nil {
		return Principal{}, err
	}
	return Principal{UserID: r.UserID, Version: r.Version}, nil
}
func csrfMatches(hash, csrf string) bool {
	return tokenOK(csrf) && subtle.ConstantTimeCompare([]byte(hash), []byte(digest(csrf))) == 1
}
func (s *Store) VerifyCSRF(ctx context.Context, realm Realm, token, csrf string) error {
	r, err := s.load(ctx, realm, "session", token)
	if err != nil {
		return err
	}
	if !csrfMatches(r.CSRFHash, csrf) {
		return ErrCSRF
	}
	return nil
}

// Compare-and-delete makes each pre-login challenge single use, even across
// processes. Invalid CSRF does not consume somebody else's challenge.
const consumeScript = `if redis.call('GET',KEYS[1]) == ARGV[1] then
 return redis.call('DEL',KEYS[1])
end
return 0`

func (s *Store) ConsumePrelogin(ctx context.Context, realm Realm, token, csrf string) error {
	r, err := s.load(ctx, realm, "prelogin", token)
	if err != nil {
		return err
	}
	if !csrfMatches(r.CSRFHash, csrf) {
		return ErrCSRF
	}
	data, _ := json.Marshal(r)
	n, err := s.client.Eval(ctx, consumeScript, []string{s.key(realm, "prelogin", token)}, data).Int64()
	if err != nil {
		return ErrUnavailable
	}
	if n != 1 {
		return ErrMissing
	}
	return nil
}
func (s *Store) Revoke(ctx context.Context, realm Realm, token string) error {
	if !realmOK(realm) || !tokenOK(token) {
		return ErrInvalid
	}
	if err := s.client.Del(ctx, s.key(realm, "session", token)).Err(); err != nil {
		return ErrUnavailable
	}
	return nil
}

// Atomic fixed window; denied requests do not extend the window or increase
// its counter. No Redis retry: an uncertain INCR must not silently repeat.
const attemptScript = `local n=tonumber(redis.call('GET',KEYS[1]) or '0')
if n>=tonumber(ARGV[1]) then return 0 end
n=redis.call('INCR',KEYS[1])
if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[2]) end
if redis.call('PTTL',KEYS[1])<=0 then return 0 end
return 1`

func (s *Store) AllowLogin(ctx context.Context, realm Realm, identity string) (bool, error) {
	if !realmOK(realm) || len(identity) == 0 || len(identity) > 1024 {
		return false, ErrInvalid
	}
	n, err := s.client.Eval(ctx, attemptScript, []string{s.key(realm, "login-attempt", identity)}, LoginAttempts, LoginWindow.Milliseconds()).Int64()
	if err != nil {
		return false, ErrUnavailable
	}
	return n == 1, nil
}
