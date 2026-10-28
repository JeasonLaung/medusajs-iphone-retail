package sessionstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cartRealm Realm = "cart"
	cartKind        = "possession"
	CartTTL         = 12 * time.Hour
)

// CartReference is a capability's binding, not a customer principal. The cart
// coordinator must compare all three fields with its current durable runtime
// ownership record inside the business transaction. Historical customer IDs,
// emails and cart IDs alone cannot create this authority.
type CartReference struct {
	CartID     string `json:"cart_id"`
	StoreID    string `json:"store_id"`
	Generation string `json:"generation"`
}

type cartRecord struct {
	Format    int           `json:"format"`
	Realm     Realm         `json:"realm"`
	Reference CartReference `json:"reference"`
	CSRFHash  string        `json:"csrf_hash"`
	Expires   int64         `json:"expires"`
}

var cartIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$`)

func cartReferenceOK(r CartReference) bool {
	return cartIDPattern.MatchString(r.CartID) && cartIDPattern.MatchString(r.StoreID) && tokenOK(r.Generation)
}

func cartData(ref CartReference, csrf string, expires int64) ([]byte, error) {
	return json.Marshal(cartRecord{Format: 1, Realm: cartRealm, Reference: ref, CSRFHash: digest(csrf), Expires: expires})
}

// IssueCart accepts only a server-created binding. Callers must first commit a
// new cart and its runtime generation, or explicitly authorize a reissue. This
// component has no HTTP route, cannot mint from a browser-supplied cart ID and
// cannot authorize SQL work by itself. No old account/session is consulted.
func (s *Store) IssueCart(ctx context.Context, ref CartReference) (Issued, error) {
	if !cartReferenceOK(ref) {
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
	expires := s.now().Add(CartTTL).Unix()
	data, err := cartData(ref, csrf, expires)
	if err != nil {
		return Issued{}, ErrUnavailable
	}
	ok, err := s.client.SetNX(ctx, s.key(cartRealm, cartKind, token), data, CartTTL).Result()
	if err != nil || !ok {
		return Issued{}, ErrUnavailable
	}
	return Issued{Token: token, CSRF: csrf, ExpiresAt: time.Unix(expires, 0).UTC()}, nil
}

func (s *Store) loadCart(ctx context.Context, token string) (cartRecord, []byte, error) {
	if !tokenOK(token) {
		return cartRecord{}, nil, ErrMissing
	}
	data, err := s.client.Get(ctx, s.key(cartRealm, cartKind, token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return cartRecord{}, nil, ErrMissing
	}
	if err != nil {
		return cartRecord{}, nil, ErrUnavailable
	}
	var r cartRecord
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if len(data) > 2048 || d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Format != 1 || r.Realm != cartRealm || !cartReferenceOK(r.Reference) || !tokenOK(r.CSRFHash) || r.Expires <= s.now().Unix() {
		return cartRecord{}, nil, ErrMissing
	}
	return r, data, nil
}

// ResolveCart returns a reference only; the coordinator must still verify the
// configured store, current generation, cart lifecycle and customer transfer.
// Reads never extend either the absolute lifetime or the Redis TTL.
func (s *Store) ResolveCart(ctx context.Context, token string) (CartReference, error) {
	r, _, err := s.loadCart(ctx, token)
	if err != nil {
		return CartReference{}, err
	}
	return r.Reference, nil
}

// AuthorizeCartWrite resolves and checks CSRF against the same Redis record.
// It grants no inventory, pricing, payment or SQL commit authority.
func (s *Store) AuthorizeCartWrite(ctx context.Context, token, csrf string) (CartReference, error) {
	r, _, err := s.loadCart(ctx, token)
	if err != nil {
		return CartReference{}, err
	}
	if !csrfMatches(r.CSRFHash, csrf) {
		return CartReference{}, ErrCSRF
	}
	return r.Reference, nil
}

// SET precedes DEL: a failed SET must leave the original capability intact.
// Comparing the complete old record prevents a concurrent revoke/rotation from
// being undone. New tokens retain the old absolute expiry and remaining TTL.
const rotateCartScript = `if redis.call('GET',KEYS[1]) ~= ARGV[1] then return 0 end
local ttl=redis.call('PTTL',KEYS[1])
if ttl<=0 then return 0 end
if redis.call('EXISTS',KEYS[2]) ~= 0 then return -1 end
redis.call('SET',KEYS[2],ARGV[2],'PX',ttl,'NX')
redis.call('DEL',KEYS[1])
return 1`

// RotateCart replaces only the token/CSRF pair, preserving the binding. It is
// not customer transfer: durable generation rotation and business ownership
// changes belong to the MySQL coordinator. Exactly one concurrent caller wins.
func (s *Store) RotateCart(ctx context.Context, token, csrf string) (Issued, error) {
	r, previous, err := s.loadCart(ctx, token)
	if err != nil {
		return Issued{}, err
	}
	if !csrfMatches(r.CSRFHash, csrf) {
		return Issued{}, ErrCSRF
	}
	next, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	nextCSRF, err := randomToken()
	if err != nil {
		return Issued{}, err
	}
	data, err := cartData(r.Reference, nextCSRF, r.Expires)
	if err != nil {
		return Issued{}, ErrUnavailable
	}
	n, err := s.client.Eval(ctx, rotateCartScript,
		[]string{s.key(cartRealm, cartKind, token), s.key(cartRealm, cartKind, next)}, previous, data).Int64()
	if err != nil || n == -1 {
		return Issued{}, ErrUnavailable
	}
	if n != 1 {
		return Issued{}, ErrMissing
	}
	return Issued{Token: next, CSRF: nextCSRF, ExpiresAt: time.Unix(r.Expires, 0).UTC()}, nil
}

func (s *Store) RevokeCart(ctx context.Context, token string) error {
	if !tokenOK(token) {
		return ErrInvalid
	}
	if s.client.Del(ctx, s.key(cartRealm, cartKind, token)).Err() != nil {
		return ErrUnavailable
	}
	return nil
}
