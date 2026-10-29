package orders

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNumbering = errors.New("order numbering unavailable")
var ErrNumberSeed = errors.New("order numbering seed conflicts with history")

const maxDisplayID int64 = 2147483647
const ticksPerSecond int64 = 100000

var orderNumberZone = time.FixedZone("Asia/Shanghai", 8*60*60)

// OrderNumberSeed is an internal cutover input, never an HTTP request. The
// operator must freeze source writes and review sequence is_called, ALL imported
// rows (including deleted rows), and the source clock before supplying it.
// EvidenceSHA256 identifies that reviewed snapshot, not a credential.
type OrderNumberSeed struct {
	LastDisplayID  int64
	LastTick       int64
	EvidenceSHA256 [32]byte
}

type OrderNumbers struct {
	DisplayID       int64
	CustomDisplayID string
}

type numberAllocator struct {
	Version                 int
	Initialized             bool
	LastDisplayID, LastTick int64
	SeedDisplayID, SeedTick *int64
	SeedSHA256              []byte
}

func numberTransaction(ctx context.Context, tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil {
		return ErrNumbering
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrNumbering
	}
	if Ready(ctx, tx) != nil {
		return ErrNumbering
	}
	return nil
}

func lockNumberAllocator(ctx context.Context, tx *gorm.DB) (numberAllocator, error) {
	if err := numberTransaction(ctx, tx); err != nil {
		return numberAllocator{}, err
	}
	var row numberAllocator
	if tx.WithContext(ctx).Table("order_number_allocator").Where("id=1").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error != nil || row.Version != 1 {
		return numberAllocator{}, ErrNumbering
	}
	return row, nil
}

// SeedOrderNumbers initializes once, in an explicit caller-owned transaction.
// An exact retry is harmless even after allocation; it never resets counters.
// Imported order rows are not changed. New imports must precede this cutover.
func SeedOrderNumbers(ctx context.Context, tx *gorm.DB, seed OrderNumberSeed) error {
	if seed.LastDisplayID < 0 || seed.LastDisplayID > maxDisplayID || seed.LastTick < 0 || seed.EvidenceSHA256 == ([32]byte{}) {
		return ErrNumberSeed
	}
	if _, err := customOrderNumber(seed.LastTick); err != nil {
		return ErrNumberSeed
	}
	row, err := lockNumberAllocator(ctx, tx)
	if err != nil {
		return err
	}
	if row.Initialized {
		if row.SeedDisplayID == nil || row.SeedTick == nil || *row.SeedDisplayID != seed.LastDisplayID || *row.SeedTick != seed.LastTick || subtle.ConstantTimeCompare(row.SeedSHA256, seed.EvidenceSHA256[:]) != 1 {
			return ErrNumberSeed
		}
		return nil
	}
	// Check persisted history as well as the supplied source high water marks.
	// Source sequences can be ahead of MAX after deletes/rolled-back inserts;
	// that is why MAX alone must never initialize the allocator.
	var display int64
	if tx.WithContext(ctx).Raw("SELECT COALESCE(MAX(display_id),0) FROM orders").Scan(&display).Error != nil || display < 0 || display > seed.LastDisplayID {
		return ErrNumberSeed
	}
	var custom []string
	if tx.Table("orders").Distinct("custom_display_id").Where("custom_display_id IS NOT NULL").Order("custom_display_id").Limit(1000001).Pluck("custom_display_id", &custom).Error != nil || len(custom) > 1000000 {
		return ErrNumberSeed
	}
	for _, value := range custom {
		tick, err := parseCustomOrderNumber(value)
		if err != nil || tick > seed.LastTick {
			return ErrNumberSeed
		}
	}
	r := tx.WithContext(ctx).Table("order_number_allocator").Where("id=1 AND initialized=FALSE").Updates(map[string]any{
		"initialized": true, "last_display_id": seed.LastDisplayID, "last_tick": seed.LastTick,
		"seed_display_id": seed.LastDisplayID, "seed_tick": seed.LastTick, "seed_sha256": seed.EvidenceSHA256[:], "updated_at": gorm.Expr("UTC_TIMESTAMP(6)"),
	})
	if r.Error != nil || r.RowsAffected != 1 {
		return ErrNumbering
	}
	return nil
}

// AllocateOrderNumbers participates in the order transaction. No commit, Redis
// lock, event or provider call occurs here. The owning workflow must check its
// idempotency receipt BEFORE allocating, and persist both numbers with the order
// before commit. Rollback rolls back these table counters too; an uncommitted
// number must never be published externally. Gaps are not a business guarantee.
func AllocateOrderNumbers(ctx context.Context, tx *gorm.DB) (OrderNumbers, error) {
	row, err := lockNumberAllocator(ctx, tx)
	if err != nil {
		return OrderNumbers{}, err
	}
	if !row.Initialized || row.LastDisplayID < 0 || row.LastDisplayID >= maxDisplayID || row.LastTick < 0 || row.LastTick == int64(^uint64(0)>>1) {
		return OrderNumbers{}, ErrNumbering
	}
	// Database UTC clock, integer microseconds / 10: no floating point or session
	// time-zone dependent conversion, matching the original ten-microsecond tick.
	var now int64
	if tx.WithContext(ctx).Raw("SELECT TIMESTAMPDIFF(MICROSECOND,'1970-01-01 00:00:00',UTC_TIMESTAMP(6)) DIV 10").Scan(&now).Error != nil || now < 0 {
		return OrderNumbers{}, ErrNumbering
	}
	tick := row.LastTick + 1
	if now > tick {
		tick = now
	}
	custom, err := customOrderNumber(tick)
	if err != nil {
		return OrderNumbers{}, err
	}
	r := tx.WithContext(ctx).Table("order_number_allocator").Where("id=1 AND initialized=TRUE").Updates(map[string]any{"last_display_id": row.LastDisplayID + 1, "last_tick": tick, "updated_at": gorm.Expr("UTC_TIMESTAMP(6)")})
	if r.Error != nil || r.RowsAffected != 1 {
		return OrderNumbers{}, ErrNumbering
	}
	return OrderNumbers{DisplayID: row.LastDisplayID + 1, CustomDisplayID: custom}, nil
}

func customOrderNumber(tick int64) (string, error) {
	if tick < 0 {
		return "", ErrNumbering
	}
	at := time.Unix(tick/ticksPerSecond, (tick%ticksPerSecond)*10000).In(orderNumberZone)
	if at.Year() < 1970 || at.Year() > 9999 {
		return "", ErrNumbering
	}
	return at.Format("20060102150405") + fmt.Sprintf("%05d", tick%ticksPerSecond), nil
}

func parseCustomOrderNumber(value string) (int64, error) {
	if len(value) != 19 {
		return 0, ErrNumberSeed
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, ErrNumberSeed
		}
	}
	at, err := time.ParseInLocation("20060102150405", value[:14], orderNumberZone)
	if err != nil || at.Year() < 1970 {
		return 0, ErrNumberSeed
	}
	tail, err := strconv.ParseInt(value[14:], 10, 64)
	if err != nil {
		return 0, ErrNumberSeed
	}
	tick := at.Unix()*ticksPerSecond + tail
	canonical, err := customOrderNumber(tick)
	if err != nil || canonical != value {
		return 0, ErrNumberSeed
	}
	return tick, nil
}
