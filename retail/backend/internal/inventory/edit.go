package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StockInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
	StockedQuantity  string `json:"stocked_quantity"`
	IncomingQuantity string `json:"incoming_quantity"`
}
type StockResult struct {
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}
type stockState struct {
	ID               string    `json:"id"`
	InventoryItemID  string    `json:"inventory_item_id"`
	LocationID       string    `json:"location_id"`
	StockedQuantity  string    `json:"stocked_quantity"`
	ReservedQuantity string    `json:"reserved_quantity"`
	IncomingQuantity string    `json:"incoming_quantity"`
	UpdatedAt        time.Time `json:"updated_at"`
}

var ErrStockConflict = errors.New("inventory stock edit conflict")
var ErrStockDenied = errors.New("inventory stock authority changed")
var ErrStockHeld = errors.New("inventory consumption requires reconciliation")
var errStockInput = errors.New("invalid inventory stock edit")

func stockRevision(s stockState) string {
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validStockInput(in StockInput) bool {
	id, err := uuid.Parse(in.OperationID)
	if err != nil || id.Version() != 4 || id.String() != in.OperationID {
		return false
	}
	rev, err := hex.DecodeString(in.ExpectedRevision)
	if err != nil || len(rev) != 32 || hex.EncodeToString(rev) != in.ExpectedRevision {
		return false
	}
	for _, s := range []string{in.StockedQuantity, in.IncomingQuantity} {
		// Canonical fixed scale avoids equivalent but different retry payloads.
		if !decimal.MatchString(s) || strings.HasPrefix(s, "-") {
			return false
		}
		raw, _ := json.Marshal(s)
		_, err := importvalue.Decimal38_18(raw)
		if err != nil {
			return false
		}
		n, err := scaled(s)
		if err != nil || fixed(n) != s {
			return false
		}
	}
	return true
}
func decodeStock(reader io.Reader) (StockInput, error) {
	var in StockInput
	d := json.NewDecoder(reader)
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return in, errStockInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return in, err
		}
		key, ok := tok.(string)
		if !ok || fields[key] != nil {
			return in, errStockInput
		}
		switch key {
		case "operation_id", "expected_revision", "stocked_quantity", "incoming_quantity":
		default:
			return in, errStockInput
		}
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return in, err
		}
		if _, err = importvalue.Text(raw, 80); err != nil {
			return in, errStockInput
		}
		fields[key] = raw
	}
	if _, err = d.Token(); err != nil {
		return in, err
	}
	if len(fields) != 4 {
		return in, errStockInput
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return in, errStockInput
	}
	b, _ := json.Marshal(fields)
	if json.Unmarshal(b, &in) != nil || !validStockInput(in) {
		return in, errStockInput
	}
	return in, nil
}

// EditStock sets absolute stock and incoming quantities. It never modifies
// reservations, serial devices, variant requirements or releases migration holds.
// Every future reservation/fulfillment command must lock the same level first.
func (h Handler) EditStock(ctx context.Context, levelID string, actor identity.User, in StockInput) (StockResult, error) {
	return h.editStock(ctx, levelID, "", actor, in)
}

func (h Handler) EditWarehouseStock(ctx context.Context, locationID, levelID string, actor identity.User, in StockInput) (StockResult, error) {
	if !validID(locationID) {
		return StockResult{}, errStockInput
	}
	return h.editStock(ctx, levelID, locationID, actor, in)
}

