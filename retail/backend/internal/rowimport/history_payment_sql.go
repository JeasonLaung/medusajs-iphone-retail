package rowimport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrPaymentStorage  = errors.New("historical payment transaction failed")
	ErrPaymentConflict = errors.New("historical payment differs from recorded migration evidence")
)

// Import stages the complete payment row and mandatory operation hold in a
// caller-owned isolated-target transaction. Roll back on ANY error. Do not
// commit before complete snapshot integrity and domain acceptance. This method
// neither commits nor calls a provider. Parents must already be staged.
// Exact replay authenticates the ORIGINAL persisted ciphertext, without
// resealing/overwriting it; random nonces must not turn replay into a conflict.
func (p *PaymentHistory) Import(ctx context.Context, tx *sql.Tx, snapshotSHA256, table string, source SourceRow) error {
	if p == nil || p.ring == nil || tx == nil {
		return ErrPaymentStorage
	}
	snapshot, err := hex.DecodeString(snapshotSHA256)
	if err != nil || len(snapshot) != 32 || strings.ToLower(snapshotSHA256) != snapshotSHA256 {
		return ErrHistoryPolicy
	}
	converted, err := p.Convert(table, source)
	if err != nil {
		return err
	}
	identity, _ := paymentRowIdentity(table, source)
	sourceDigest := sourceRowDigest(source)
	encoded, err := json.Marshal(converted.Row)
	if err != nil {
		return ErrHistoryPolicy
	}
	targetDigest := sha256.Sum256(encoded)
	attempt := make([]byte, 16)
	if _, err := rand.Read(attempt); err != nil {
		return ErrPaymentStorage
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_payment_dispositions
	 (source_table,source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason)
	 VALUES(?,?,?,?,?,?,1,'hold','legacy-payment-reconciliation')
	 ON DUPLICATE KEY UPDATE source_id=migration_payment_dispositions.source_id`,
		table, identity.SourceID, snapshot, sourceDigest[:], targetDigest[:], attempt)
	if err != nil {
		return ErrPaymentStorage
	}
	var storedSnapshot, storedSource, storedTarget, storedAttempt []byte
	var version int
	var disposition, reason string
	err = tx.QueryRowContext(ctx, `SELECT snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason
	 FROM migration_payment_dispositions WHERE source_table=? AND source_id=? FOR UPDATE`, table, identity.SourceID).
		Scan(&storedSnapshot, &storedSource, &storedTarget, &storedAttempt, &version, &disposition, &reason)
	if err != nil {
		return ErrPaymentStorage
	}
	if !bytes.Equal(storedSnapshot, snapshot) || !bytes.Equal(storedSource, sourceDigest[:]) || version != 1 || disposition != "hold" || reason != "legacy-payment-reconciliation" {
		return ErrPaymentConflict
	}
	first := bytes.Equal(storedAttempt, attempt)
	if !first {
		// Current read, not a repeatable-read snapshot. Lock ordering is always
		// ledger then business row; never trust a ledger without its target.
		var names []string
		var destinations []any
		var protectedIndices []int
		var values []sql.NullString
		for i, column := range converted.Row.Columns {
			if column == "data" || column == "context" || column == "metadata" {
				names = append(names, "`"+column+"`")
				protectedIndices = append(protectedIndices, i)
				values = append(values, sql.NullString{})
			}
		}
		for i := range values {
			destinations = append(destinations, &values[i])
		}
		err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(names, ",")+" FROM `"+converted.Row.Table+"` WHERE id=? FOR UPDATE", identity.SourceID).Scan(destinations...)
		if err == sql.ErrNoRows {
			return ErrPaymentConflict
		}
		if err != nil {
			return ErrPaymentStorage
		}
		for j, i := range protectedIndices {
			column := converted.Row.Columns[i]
			expected := converted.Row.Values[i]
			persisted := values[j]
			if expected == nil {
				if persisted.Valid {
					return ErrPaymentConflict
				}
				continue
			}
			if !persisted.Valid {
				return ErrPaymentConflict
			}
			open := func(stored string) (json.RawMessage, error) {
				switch column {
				case "data":
					return OpenPaymentData(p.ring, identity, stored)
				case "context":
					return OpenPaymentContext(p.ring, identity.SourceID, stored)
				case "metadata":
					return OpenPaymentMetadata(p.ring, identity, stored)
				}
				return nil, ErrProtection
			}
			fresh, freshErr := open(expected.(string))
			original, originalErr := open(persisted.String)
			equal := freshErr == nil && originalErr == nil && bytes.Equal(fresh, original)
			clear(fresh)
			clear(original)
			if !equal {
				return ErrPaymentConflict
			}
			converted.Row.Values[i] = persisted.String
		}
		encoded, err = json.Marshal(converted.Row)
		if err != nil {
			return ErrPaymentStorage
		}
		targetDigest = sha256.Sum256(encoded)
	}
	if !bytes.Equal(storedTarget, targetDigest[:]) {
		return ErrPaymentConflict
	}
	var columns, markers, comparisons []string
	for _, column := range converted.Row.Columns {
		quoted := "`" + column + "`"
		columns = append(columns, quoted)
		markers = append(markers, "?")
		parameter := "?"
		if column == "amount" {
			// Bound exact decimals are strings. Force DECIMAL comparison rather
			// than relying on string/numeric implicit coercion during replay.
			parameter = "CAST(? AS DECIMAL(38,18))"
		}
		comparisons = append(comparisons, "("+quoted+" <=> "+parameter+")")
	}
	if first {
		_, err = tx.ExecContext(ctx, "INSERT INTO `"+converted.Row.Table+"` ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", converted.Row.Values...)
		if err != nil {
			return ErrPaymentStorage
		}
	}
	args := append(append([]any{}, converted.Row.Values...), identity.SourceID)
	var equal bool
	err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM `"+converted.Row.Table+"` WHERE id=? FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows {
		return ErrPaymentConflict
	}
	if err != nil {
		return ErrPaymentStorage
	}
	if !equal {
		return ErrPaymentConflict
	}
	return nil
}

// HistoricalPaymentDisposition is a mandatory internal worker prerequisite.
// The only accepted result is hold, which denies ALL payment operations.
// Missing/corrupt/unknown records fail closed. There is no release operation.
// Future workers must enforce this in addition to their normal authorization.
func HistoricalPaymentDisposition(ctx context.Context, tx *sql.Tx, table, id string) (string, error) {
	if tx == nil || (table != "payment_session" && table != "payment") || id == "" {
		return "", ErrPaymentStorage
	}
	var version int
	var disposition, reason string
	err := tx.QueryRowContext(ctx, `SELECT policy_version,disposition,reason FROM migration_payment_dispositions
	 WHERE source_table=? AND source_id=? FOR UPDATE`, table, id).Scan(&version, &disposition, &reason)
	if err != nil {
		return "", ErrPaymentStorage
	}
	if version != 1 || disposition != "hold" || reason != "legacy-payment-reconciliation" {
		return "", ErrPaymentConflict
	}
	return disposition, nil
}
