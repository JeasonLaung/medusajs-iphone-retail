// Package merchantimport stages reviewed merchant configurations, durable
// external-effect holds and full-value identity claims in the SAME transaction.
// It has no environment, provider, HTTP or commit side effects.
package merchantimport

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

	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/uniquekey"
)

var (
	ErrPolicy   = errors.New("merchant import requires reviewed configuration")
	ErrStorage  = errors.New("merchant import transaction failed")
	ErrConflict = errors.New("merchant import evidence or identity conflict")
)

type plan struct {
	target  string
	secrets []string
	claims  bool
}

// No arbitrary tables, SQL names or grant scopes can be supplied by callers.
var plans = map[string]plan{
	"mail_account":      {"mail_accounts", []string{"api_key", "webhook_secret"}, true},
	"seo_provider":      {"seo_providers", []string{"api_key"}, false},
	"seo_setting":       {"seo_settings", []string{"value"}, true},
	"webhook_endpoint":  {"webhook_endpoints", []string{"url", "secret", "token"}, false},
	"mollie_config":     {"mollie_configs", []string{"api_key"}, false},
	"paypal_connection": {"paypal_connections", []string{"auth_code", "seller_client_secret", "app_access_token", "metadata"}, false},
	"paypal_settings":   {"paypal_settings", []string{"data"}, false},
}

type Importer struct {
	ring      *protected.Ring
	converter *rowimport.ConfigSecrets
}

func New(ring *protected.Ring, legacy *legacysecret.Keys, paypal *legacysecret.PayPal) (*Importer, error) {
	converter, err := rowimport.NewConfigSecretsWithPayPal(ring, legacy, paypal)
	if err != nil {
		return nil, ErrPolicy
	}
	return &Importer{ring, converter}, nil
}

func sourceDigest(source rowimport.SourceRow) []byte {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	hash.Write([]byte("eurorefurb.merchant-source.v1"))
	for _, key := range keys {
		cell := source[key]
		for _, value := range [][]byte{[]byte(key), cell.Raw} {
			hash.Write(binary.BigEndian.AppendUint64(nil, uint64(len(value))))
			hash.Write(value)
		}
		marker := byte(0)
		if cell.SQLNull {
			marker = 1
		}
		hash.Write([]byte{marker})
	}
	return hash.Sum(nil)
}