func (h Handler) editStock(ctx context.Context, levelID, locationID string, actor identity.User, in StockInput) (StockResult, error) {
	var result StockResult
	if !validID(levelID) || !validStockInput(in) {
		return result, errStockInput
	}
	if Ready(ctx, h.DB) != nil {
		return result, ErrSchema
	}
	request, _ := json.Marshal(in)
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		check := func() error {
			if locationID != "" {
				return authorization.CheckWarehouseTx(ctx, tx, actor, locationID, authorization.InventoryRead, authorization.StockWrite)
			}
			return authorization.CheckTx(ctx, tx, actor, authorization.InventoryRead, authorization.StockWrite)
		}
		if err := check(); err != nil {
			if errors.Is(err, authorization.ErrDenied) {
				if locationID != "" {
					return authorization.ErrDenied
				}
				return ErrStockDenied
			}
			return err
		}
		var err error

		var marker struct{ ID int }
		if tx.Table("inventory_runtime_schema").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("id=1").Take(&marker).Error != nil || Ready(ctx, tx) != nil {
			return ErrSchema
		}
		var cap struct {
			Version int
			Enabled bool
		}
		if tx.Table("inventory_command_capabilities").Clauses(clause.Locking{Strength: "SHARE"}).Where("command=?", "inventory.edit_stock").Take(&cap).Error != nil || cap.Version != 1 || !cap.Enabled {
			return ErrSchema
		}
		var before stockState
		levelQuery := tx.Table("inventory_levels").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,inventory_item_id,location_id,stocked_quantity,reserved_quantity,incoming_quantity,updated_at").Where("id=? AND deleted_at IS NULL", levelID)
		if locationID != "" {
			levelQuery = levelQuery.Where("location_id=?", locationID)
		}
		if err = levelQuery.Take(&before).Error; err != nil {
			if locationID != "" && errors.Is(err, gorm.ErrRecordNotFound) {
				return authorization.ErrDenied
			}
			return err
		}
		if locationID != "" && before.LocationID != locationID {
			return authorization.ErrDenied
		}
		// Parent locks serialize deletions; missing/deleted objects cannot be edited.
		for _, p := range []struct{ table, id string }{{"inventory_items", before.InventoryItemID}, {"inventory_locations", before.LocationID}} {
			var parent struct{ ID string }
			if err = tx.Table(p.table).Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Where("id=? AND deleted_at IS NULL", p.id).Take(&parent).Error; err != nil {
				return err
			}
		}
		var old struct{ ActorID, LevelID, RequestJSON, ResultRevision string }
		err = tx.Table("inventory_stock_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&old).Error
		if err == nil {
			if old.ActorID != actor.ID || old.LevelID != levelID || old.RequestJSON != string(request) {
				return ErrStockConflict
			}
			result = StockResult{old.ResultRevision, true}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if stockRevision(before) != in.ExpectedRevision {
			return ErrStockConflict
		}
		delta, err := Difference(in.StockedQuantity, before.StockedQuantity)
		if err != nil {
			return err
		}
		if strings.HasPrefix(delta, "-") {
			// Locking reads, including the no-hold gap, prevent stale RR snapshots.
			var holds []struct{ Reason string }
			if err = tx.Table("inventory_migration_holds").Clauses(clause.Locking{Strength: "SHARE"}).Select("reason").Where("inventory_item_id=? AND location_id=?", before.InventoryItemID, before.LocationID).Limit(4).Find(&holds).Error; err != nil {
				return err
			}
			if len(holds) > 0 {
				return ErrStockHeld
			}
			var reservations []struct{ Quantity string }
			if err = tx.Table("inventory_reservations").Clauses(clause.Locking{Strength: "UPDATE"}).Select("quantity").Where("inventory_item_id=? AND location_id=? AND deleted_at IS NULL", before.InventoryItemID, before.LocationID).Order("id").Limit(10001).Find(&reservations).Error; err != nil {
				return err
			}
			if len(reservations) > 10000 {
				return errLimit
			}
			total, _ := scaled("0")
			for _, r := range reservations {
				n, err := scaled(r.Quantity)
				if err != nil {
					return err
				}
				total.Add(total, n)
			}
			matches, err := Equal(before.ReservedQuantity, fixed(total))
			if err != nil {
				return err
			}
			if !matches {
				return ErrStockHeld
			}
			balance, err := Difference(before.StockedQuantity, before.ReservedQuantity)
			if err != nil {
				return err
			}
			if strings.HasPrefix(balance, "-") {
				return ErrStockHeld
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if !now.After(before.UpdatedAt) {
			now = before.UpdatedAt.Add(time.Microsecond)
		}
		after := before
		after.StockedQuantity = in.StockedQuantity
		after.IncomingQuantity = in.IncomingQuantity
		after.UpdatedAt = now
		// Keep the native raw/scalar pair exact for future imports and reconciliations.
		raw := func(value string) string {
			b, _ := json.Marshal(struct {
				Value     string `json:"value"`
				Precision int    `json:"precision"`
			}{value, 38})
			return string(b)
		}
		if err = tx.Table("inventory_levels").Where("id=?", levelID).Updates(map[string]any{"stocked_quantity": after.StockedQuantity, "incoming_quantity": after.IncomingQuantity, "raw_stocked_quantity": raw(after.StockedQuantity), "raw_incoming_quantity": raw(after.IncomingQuantity), "updated_at": now}).Error; err != nil {
			return err
		}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(after)
		result.Revision = stockRevision(after)
		err = tx.Table("inventory_stock_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "level_id": levelID, "request_json": string(request), "before_json": string(b), "after_json": string(a), "result_revision": result.Revision, "created_at": now}).Error
		var sqlErr *driver.MySQLError
		if errors.As(err, &sqlErr) && sqlErr.Number == 1062 {
			return ErrStockConflict
		}
		return err
	})
	return result, err
}
func (h Handler) UpdateStock(c *gin.Context) { h.updateStock(c, "") }
func (h Handler) UpdateWarehouseStock(c *gin.Context) {
	if !validID(c.Param("location_id")) {
		platform.Fail(c, 400, "inventory.invalid_edit", "Invalid warehouse")
		return
	}
	h.updateStock(c, c.Param("location_id"))
}
func (h Handler) updateStock(c *gin.Context, locationID string) {
	if !validID(c.Param("id")) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "inventory.invalid_edit", "Invalid stock edit")
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 400, "inventory.invalid_edit", "JSON stock edit required")
		return
	}
	in, err := decodeStock(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "inventory.invalid_edit", "Invalid stock edit")
		return
	}
	value, ok := c.Get("admin_user")
	actor, typed := value.(identity.User)
	if !ok || !typed {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	result, err := h.editStock(c.Request.Context(), c.Param("id"), locationID, actor, in)
	switch {
	case errors.Is(err, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Action not permitted")
	case errors.Is(err, ErrStockDenied):
		platform.Fail(c, 401, "auth.unauthorized", "Account changed; sign in again")
	case errors.Is(err, ErrStockConflict):
		platform.Fail(c, 409, "inventory.edit_conflict", "Stock or operation changed; reload before editing")
	case errors.Is(err, ErrStockHeld):
		platform.Fail(c, 409, "inventory.reconciliation_required", "Stock consumption requires reconciliation")
	case errors.Is(err, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "inventory.not_found", "Stock level or parent not found")
	case err != nil:
		platform.Fail(c, 503, "inventory.unavailable", "Stock edit unavailable; retry the same operation")
	default:
		c.JSON(200, result)
	}
}
