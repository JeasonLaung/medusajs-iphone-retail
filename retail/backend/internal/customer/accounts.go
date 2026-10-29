// Package customer owns new customer identities, independently of administrator
// accounts and historical business subjects. It never grants ownership by email.
package customer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/sessionstore"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalid      = errors.New("invalid customer request")
	ErrUnavailable  = errors.New("customer service unavailable")
	ErrExists       = errors.New("customer registration unavailable for this email")
	ErrCredentials  = errors.New("email or password is incorrect")
	ErrUnverified   = errors.New("email verification required")
	ErrVerification = errors.New("verification link is invalid or expired")
	ErrUnauthorized = errors.New("customer sign in required")
	ErrRateLimited  = errors.New("wait a minute before requesting another verification email")
)

const VerificationTTL = 15 * time.Minute

// Account is the public projection. Passwords, revocation generations and
// verification capabilities cannot be serialized through this type.
type Account struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	Phone      string     `json:"phone"`
	VerifiedAt *time.Time `json:"verified_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type record struct {
	Account      `gorm:"embedded"`
	PasswordHash string
	Enabled      bool
	AuthVersion  uint64
}

func (record) TableName() string { return "customer_accounts" }

type verification struct {
	ID, CustomerID string
	TokenHash      []byte
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	CreatedAt      time.Time
}

func (verification) TableName() string { return "customer_verifications" }

type mailJob struct {
	ID, CustomerID       string
	Payload              []byte
	Status               string
	CreatedAt, ExpiresAt time.Time
}

func (mailJob) TableName() string { return "customer_verification_mail" }

type Sessions interface {
	Issue(context.Context, sessionstore.Realm, sessionstore.Principal) (sessionstore.Issued, error)
	Resolve(context.Context, sessionstore.Realm, string) (sessionstore.Principal, error)
	Revoke(context.Context, sessionstore.Realm, string) error
}

type Store struct {
	db       *gorm.DB
	ring     *protected.Ring
	sessions Sessions
	origin   string
}

func New(db *gorm.DB, ring *protected.Ring, sessions Sessions, origin string) (*Store, error) {
	u, err := url.Parse(origin)
	if db == nil || ring == nil || sessions == nil || err != nil || u == nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin {
		return nil, ErrUnavailable
	}
	if u.Scheme != "https" && (u.Scheme != "http" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return nil, ErrUnavailable
	}
	return &Store{db: db, ring: ring, sessions: sessions, origin: origin}, nil
}

type Registration struct {
	Email, Password, FirstName, LastName, Phone string
	Country, Locale                             string
}

func cleanText(v string, max int, required bool) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max || (required && strings.TrimSpace(v) == "") {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func normalizedEmail(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	parsed, err := mail.ParseAddress(v)
	return v, err == nil && parsed.Address == v && parsed.Name == "" && len(v) <= 254 && cleanText(v, 254, true)
}
func passwordOK(p string) bool { return utf8.ValidString(p) && len(p) > 0 && len(p) <= 72 }
func localeOK(country, locale string) bool {
	return len(country) == 2 && country[0] >= 'a' && country[0] <= 'z' && country[1] >= 'a' && country[1] <= 'z' && (locale == "en" || locale == "zh")
}
func (s *Store) ready() bool {
	return s != nil && s.db != nil && s.ring != nil && s.sessions != nil && s.origin != ""
}
func conflict(err error) bool {
	var e *driver.MySQLError
	return errors.As(err, &e) && e.Number == 1062
}

// Register commits the pending identity and encrypted mail intent together. It
// does not return a verification token or claim that an email has been sent.
// The HTTP boundary must supply origin/CSRF checks and rate limiting.
func (s *Store) Register(ctx context.Context, input Registration) (Account, error) {
	email, ok := normalizedEmail(input.Email)
	if !ok || !passwordOK(input.Password) || !cleanText(input.FirstName, 160, true) || !cleanText(input.LastName, 160, true) || !cleanText(input.Phone, 64, false) || !localeOK(input.Country, input.Locale) {
		return Account{}, ErrInvalid
	}
	if !s.ready() {
		return Account{}, ErrUnavailable
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	r := record{Account: Account{ID: "cust_" + uuid.NewString(), Email: email, FirstName: input.FirstName, LastName: input.LastName, Phone: input.Phone, CreatedAt: now, UpdatedAt: now}, PasswordHash: string(hash), Enabled: true, AuthVersion: 1}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&r).Error; err != nil {
			if conflict(err) {
				return ErrExists
			}
			return ErrUnavailable
		}
		return s.queue(tx, r, input.Country, input.Locale, now)
	})
	if err != nil {
		if errors.Is(err, ErrExists) {
			return Account{}, ErrExists
		}
		return Account{}, ErrUnavailable
	}
	return r.Account, nil
}

func mailContext(id string) protected.Context {
	return protected.Context{Purpose: "customer-verification", Table: "customer_verification_mail", Column: "payload", Row: id}
}
func (s *Store) queue(tx *gorm.DB, r record, country, locale string, now time.Time) error {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return ErrUnavailable
	}
	raw := hex.EncodeToString(token[:])
	digest := sha256.Sum256([]byte(raw))
	id := "cv_" + uuid.NewString()
	expires := now.Add(VerificationTTL)
	link := s.origin + "/" + country + "-" + locale + "/verify-account?token=" + raw
	data, err := json.Marshal(struct{ Email, URL, Locale string }{r.Email, link, locale})
	if err != nil {
		return ErrUnavailable
	}
	sealed, err := s.ring.Seal(mailContext(id), data)
	if err != nil {
		return ErrUnavailable
	}
	if err := tx.Where("customer_id=?", r.ID).Delete(&verification{}).Error; err != nil {
		return ErrUnavailable
	}
	if err := tx.Model(&mailJob{}).Where("customer_id=? AND status='pending'", r.ID).Update("status", "cancelled").Error; err != nil {
		return ErrUnavailable
	}
	if err := tx.Create(&verification{ID: id, CustomerID: r.ID, TokenHash: digest[:], ExpiresAt: expires, CreatedAt: now}).Error; err != nil {
		return ErrUnavailable
	}
	return tx.Create(&mailJob{ID: id, CustomerID: r.ID, Payload: sealed, Status: "pending", CreatedAt: now, ExpiresAt: expires}).Error
}

const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *Store) credentials(ctx context.Context, email, password string) (record, error) {
	email, ok := normalizedEmail(email)
	if !ok || !passwordOK(password) {
		return record{}, ErrCredentials
	}
	if !s.ready() {
		return record{}, ErrUnavailable
	}
	var r record
	err := s.db.WithContext(ctx).Where("email=?", email).Take(&r).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return record{}, ErrUnavailable
	}
	encoded := r.PasswordHash
	if err != nil {
		encoded = dummyHash
	}
	valid := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil
	if err != nil || !valid || !r.Enabled || r.AuthVersion == 0 {
		return record{}, ErrCredentials
	}
	return r, nil
}
func version(r record) string {
	value, _ := json.Marshal([]any{r.ID, r.PasswordHash, r.Enabled, r.AuthVersion, r.VerifiedAt})
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func same(a, b record) bool {
	return subtle.ConstantTimeCompare([]byte(version(a)), []byte(version(b))) == 1
}

// Resend requires the pending account's password. A repeated registration never
// overwrites another pending account or issues a capability for it.
func (s *Store) Resend(ctx context.Context, email, password, country, locale string) error {
	if !localeOK(country, locale) {
		return ErrInvalid
	}
	r, err := s.credentials(ctx, email, password)
	if err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.ID).Take(&current).Error; err != nil {
			return ErrUnavailable
		}
		if !same(r, current) {
			return ErrCredentials
		}
		if current.VerifiedAt != nil {
			return ErrVerification
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		var previous verification
		if err := tx.Where("customer_id=?", current.ID).Take(&previous).Error; err != nil {
			return ErrUnavailable
		}
		if now.Sub(previous.CreatedAt) < time.Minute {
			return ErrRateLimited
		}
		return s.queue(tx, current, country, locale, now)
	})
	if err != nil && !errors.Is(err, ErrCredentials) && !errors.Is(err, ErrVerification) && !errors.Is(err, ErrRateLimited) {
		return ErrUnavailable
	}
	return err
}

func tokenOK(raw string) bool {
	if len(raw) != 64 {
		return false
	}
	value, err := hex.DecodeString(raw)
	return err == nil && hex.EncodeToString(value) == raw
}

// Verify consumes a link once. It deliberately does not sign in the browser
// opening the link; verification may happen on a different device.
func (s *Store) Verify(ctx context.Context, raw string) error {
	if !tokenOK(raw) {
		return ErrVerification
	}
	if !s.ready() {
		return ErrUnavailable
	}
	digest := sha256.Sum256([]byte(raw))
	var hint verification
	if err := s.db.WithContext(ctx).Where("token_hash=?", digest[:]).Take(&hint).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVerification
		}
		return ErrUnavailable
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Always lock the account before the challenge, also in Resend/Login.
		var r record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", hint.CustomerID).Take(&r).Error; err != nil {
			return ErrUnavailable
		}
		var v verification
		if err := tx.Where("customer_id=? AND token_hash=?", r.ID, digest[:]).Take(&v).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrVerification
			}
			return ErrUnavailable
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if !r.Enabled || r.AuthVersion == 0 || r.VerifiedAt != nil || v.ConsumedAt != nil || !v.ExpiresAt.After(now) {
			return ErrVerification
		}
		result := tx.Model(&verification{}).Where("id=? AND consumed_at IS NULL AND expires_at>?", v.ID, now).Update("consumed_at", now)
		if result.Error != nil {
			return ErrUnavailable
		}
		if result.RowsAffected != 1 {
			return ErrVerification
		}
		if err := tx.Model(&record{}).Where("id=?", r.ID).Updates(map[string]any{"verified_at": now, "updated_at": now, "auth_version": gorm.Expr("auth_version+1")}).Error; err != nil {
			return ErrUnavailable
		}
		return tx.Model(&mailJob{}).Where("customer_id=? AND status='pending'", r.ID).Update("status", "cancelled").Error
	})
	if err != nil && !errors.Is(err, ErrVerification) {
		return ErrUnavailable
	}
	return err
}

type Login struct {
	Customer Account             `json:"customer"`
	Session  sessionstore.Issued `json:"-"`
}

func (s *Store) Login(ctx context.Context, email, password string) (Login, error) {
	r, err := s.credentials(ctx, email, password)
	if err != nil {
		return Login{}, err
	}
	var result Login
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.ID).Take(&current).Error; err != nil {
			return ErrUnavailable
		}
		if !same(r, current) {
			return ErrCredentials
		}
		if current.VerifiedAt == nil {
			return ErrUnverified
		}
		issued, err := s.sessions.Issue(ctx, sessionstore.Customer, sessionstore.Principal{UserID: current.ID, Version: version(current)})
		if err != nil {
			return ErrUnavailable
		}
		result = Login{Customer: current.Account, Session: issued}
		return nil
	})
	if err != nil {
		if result.Session.Token != "" {
			_ = s.sessions.Revoke(ctx, sessionstore.Customer, result.Session.Token)
		}
		if errors.Is(err, ErrCredentials) || errors.Is(err, ErrUnverified) {
			return Login{}, err
		}
		return Login{}, ErrUnavailable
	}
	return result, nil
}

// Current rechecks durable status/revocation on every resolution. Redis alone
// never grants an account or historical business access.
func (s *Store) Current(ctx context.Context, token string) (Account, error) {
	if !tokenOK(token) {
		return Account{}, ErrUnauthorized
	}
	if !s.ready() {
		return Account{}, ErrUnavailable
	}
	p, err := s.sessions.Resolve(ctx, sessionstore.Customer, token)
	if err != nil {
		if errors.Is(err, sessionstore.ErrUnavailable) {
			return Account{}, ErrUnavailable
		}
		return Account{}, ErrUnauthorized
	}
	var r record
	if err := s.db.WithContext(ctx).Where("id=?", p.UserID).Take(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, ErrUnauthorized
		}
		return Account{}, ErrUnavailable
	}
	if !r.Enabled || r.VerifiedAt == nil || r.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(p.Version), []byte(version(r))) != 1 {
		return Account{}, ErrUnauthorized
	}
	return r.Account, nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	if !tokenOK(token) {
		return ErrUnauthorized
	}
	if !s.ready() {
		return ErrUnavailable
	}
	if err := s.sessions.Revoke(ctx, sessionstore.Customer, token); err != nil {
		return ErrUnavailable
	}
	return nil
}
