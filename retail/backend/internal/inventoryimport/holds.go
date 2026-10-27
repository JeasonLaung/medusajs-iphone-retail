package inventoryimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/eurorefurb/retail/internal/inventory"
)

type assessment struct {
	Item        string  `json:"inventory_item_id"`
	Location    string  `json:"location_id"`
	Stocked     *string `json:"stocked_quantity"`
	Reserved    *string `json:"reserved_quantity"`
	Reservation string  `json:"reservation_quantity"`
	Count       int64   `json:"reservation_count"`
}

// PreserveHolds must follow a fully verified import in the SAME transaction.
// It only adds immutable evidence. It never clears a hold or repairs quantities.
// Success is not full snapshot acceptance; source counts and references still matter.
func PreserveHolds(ctx context.Context, tx *sql.Tx, snapshotSHA string) (int, error) {
	if tx == nil {
		return 0, ErrPolicy
	}
	snapshot, err := hex.DecodeString(snapshotSHA)
	if err != nil || len(snapshot) != 32 || snapshotSHA != strings.ToLower(snapshotSHA) {
		return 0, ErrPolicy
	}
	var version int
	var reads, writes bool
	if tx.QueryRowContext(ctx, "SELECT schema_version,reads_enabled,writes_enabled FROM inventory_runtime_schema WHERE id=1 FOR UPDATE").Scan(&version, &reads, &writes) != nil || version != 1 || !reads || writes {
		return 0, ErrPolicy
	}
	// All rows are locked before aggregation; old application workers must never
	// share this target. Inventory runtime writes remain disabled by schema.
	rows, err := tx.QueryContext(ctx, "SELECT id FROM inventory_levels ORDER BY id FOR UPDATE")
	if err != nil {
		return 0, ErrStorage
	}
	n := 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return 0, ErrStorage
		}
		n++
		if n > 100000 {
			rows.Close()
			return 0, ErrPolicy
		}
	}
	if rows.Err() != nil || rows.Close() != nil {
		return 0, ErrStorage
	}
	rows, err = tx.QueryContext(ctx, "SELECT id FROM inventory_reservations ORDER BY id FOR UPDATE")
	if err != nil {
		return 0, ErrStorage
	}
	n = 0
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return 0, ErrStorage
		}
		n++
		if n > 100000 {
			rows.Close()
			return 0, ErrPolicy
		}
	}
	if rows.Err() != nil || rows.Close() != nil {
		return 0, ErrStorage
	}
	// Equality against each source column is checked during Import. Here retain
	// the two independent facts rather than replacing either by the other.
	rows, err = tx.QueryContext(ctx, `SELECT l.inventory_item_id,l.location_id,l.stocked_quantity,l.reserved_quantity,COALESCE(r.quantity,0),COALESCE(r.n,0)
 FROM inventory_levels l LEFT JOIN (SELECT inventory_item_id,location_id,SUM(quantity) quantity,COUNT(*) n FROM inventory_reservations WHERE deleted_at IS NULL GROUP BY inventory_item_id,location_id) r ON r.inventory_item_id=l.inventory_item_id AND r.location_id=l.location_id
 WHERE l.deleted_at IS NULL ORDER BY l.inventory_item_id,l.location_id`)
	if err != nil {
		return 0, ErrStorage
	}
	var assessments []assessment
	for rows.Next() {
		var a assessment
		if rows.Scan(&a.Item, &a.Location, &a.Stocked, &a.Reserved, &a.Reservation, &a.Count) != nil {
			rows.Close()
			return 0, ErrStorage
		}
		assessments = append(assessments, a)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return 0, ErrStorage
	}
	rows, err = tx.QueryContext(ctx, `SELECT r.inventory_item_id,r.location_id,SUM(r.quantity),COUNT(*) FROM inventory_reservations r
 WHERE r.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM inventory_levels l WHERE l.inventory_item_id=r.inventory_item_id AND l.location_id=r.location_id AND l.deleted_at IS NULL)
 GROUP BY r.inventory_item_id,r.location_id ORDER BY r.inventory_item_id,r.location_id`)
	if err != nil {
		return 0, ErrStorage
	}
	for rows.Next() {
		var a assessment
		if rows.Scan(&a.Item, &a.Location, &a.Reservation, &a.Count) != nil {
			rows.Close()
			return 0, ErrStorage
		}
		assessments = append(assessments, a)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return 0, ErrStorage
	}
	count := 0
	for _, a := range assessments {
		var reasons []string
		if a.Reserved == nil {
			reasons = append(reasons, "unmatched_reservations")
		} else {
			equal, err := inventory.Equal(*a.Reserved, a.Reservation)
			if err != nil {
				return 0, ErrPolicy
			}
			if !equal {
				reasons = append(reasons, "reservation_mismatch")
			}
			balance, err := inventory.Difference(*a.Stocked, *a.Reserved)
			if err != nil {
				return 0, ErrPolicy
			}
			if strings.HasPrefix(balance, "-") {
				reasons = append(reasons, "negative_balance")
			}
		}
		evidence, err := json.Marshal(a)
		if err != nil {
			return 0, ErrPolicy
		}
		hash := sha256.Sum256(evidence)
		for _, reason := range reasons {
			if _, err := tx.ExecContext(ctx, `INSERT INTO inventory_migration_holds(inventory_item_id,location_id,reason,snapshot_sha256,evidence_sha256,evidence_json,released,policy_version) VALUES(?,?,?,?,?,?,FALSE,1)
    ON DUPLICATE KEY UPDATE reason=inventory_migration_holds.reason`, a.Item, a.Location, reason, snapshot, hash[:], string(evidence)); err != nil {
				return 0, ErrStorage
			}
			var oldSnapshot, oldHash []byte
			var oldJSON string
			var released bool
			var policy int
			if tx.QueryRowContext(ctx, `SELECT snapshot_sha256,evidence_sha256,evidence_json,released,policy_version FROM inventory_migration_holds WHERE inventory_item_id=? AND location_id=? AND reason=? FOR UPDATE`, a.Item, a.Location, reason).Scan(&oldSnapshot, &oldHash, &oldJSON, &released, &policy) != nil {
				return 0, ErrStorage
			}
			if !bytes.Equal(oldSnapshot, snapshot) || !bytes.Equal(oldHash, hash[:]) || oldJSON != string(evidence) || released || policy != 1 {
				return 0, ErrConflict
			}
			count++
		}
	}
	return count, nil
}