// Import requires an isolated target with explicit business/registry/ledger
// migrations. Caller MUST roll back on ANY error, and MUST NOT commit before
// complete snapshot integrity and domain acceptance. No implicit upsert/release
// or startup DDL. Existing configuration and source status are never rewritten.
func (i *Importer) Import(ctx context.Context, tx *sql.Tx, snapshotSHA, table string, source rowimport.SourceRow) error {
	p, exists := plans[table]
	if !exists || i == nil || i.converter == nil || i.ring == nil || tx == nil {
		return ErrPolicy
	}
	snapshot, err := hex.DecodeString(snapshotSHA)
	if err != nil || len(snapshot) != 32 || snapshotSHA != strings.ToLower(snapshotSHA) {
		return ErrPolicy
	}
	bound, err := i.converter.Convert(table, source)
	if err != nil || bound.Table != p.target {
		return ErrPolicy
	}
	id, err := importvalue.Text(source["id"].Raw, 191)
	if err != nil || id == "" || source["id"].SQLNull {
		return ErrPolicy
	}
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
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_merchant_dispositions
	 (source_table,source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason)
	 VALUES(?,?,?,?,?,?,1,'hold','legacy-merchant-reconciliation')
	 ON DUPLICATE KEY UPDATE source_id=migration_merchant_dispositions.source_id`, table, id, snapshot, sourceHash, targetHash[:], attempt)
	if err != nil {
		return ErrStorage
	}
	var storedSnapshot, storedSource, storedTarget, storedAttempt []byte
	var version int
	var disposition, reason string
	err = tx.QueryRowContext(ctx, `SELECT snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason
	 FROM migration_merchant_dispositions WHERE source_table=? AND source_id=? FOR UPDATE`, table, id).
		Scan(&storedSnapshot, &storedSource, &storedTarget, &storedAttempt, &version, &disposition, &reason)
	if err != nil {
		return ErrStorage
	}
	if !bytes.Equal(storedSnapshot, snapshot) || !bytes.Equal(storedSource, sourceHash) || version != 1 || disposition != "hold" || reason != "legacy-merchant-reconciliation" {
		return ErrConflict
	}
	first := bytes.Equal(attempt, storedAttempt)
	if !first {
		var names []string
		var destinations []any
		values := make([][]byte, len(p.secrets))
		for index, column := range p.secrets {
			names = append(names, "`"+column+"`")
			destinations = append(destinations, &values[index])
		}
		err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(names, ",")+" FROM `"+p.target+"` WHERE id=? FOR UPDATE", id).Scan(destinations...)
		if err == sql.ErrNoRows {
			return ErrConflict
		}
		if err != nil {
			return ErrStorage
		}
		for index, column := range p.secrets {
			position := -1
			for j, name := range bound.Columns {
				if name == column {
					position = j
				}
			}
			if position < 0 {
				return ErrPolicy
			}
			expected := bound.Values[position]
			stored := values[index]
			if expected == nil {
				if stored != nil {
					return ErrConflict
				}
				continue
			}
			freshCipher, ok := expected.([]byte)
			if !ok || stored == nil {
				return ErrConflict
			}
			context := protected.Context{Purpose: "merchant-config", Table: p.target, Column: column, Row: id}
			fresh, freshErr := i.ring.Open(context, freshCipher)
			original, originalErr := i.ring.Open(context, stored)
			equal := freshErr == nil && originalErr == nil && bytes.Equal(fresh, original)
			clear(fresh)
			clear(original)
			if !equal {
				return ErrConflict
			}
			bound.Values[position] = stored // preserve first nonce and key ID
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
	// These two reviewed source keys exceed native MySQL index lengths.
	// Full-value claims belong to the SAME transaction as row and hold.
	if p.claims {
		projection := uniquekey.TargetRow{}
		for name, cell := range source {
			if cell.SQLNull {
				projection[name] = json.RawMessage(`null`)
			} else {
				projection[name] = cell.Raw
			}
		}
		if err := uniquekey.ApplySourceRow(ctx, tx, p.target, projection); err != nil {
			if errors.Is(err, uniquekey.ErrConflict) {
				return ErrConflict
			}
			return ErrStorage
		}
	}
	var columns, markers, comparisons []string
	for _, column := range bound.Columns {
		quoted := "`" + column + "`"
		columns = append(columns, quoted)
		markers = append(markers, "?")
		comparisons = append(comparisons, "("+quoted+" <=> ?)")
	}
	if first {
		_, err = tx.ExecContext(ctx, "INSERT INTO `"+p.target+"` ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", bound.Values...)
		if err != nil {
			return ErrStorage
		}
	}
	var equal bool
	args := append(append([]any{}, bound.Values...), id)
	err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+p.target+"` WHERE id=? FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows || (err == nil && !equal) {
		return ErrConflict
	}
	if err != nil {
		return ErrStorage
	}
	return nil
}

// Disposition is an internal worker prerequisite, never an authorization grant.
// The only valid return value is hold, denying external effects. A missing or
// corrupt record fails closed. Future workers MUST enforce it; no release API.
func Disposition(ctx context.Context, tx *sql.Tx, table, id string) (string, error) {
	if _, exists := plans[table]; !exists || tx == nil || id == "" {
		return "", ErrPolicy
	}
	var version int
	var disposition, reason string
	err := tx.QueryRowContext(ctx, `SELECT policy_version,disposition,reason FROM migration_merchant_dispositions
	 WHERE source_table=? AND source_id=? FOR UPDATE`, table, id).Scan(&version, &disposition, &reason)
	if err != nil {
		return "", ErrStorage
	}
	if version != 1 || disposition != "hold" || reason != "legacy-merchant-reconciliation" {
		return "", ErrConflict
	}
	return disposition, nil
}
