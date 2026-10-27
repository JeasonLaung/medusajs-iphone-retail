// Package customerimport stages historical business subjects, never login grants.
// All SQL writes use one caller transaction with no implicit commit or provider.
package customerimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
)

var (
	ErrPolicy   = errors.New("historical customer requires reviewed policy")
	ErrStorage  = errors.New("historical customer transaction failed")
	ErrConflict = errors.New("historical customer evidence or identity conflict")
)

type Importer struct {
	ring      *protected.Ring
	converter *rowimport.CustomerHistory
}

func New(ring *protected.Ring) (*Importer, error) {
	c, err := rowimport.NewCustomerHistory(ring)
	if err != nil {
		return nil, ErrPolicy
	}
	return &Importer{ring, c}, nil
}

func sourceDigest(source rowimport.SourceRow) []byte {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte("eurorefurb.customer-subject-source.v1"))
	for _, key := range keys {
		cell := source[key]
		for _, v := range [][]byte{[]byte(key), cell.Raw} {
			h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(v))))
			h.Write(v)
		}
		marker := byte(0)
		if cell.SQLNull {
			marker = 1
		}
		h.Write([]byte{marker})
	}
	return h.Sum(nil)
}

// Import requires the explicit isolated 0016/0018 schema. Caller
// MUST roll back on ANY error and MUST NOT commit before complete snapshot,
// references and domain acceptance. There is no upsert, release, identity/email
// reassignment, customer-link recreation, startup DDL or remote provider call.
func (i *Importer) Import(ctx context.Context, tx *sql.Tx, snapshotSHA string, source rowimport.SourceRow) error {
	if i == nil || i.ring == nil || i.converter == nil || tx == nil {
		return ErrPolicy
	}
	snapshot, err := hex.DecodeString(snapshotSHA)
	if err != nil || len(snapshot) != 32 || snapshotSHA != strings.ToLower(snapshotSHA) {
		return ErrPolicy
	}
	history, err := i.converter.Convert(source)
	if err != nil || history.Row.Table != "historical_customer_subjects" || history.Binding.Disposition != "excluded" || history.Binding.Reason != "historical-subject-not-login" {
		return ErrPolicy
	}
	bound := history.Row
	id := history.Binding.SourceID
	sourceHash := sourceDigest(source)
	encoded, err := json.Marshal(bound)
	if err != nil {
		return ErrPolicy
	}
	targetHash := sha256.Sum256(encoded)
	attempt := make([]byte, 16)
	if _, err := rand.Read(attempt); err != nil {
		return ErrStorage
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_customer_dispositions
	 (source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason)
	 VALUES(?,?,?,?,?,1,'excluded','historical-subject-not-login')
	 ON DUPLICATE KEY UPDATE source_id=migration_customer_dispositions.source_id`, id, snapshot, sourceHash, targetHash[:], attempt)
	if err != nil {
		return ErrStorage
	}
	var storedSnapshot, storedSource, storedTarget, storedAttempt []byte
	var version int
	var disposition, reason string
	err = tx.QueryRowContext(ctx, `SELECT snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason
	 FROM migration_customer_dispositions WHERE source_id=? FOR UPDATE`, id).Scan(&storedSnapshot, &storedSource, &storedTarget, &storedAttempt, &version, &disposition, &reason)
	if err != nil {
		return ErrStorage
	}
	if !bytes.Equal(storedSnapshot, snapshot) || !bytes.Equal(storedSource, sourceHash) || version != 1 || disposition != "excluded" || reason != "historical-subject-not-login" {
		return ErrConflict
	}
	first := bytes.Equal(attempt, storedAttempt)
	if !first {
		var stored []byte
		err := tx.QueryRowContext(ctx, "SELECT metadata FROM historical_customer_subjects WHERE id=? FOR UPDATE", id).Scan(&stored)
		if err == sql.ErrNoRows {
			return ErrConflict
		}
		if err != nil {
			return ErrStorage
		}
		for _, column := range []string{"metadata"} {
			position := -1
			for j, name := range bound.Columns {
				if name == column {
					position = j
				}
			}
			if position < 0 {
				return ErrPolicy
			}
			if bound.Values[position] == nil {
				if stored != nil {
					return ErrConflict
				}
				continue
			}
			fresh, ok := bound.Values[position].([]byte)
			if !ok || stored == nil {
				return ErrConflict
			}
			freshPlain, freshErr := rowimport.OpenCustomerMetadata(i.ring, id, fresh)
			oldPlain, oldErr := rowimport.OpenCustomerMetadata(i.ring, id, stored)
			equal := freshErr == nil && oldErr == nil && bytes.Equal(freshPlain, oldPlain)
			clear(freshPlain)
			clear(oldPlain)
			if !equal {
				return ErrConflict
			}
			bound.Values[position] = stored
		}
		encoded, err = json.Marshal(bound)
		if err != nil {
			return ErrStorage
		}
		targetHash = sha256.Sum256(encoded)
	}
	if !bytes.Equal(storedTarget, targetHash[:]) {
		return ErrConflict
	}
	var columns, markers, comparisons []string
	for _, column := range bound.Columns {
		quoted := "`" + column + "`"
		columns = append(columns, quoted)
		markers = append(markers, "?")
		comparisons = append(comparisons, "("+quoted+" <=> ?)")
	}
	if first {
		if _, err := tx.ExecContext(ctx, "INSERT INTO historical_customer_subjects ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...); err != nil {
			return ErrStorage
		}
	}
	var equal bool
	args := append(append([]any{}, bound.Values...), id)
	err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM historical_customer_subjects WHERE id=? FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows || (err == nil && !equal) {
		return ErrConflict
	}
	if err != nil {
		return ErrStorage
	}
	return nil
}

// BindingDisposition reports only the permanent exclusion of old login grants.
// It cannot create an account, authorize a private order or rebind a customer.
func BindingDisposition(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	if tx == nil || id == "" {
		return "", ErrPolicy
	}
	var version int
	var d, reason string
	if tx.QueryRowContext(ctx, "SELECT policy_version,disposition,reason FROM migration_customer_dispositions WHERE source_id=? FOR SHARE", id).Scan(&version, &d, &reason) != nil {
		return "", ErrStorage
	}
	if version != 1 || d != "excluded" || reason != "historical-subject-not-login" {
		return "", ErrConflict
	}
	return d, nil
}
