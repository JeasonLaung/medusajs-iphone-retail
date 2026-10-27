package rowimport

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
)

var (
	ErrHistoryStorage  = errors.New("historical mail transaction failed")
	ErrHistoryConflict = errors.New("historical mail differs from recorded migration evidence")
)

func sourceRowDigest(row SourceRow) [32]byte {
	keys := make([]string, 0, len(row))
	for key := range row {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	_, _ = hash.Write([]byte("eurorefurb.source-row.v1"))
	for _, key := range keys {
		cell := row[key]
		for _, value := range [][]byte{[]byte(key), cell.Raw} {
			_, _ = hash.Write(binary.BigEndian.AppendUint64(nil, uint64(len(value))))
			_, _ = hash.Write(value)
		}
		marker := byte(0)
		if cell.SQLNull {
			marker = 1
		}
		_, _ = hash.Write([]byte{marker})
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

// ImportHistoricalMail stages the source-derived row AND mandatory disposition
// in a caller-owned transaction. Callers MUST roll back on any error and MUST
// NOT commit before complete snapshot integrity/domain acceptance. The tx must
// select the isolated target containing the reviewed mail_messages table and
// migration 0004. Never use this against a live source database.
// Existing rows are never overwritten. Repeated exact input is accepted only
// if ledger provenance, policy and every persisted target value still match.
func ImportHistoricalMail(ctx context.Context, tx *sql.Tx, snapshotSHA256 string, source SourceRow) error {
	if tx == nil {
		return ErrHistoryStorage
	}
	snapshot, err := hex.DecodeString(snapshotSHA256)
	if err != nil || len(snapshot) != 32 || strings.ToLower(snapshotSHA256) != snapshotSHA256 {
		return ErrHistoryPolicy
	}
	converted, err := ConvertHistoricalMail(source)
	if err != nil {
		return err
	}
	sourceDigest := sourceRowDigest(source)
	targetBytes, err := json.Marshal(converted.Row)
	if err != nil {
		return ErrHistoryPolicy
	}
	targetDigest := sha256.Sum256(targetBytes)
	delivery := converted.Delivery
	attempt := make([]byte, 16)
	if _, err := rand.Read(attempt); err != nil {
		return ErrHistoryStorage
	}
	// Acquire the same durable row lock for concurrent/repeated attempts.
	_, err = tx.ExecContext(ctx, `INSERT INTO migration_mail_dispositions
	 (source_table,source_id,snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason)
	 VALUES(?,?,?,?,?,?,1,?,?) ON DUPLICATE KEY UPDATE source_id=migration_mail_dispositions.source_id`,
		delivery.SourceTable, delivery.SourceID, snapshot, sourceDigest[:], targetDigest[:], attempt, delivery.Disposition, delivery.Reason)
	if err != nil {
		return ErrHistoryStorage
	}
	var storedSnapshot, storedSource, storedTarget []byte
	var storedAttempt []byte
	var version int
	var disposition, reason string
	err = tx.QueryRowContext(ctx, `SELECT snapshot_sha256,source_row_sha256,target_row_sha256,initial_attempt,policy_version,disposition,reason
	 FROM migration_mail_dispositions WHERE source_table=? AND source_id=? FOR UPDATE`, delivery.SourceTable, delivery.SourceID).
		Scan(&storedSnapshot, &storedSource, &storedTarget, &storedAttempt, &version, &disposition, &reason)
	if err != nil {
		return ErrHistoryStorage
	}
	if !bytes.Equal(storedSnapshot, snapshot) || !bytes.Equal(storedSource, sourceDigest[:]) || !bytes.Equal(storedTarget, targetDigest[:]) || version != 1 || disposition != delivery.Disposition || reason != delivery.Reason {
		return ErrHistoryConflict
	}
	var columns, markers, comparisons []string
	for _, column := range converted.Row.Columns {
		// Names originate only from the embedded fixed plan, never caller SQL.
		quoted := "`" + column + "`"
		columns = append(columns, quoted)
		markers = append(markers, "?")
		comparisons = append(comparisons, "("+quoted+" <=> ?)")
	}
	if bytes.Equal(attempt, storedAttempt) {
		_, err = tx.ExecContext(ctx, "INSERT INTO mail_messages ("+strings.Join(columns, ",")+") VALUES("+strings.Join(markers, ",")+")", converted.Row.Values...)
		if err != nil {
			return ErrHistoryStorage
		}
	}
	args := append(append([]any{}, converted.Row.Values...), delivery.SourceID)
	var equal bool
	err = tx.QueryRowContext(ctx, "SELECT "+strings.Join(comparisons, " AND ")+" FROM mail_messages WHERE id=? FOR UPDATE", args...).Scan(&equal)
	if err == sql.ErrNoRows {
		return ErrHistoryConflict
	}
	if err != nil {
		return ErrHistoryStorage
	}
	if !equal {
		return ErrHistoryConflict
	}
	return nil
}

// HistoricalMailDeliveryDisposition is an internal worker prerequisite, not a
// permission grant. Both valid results prohibit delivery. A missing/corrupt
// ledger is an error, never permission to send. No release API is provided.
func HistoricalMailDeliveryDisposition(ctx context.Context, tx *sql.Tx, sourceID string) (string, error) {
	if tx == nil || sourceID == "" {
		return "", ErrHistoryStorage
	}
	var disposition, reason string
	var version int
	err := tx.QueryRowContext(ctx, `SELECT disposition,reason,policy_version FROM migration_mail_dispositions
	 WHERE source_table='mail_message' AND source_id=? FOR UPDATE`, sourceID).Scan(&disposition, &reason, &version)
	if err != nil {
		return "", ErrHistoryStorage
	}
	if version != 1 || !((disposition == "hold" && reason == "legacy-delivery-reconciliation") || (disposition == "suppress-permanently" && reason == "excluded-legacy-registration-capability")) {
		return "", ErrHistoryConflict
	}
	return disposition, nil
}
