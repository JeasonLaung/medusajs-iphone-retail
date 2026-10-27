package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

var ErrReservationInput = errors.New("invalid inventory reservation command")
var ErrReservationConflict = errors.New("inventory reservation operation changed")
var ErrInsufficientStock = errors.New("insufficient stock at reservation location")
var ErrReservationStorage = errors.New("inventory reservation transaction failed")

// ReservationInput is INTERNAL: the owning order workflow must validate current
// identity, permissions, channel/location, order state and every line binding.
// Quantity is required_quantity * order-line quantity, NOT a client price/amount.
// There is deliberately no HTTP handler and the capability defaults to disabled.
type ReservationInput struct {
	ID              string `json:"id"`
	InventoryItemID string `json:"inventory_item_id"`
	LocationID      string `json:"location_id"`
	LineItemID      string `json:"line_item_id"`
	Quantity        string `json:"quantity"`
	AllowBackorder  bool   `json:"allow_backorder"`
}
type ReservationBinding struct {
	ID              string `json:"id"`
	InventoryItemID string `json:"inventory_item_id"`
	LocationID      string `json:"location_id"`
	LineItemID      string `json:"line_item_id"`
}
type ReservationCommand struct {
	OperationID string               `json:"operation_id"`
	OwnerID     string               `json:"owner_id"`
	ActorRef    string               `json:"actor_ref"`
	Kind        string               `json:"kind"`
	Reserve     []ReservationInput   `json:"reserve"`
	Release     []ReservationBinding `json:"release"`
}
type ReservationResult struct {
	IDs      []string `json:"ids"`
	Replayed bool     `json:"replayed"`
}
type reservationSnapshot struct {
	ID              string `json:"id"`
	InventoryItemID string `json:"inventory_item_id"`
	LocationID      string `json:"location_id"`
	LineItemID      string `json:"line_item_id"`
	Quantity        string `json:"quantity"`
	AllowBackorder  *bool  `json:"allow_backorder"`
}
type reservationPair struct{ item, location string }
type reservationLevel struct {
	ID              string    `json:"id"`
	InventoryItemID string    `json:"inventory_item_id"`
	LocationID      string    `json:"location_id"`
	Stocked         string    `json:"stocked_quantity"`
	Reserved        string    `json:"reserved_quantity"`
	Incoming        string    `json:"incoming_quantity"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RequiredInventoryQuantity computes the exact product without float or rounding.
// A fractional result below scale 18 or an overflow is rejected, never rounded.
func RequiredInventoryQuantity(units, required string) (string, error) {
	a, err := scaled(units)
	if err != nil || a.Sign() <= 0 {
		return "", ErrQuantity
	}
	b, err := scaled(required)
	if err != nil || b.Sign() <= 0 {
		return "", ErrQuantity
	}
	product := new(big.Int).Mul(a, b)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(product, scale, remainder)
	if remainder.Sign() != 0 || !fitsQuantity(quotient) {
		return "", ErrQuantity
	}
	return fixed(quotient), nil
}
func fitsQuantity(n *big.Int) bool { return len(new(big.Int).Abs(n).String()) <= 38 }
func normalizedReservationCommand(in ReservationCommand) (ReservationCommand, error) {
	id, err := uuid.Parse(in.OperationID)
	if err != nil || id.String() != in.OperationID || id.Version() != 4 || !validID(in.OwnerID) || !validID(in.ActorRef) {
		return in, ErrReservationInput
	}
	// Copy before sorting: callers may share an immutable request across retries.
	in.Reserve = append([]ReservationInput(nil), in.Reserve...)
	in.Release = append([]ReservationBinding(nil), in.Release...)
	seen := map[string]bool{}
	valid := func(id, item, location, line string) bool {
		if !validID(id) || !validID(item) || !validID(location) || !validID(line) || seen[id] {
			return false
		}
		seen[id] = true
		return true
	}
	switch in.Kind {
	case "reserve":
		if len(in.Reserve) == 0 || len(in.Reserve) > 1000 || len(in.Release) != 0 {
			return in, ErrReservationInput
		}
		for _, r := range in.Reserve {
			if !valid(r.ID, r.InventoryItemID, r.LocationID, r.LineItemID) {
				return in, ErrReservationInput
			}
			n, err := scaled(r.Quantity)
			if err != nil || n.Sign() <= 0 || !fitsQuantity(n) || fixed(n) != r.Quantity {
				return in, ErrReservationInput
			}
		}
		sort.Slice(in.Reserve, func(i, j int) bool { return in.Reserve[i].ID < in.Reserve[j].ID })
	case "release":
		if len(in.Release) == 0 || len(in.Release) > 1000 || len(in.Reserve) != 0 {
			return in, ErrReservationInput
		}
		for _, r := range in.Release {
			if !valid(r.ID, r.InventoryItemID, r.LocationID, r.LineItemID) {
				return in, ErrReservationInput
			}
		}
		sort.Slice(in.Release, func(i, j int) bool { return in.Release[i].ID < in.Release[j].ID })
	default:
		return in, ErrReservationInput
	}
	return in, nil
}

// ApplyReservations participates in the caller's existing MySQL transaction. It
// never commits, owns no Redis lock and makes no provider/event calls. Caller MUST
// roll back the WHOLE transaction on ANY error, including a later order failure.
// It does not establish ownership: OwnerID/line membership must be verified by
// the order workflow before invoking this internal participant.
func ApplyReservations(ctx context.Context, tx *sql.Tx, input ReservationCommand) (ReservationResult, error) {
	result := ReservationResult{IDs: []string{}}
	in, err := normalizedReservationCommand(input)
	if err != nil || tx == nil {
		return result, ErrReservationInput
	}
	var expected struct {
		SchemaVersion int    `json:"schema_version"`
		ModelSHA      string `json:"model_sha256"`
	}
	if json.Unmarshal(schemaPlan, &expected) != nil {
		return result, ErrSchema
	}
	var version int
	var model string
	var reads, writes bool
	if tx.QueryRowContext(ctx, "SELECT schema_version,model_sha256,reads_enabled,writes_enabled FROM inventory_runtime_schema WHERE id=1 FOR SHARE").Scan(&version, &model, &reads, &writes) != nil || version != expected.SchemaVersion || model != expected.ModelSHA || !reads || writes {
		return result, ErrSchema
	}
	var enabled bool
	if tx.QueryRowContext(ctx, "SELECT version,enabled FROM inventory_command_capabilities WHERE command='inventory.reservations' FOR SHARE").Scan(&version, &enabled) != nil || version != 1 || !enabled {
		return result, ErrSchema
	}
	pairs := map[reservationPair]bool{}
	for _, r := range in.Reserve {
		pairs[reservationPair{r.InventoryItemID, r.LocationID}] = true
		result.IDs = append(result.IDs, r.ID)
	}
	for _, r := range in.Release {
		pairs[reservationPair{r.InventoryItemID, r.LocationID}] = true
		result.IDs = append(result.IDs, r.ID)
	}
	ordered := make([]reservationPair, 0, len(pairs))
	for p := range pairs {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].item == ordered[j].item {
			return ordered[i].location < ordered[j].location
		}
		return ordered[i].item < ordered[j].item
	})
	levels := map[reservationPair]reservationLevel{}
	for _, p := range ordered {
		var l reservationLevel
		err = tx.QueryRowContext(ctx, `SELECT id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,updated_at FROM inventory_levels WHERE inventory_item_id=? AND location_id=? AND deleted_at IS NULL FOR UPDATE`, p.item, p.location).Scan(&l.ID, &l.InventoryItemID, &l.LocationID, &l.Stocked, &l.Reserved, &l.Incoming, &l.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return result, ErrReservationConflict
		}
		if err != nil {
			return result, ErrReservationStorage
		}
		levels[p] = l
	}
	request, _ := json.Marshal(in)
	// Current read AFTER level locks, so a retry waiting on the original commit
	// observes its receipt even under an already-established RR snapshot.
	var oldRequest, oldResult string
	err = tx.QueryRowContext(ctx, "SELECT request_json,result_json FROM inventory_reservation_operations WHERE operation_id=? FOR SHARE", in.OperationID).Scan(&oldRequest, &oldResult)
	if err == nil {
		if oldRequest != string(request) {
			return result, ErrReservationConflict
		}
		var previous ReservationResult
		if json.Unmarshal([]byte(oldResult), &previous) != nil || len(previous.IDs) != len(result.IDs) {
			return result, ErrReservationStorage
		}
		for i, id := range previous.IDs {
			if id != result.IDs[i] {
				return result, ErrReservationStorage
			}
		}
		previous.Replayed = true
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, ErrReservationStorage
	}
	deltas := map[reservationPair]*big.Int{}
	nonBackorder := map[reservationPair]*big.Int{}
	for _, p := range ordered {
		deltas[p] = new(big.Int)
		nonBackorder[p] = new(big.Int)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var beforeReservations []reservationSnapshot
	if in.Kind == "reserve" {
		for _, r := range in.Reserve {
			n, _ := scaled(r.Quantity)
			p := reservationPair{r.InventoryItemID, r.LocationID}
			deltas[p].Add(deltas[p], n)
			if !r.AllowBackorder {
				nonBackorder[p].Add(nonBackorder[p], n)
			}
		}
		for _, p := range ordered {
			// Only creation consumes capacity. Release may proceed through a migration
			// hold without erasing evidence or silently correcting the discrepancy.
			var hold string
			err = tx.QueryRowContext(ctx, "SELECT reason FROM inventory_migration_holds WHERE inventory_item_id=? AND location_id=? ORDER BY reason LIMIT 1 FOR SHARE", p.item, p.location).Scan(&hold)
			if err == nil {
				return result, ErrStockHeld
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return result, ErrReservationStorage
			}
			for _, parent := range []struct{ table, id string }{{"inventory_items", p.item}, {"inventory_locations", p.location}} {
				var id string
				err = tx.QueryRowContext(ctx, "SELECT id FROM "+parent.table+" WHERE id=? AND deleted_at IS NULL FOR SHARE", parent.id).Scan(&id)
				if errors.Is(err, sql.ErrNoRows) {
					return result, ErrReservationConflict
				}
				if err != nil {
					return result, ErrReservationStorage
				}
			}
			rows, err := tx.QueryContext(ctx, "SELECT quantity FROM inventory_reservations WHERE inventory_item_id=? AND location_id=? AND deleted_at IS NULL ORDER BY id LIMIT 10001 FOR UPDATE", p.item, p.location)
			if err != nil {
				return result, ErrReservationStorage
			}
			total := new(big.Int)
			count := 0
			for rows.Next() {
				var q string
				if rows.Scan(&q) != nil {
					rows.Close()
					return result, ErrReservationStorage
				}
				n, err := scaled(q)
				if err != nil || n.Sign() < 0 {
					rows.Close()
					return result, ErrStockHeld
				}
				total.Add(total, n)
				count++
			}
			if rows.Err() != nil || rows.Close() != nil {
				return result, ErrReservationStorage
			}
			if count > 10000 {
				return result, ErrStockHeld
			}
			l := levels[p]
			reserved, err := scaled(l.Reserved)
			if err != nil || reserved.Cmp(total) != 0 {
				return result, ErrStockHeld
			}
			stocked, err := scaled(l.Stocked)
			if err != nil {
				return result, ErrStockHeld
			}
			available := new(big.Int).Sub(stocked, reserved)
			// Sum all non-backorder lines sharing a level. Native per-line checks alone
			// can accept two individually affordable lines whose combined demand exceeds stock.
			if nonBackorder[p].Sign() > 0 && available.Cmp(nonBackorder[p]) < 0 {
				return result, ErrInsufficientStock
			}
		}
	} else {
		for _, binding := range in.Release {
			var r reservationSnapshot
			var line sql.NullString
			var deleted sql.NullTime
			err = tx.QueryRowContext(ctx, "SELECT id,inventory_item_id,location_id,line_item_id,quantity,allow_backorder,deleted_at FROM inventory_reservations WHERE id=? FOR UPDATE", binding.ID).Scan(&r.ID, &r.InventoryItemID, &r.LocationID, &line, &r.Quantity, &r.AllowBackorder, &deleted)
			if errors.Is(err, sql.ErrNoRows) {
				return result, ErrReservationConflict
			}
			if err != nil {
				return result, ErrReservationStorage
			}
			r.LineItemID = line.String
			if !line.Valid || deleted.Valid || r.InventoryItemID != binding.InventoryItemID || r.LocationID != binding.LocationID || r.LineItemID != binding.LineItemID {
				return result, ErrReservationConflict
			}
			n, err := scaled(r.Quantity)
			if err != nil || n.Sign() <= 0 {
				return result, ErrStockHeld
			}
			p := reservationPair{r.InventoryItemID, r.LocationID}
			deltas[p].Sub(deltas[p], n)
			beforeReservations = append(beforeReservations, r)
		}
	}
	before := make([]reservationLevel, 0, len(ordered))
	after := make([]reservationLevel, 0, len(ordered))
	for _, p := range ordered {
		l := levels[p]
		before = append(before, l)
		reserved, err := scaled(l.Reserved)
		if err != nil {
			return result, ErrStockHeld
		}
		reserved.Add(reserved, deltas[p])
		if reserved.Sign() < 0 || !fitsQuantity(reserved) {
			return result, ErrStockHeld
		}
		updated := now
		if !updated.After(l.UpdatedAt) {
			updated = l.UpdatedAt.Add(time.Microsecond)
		}
		l.Reserved = fixed(reserved)
		l.UpdatedAt = updated
		raw, _ := json.Marshal(struct {
			Value     string `json:"value"`
			Precision int    `json:"precision"`
		}{l.Reserved, 38})
		if _, err = tx.ExecContext(ctx, "UPDATE inventory_levels SET reserved_quantity=?,raw_reserved_quantity=?,updated_at=? WHERE id=?", l.Reserved, string(raw), updated, l.ID); err != nil {
			return result, ErrReservationStorage
		}
		after = append(after, l)
	}
	if in.Kind == "reserve" {
		for _, r := range in.Reserve {
			raw, _ := json.Marshal(struct {
				Value     string `json:"value"`
				Precision int    `json:"precision"`
			}{r.Quantity, 38})
			if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_reservations(id,inventory_item_id,location_id,line_item_id,quantity,allow_backorder,raw_quantity,created_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, r.InventoryItemID, r.LocationID, r.LineItemID, r.Quantity, r.AllowBackorder, string(raw), in.ActorRef, now, now); err != nil {
				return result, reservationSQLError(err)
			}
		}
	} else {
		for _, binding := range in.Release {
			changed, err := tx.ExecContext(ctx, "UPDATE inventory_reservations SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL", now, now, binding.ID)
			if err != nil {
				return result, ErrReservationStorage
			}
			n, err := changed.RowsAffected()
			if err != nil || n != 1 {
				return result, ErrReservationConflict
			}
		}
	}
	b, _ := json.Marshal(struct {
		Levels       []reservationLevel    `json:"levels"`
		Reservations []reservationSnapshot `json:"reservations"`
	}{before, beforeReservations})
	a, _ := json.Marshal(struct {
		Levels       []reservationLevel   `json:"levels"`
		Reservations []ReservationInput   `json:"reservations"`
		Released     []ReservationBinding `json:"released"`
	}{after, in.Reserve, in.Release})
	receipt, _ := json.Marshal(result)
	if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_reservation_operations(operation_id,owner_id,actor_ref,kind,request_json,result_json,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, in.OperationID, in.OwnerID, in.ActorRef, in.Kind, string(request), string(receipt), string(b), string(a), now); err != nil {
		return result, reservationSQLError(err)
	}
	return result, nil
}

// Duplicate identities/operation IDs roll the entire caller transaction back;
// driver diagnostics (which may contain private identities) are never exposed.
func reservationSQLError(err error) error {
	var sqlErr *driver.MySQLError
	if errors.As(err, &sqlErr) && sqlErr.Number == 1062 {
		return ErrReservationConflict
	}
	return ErrReservationStorage
}
