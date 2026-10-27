// Package uniquekey implements transactional full-value identity claims.
// It never creates tables or connects to a database. Domain writers and the
// importer must call Apply in their business transaction and roll back on ANY
// error. Source-table writes bypassing Apply are NOT protected by this registry.
package uniquekey

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"regexp"
	"sort"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid unique key change")
	ErrConflict  = errors.New("unique key already claimed")
	ErrStorage   = errors.New("unique key storage failure; roll back transaction")
	scopePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// Change replaces one owner's claim within one constraint scope. Active is the
// source predicate's participation result. Any NULL component means no claim,
// matching ordinary PostgreSQL UNIQUE NULL semantics. Empty strings are values.
// Components must be canonical source values; no trim/lower/float conversion.
type Change struct {
	Scope      string
	Owner      string
	Active     bool
	Components []*string
}

type claim struct {
	scope, owner string
	value        []byte
	digest       [32]byte
}

// encode is an unambiguous versioned UTF-8 tuple, not concatenation or JSON.
func encode(parts []*string) ([]byte, error) {
	if len(parts) == 0 {
		return nil, ErrInvalid
	}
	for _, part := range parts {
		if part == nil {
			return nil, nil
		}
		if !utf8.ValidString(*part) {
			return nil, ErrInvalid
		}
	}
	var out bytes.Buffer
	out.WriteByte(1)
	_ = binary.Write(&out, binary.BigEndian, uint64(len(parts)))
	for _, part := range parts {
		_ = binary.Write(&out, binary.BigEndian, uint64(len(*part)))
		out.WriteString(*part)
	}
	return out.Bytes(), nil
}

// Apply locks owners in byte order, then old/new digest buckets in byte order.
// Locks remain until the caller commits/rolls back. A multi-owner change allows
// atomic swaps. Multiple Apply calls in one transaction can still deadlock with
// other domain locks: callers must retry the WHOLE transaction after rollback,
// never retry just a partial registry operation or a remote side effect.
func Apply(ctx context.Context, tx *sql.Tx, changes []Change) error {
	return apply(ctx, tx, changes, sha256.Sum256)
}

// hash is injectable internally only so tests can force actual collisions.
func apply(ctx context.Context, tx *sql.Tx, changes []Change, hash func([]byte) [32]byte) error {
	return applyCodec(ctx, tx, changes, plainCodec{hash})
}

type valueCodec interface {
	prepare(context.Context, *sql.Tx, []claim) error
	digest(string, []byte) [32]byte
	seal(string, string, []byte) ([]byte, error)
	open(string, string, []byte) ([]byte, error)
}

type plainCodec struct{ hash func([]byte) [32]byte }

func (p plainCodec) prepare(_ context.Context, _ *sql.Tx, claims []claim) error {
	for _, c := range claims {
		if protectedScope(c.scope) {
			return ErrProtected
		}
	}
	return nil
}
func (p plainCodec) digest(_ string, value []byte) [32]byte         { return p.hash(value) }
func (p plainCodec) seal(_, _ string, value []byte) ([]byte, error) { return value, nil }
func (p plainCodec) open(_, _ string, value []byte) ([]byte, error) { return value, nil }

func applyCodec(ctx context.Context, tx *sql.Tx, changes []Change, codec valueCodec) error {
	if tx == nil {
		return ErrInvalid
	}
	claims := make([]claim, 0, len(changes))
	seen := make(map[string]bool)
	for _, change := range changes {
		if !scopePattern.MatchString(change.Scope) || change.Owner == "" || !utf8.ValidString(change.Owner) || utf8.RuneCountInString(change.Owner) > 191 {
			return ErrInvalid
		}
		key := change.Scope + "\x00" + change.Owner
		if seen[key] {
			return ErrInvalid
		}
		seen[key] = true
		c := claim{scope: change.Scope, owner: change.Owner}
		value, err := encode(change.Components)
		if err != nil {
			return err
		}
		if change.Active && value != nil {
			c.value, c.digest = value, codec.digest(c.scope, value)
		}
		claims = append(claims, c)
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].scope != claims[j].scope {
			return claims[i].scope < claims[j].scope
		}
		return claims[i].owner < claims[j].owner
	})
	if err := codec.prepare(ctx, tx, claims); err != nil {
		return err
	}
	type bucket struct {
		scope  string
		digest [32]byte
	}
	buckets := make(map[bucket]bool)
	for _, c := range claims {
		// INSERT ... ON DUPLICATE KEY UPDATE obtains the existing owner lock
		// without ignoring unrelated SQL errors (no INSERT IGNORE).
		if _, err := tx.ExecContext(ctx, "INSERT INTO unique_key_owners(scope,owner) VALUES(?,?) ON DUPLICATE KEY UPDATE owner=unique_key_owners.owner", c.scope, c.owner); err != nil {
			return ErrStorage
		}
		var old []byte
		err := tx.QueryRowContext(ctx, "SELECT current_digest FROM unique_key_owners WHERE scope=? AND owner=? FOR UPDATE", c.scope, c.owner).Scan(&old)
		if err != nil {
			return ErrStorage
		}
		if old != nil {
			if len(old) != 32 {
				return ErrStorage
			}
			var digest [32]byte
			copy(digest[:], old)
			buckets[bucket{c.scope, digest}] = true
		}
		if c.value != nil {
			buckets[bucket{c.scope, c.digest}] = true
		}
	}
	ordered := make([]bucket, 0, len(buckets))
	for b := range buckets {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].scope != ordered[j].scope {
			return ordered[i].scope < ordered[j].scope
		}
		return bytes.Compare(ordered[i].digest[:], ordered[j].digest[:]) < 0
	})
	for _, b := range ordered {
		if _, err := tx.ExecContext(ctx, "INSERT INTO unique_key_buckets(scope,digest) VALUES(?,?) ON DUPLICATE KEY UPDATE digest=unique_key_buckets.digest", b.scope, b.digest[:]); err != nil {
			return ErrStorage
		}
	}
	for _, c := range claims {
		if _, err := tx.ExecContext(ctx, "DELETE FROM unique_key_claims WHERE scope=? AND owner=?", c.scope, c.owner); err != nil {
			return ErrStorage
		}
		var digest any
		if c.value != nil {
			digest = c.digest[:]
		}
		if _, err := tx.ExecContext(ctx, "UPDATE unique_key_owners SET current_digest=? WHERE scope=? AND owner=?", digest, c.scope, c.owner); err != nil {
			return ErrStorage
		}
	}
	for _, c := range claims {
		if c.value == nil {
			continue
		}
		rows, err := tx.QueryContext(ctx, "SELECT owner,full_value FROM unique_key_claims WHERE scope=? AND digest=? FOR UPDATE", c.scope, c.digest[:])
		if err != nil {
			return ErrStorage
		}
		conflict, failed := false, false
		for rows.Next() {
			var value []byte
			var owner string
			if rows.Scan(&owner, &value) != nil {
				failed = true
				break
			}
			value, err = codec.open(c.scope, owner, value)
			if err != nil {
				failed = true
				break
			}
			if bytes.Equal(value, c.value) {
				conflict = true
			}
		}
		if rows.Err() != nil {
			failed = true
		}
		if rows.Close() != nil {
			failed = true
		}
		if failed {
			return ErrStorage
		}
		if conflict {
			return ErrConflict
		}
		stored, err := codec.seal(c.scope, c.owner, c.value)
		if err != nil {
			return ErrProtected
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO unique_key_claims(scope,owner,digest,full_value) VALUES(?,?,?,?)", c.scope, c.owner, c.digest[:], stored); err != nil {
			return ErrStorage
		}
	}
	return nil
}
