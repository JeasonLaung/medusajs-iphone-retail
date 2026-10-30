package buyback

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/pricing"
	"gorm.io/gorm"
)

type Store struct {
	DB      *gorm.DB
	StoreID string
}
type Price struct {
	ProductID    string      `json:"product_id"`
	Capacity     string      `json:"capacity"`
	Amount       json.Number `json:"amount"`
	CurrencyCode string      `json:"currency_code"`
}
type priceRecord struct {
	ProductID, Capacity, Amount, RawAmount, CurrencyCode string
	PricingRules                                         *string
	CapacityBytes, RawBytes, RulesBytes                  int
}

func validID(s string, max int) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func (s Store) read(ctx context.Context, fn func(*gorm.DB) error) error {
	if s.DB == nil || !validID(s.StoreID, 191) {
		return ErrInput
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if catalog.Ready(ctx, tx) != nil {
			return ErrInput
		}
		var count int64
		if tx.Table("commerce_stores").Where("id=? AND deleted_at IS NULL", s.StoreID).Count(&count).Error != nil || count != 1 {
			return ErrInput
		}
		return fn(tx)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

const priceFields = `product_id,LEFT(capacity,257) AS capacity,OCTET_LENGTH(capacity) AS capacity_bytes,CAST(amount AS CHAR) AS amount,LEFT(raw_amount,4097) AS raw_amount,OCTET_LENGTH(raw_amount) AS raw_bytes,LEFT(pricing_rules,16385) AS pricing_rules,COALESCE(OCTET_LENGTH(pricing_rules),0) AS rules_bytes,currency_code`

func projectPrice(row priceRecord) (Price, error) {
	if !validID(row.ProductID, 191) || row.CapacityBytes > 256 || row.CapacityBytes != len(row.Capacity) || row.RawBytes > 4096 || row.RulesBytes > 16384 || !validID(row.CurrencyCode, 16) {
		return Price{}, ErrInput
	}
	amount, e := pricing.ConfiguredAmount(row.Amount, row.RawAmount)
	if e != nil || strings.HasPrefix(amount, "-") {
		return Price{}, ErrInput
	}
	return Price{row.ProductID, row.Capacity, json.Number(amount), row.CurrencyCode}, nil
}
func (s Store) Prices(ctx context.Context, product string) ([]Price, error) {
	out := []Price{}
	if product != "" && !validID(product, 100) {
		return nil, ErrInput
	}
	e := s.read(ctx, func(tx *gorm.DB) error {
		if product == "" {
			return nil
		}
		var rows []priceRecord
		if tx.Table("buyback_prices").Select(priceFields).Where("product_id=? AND deleted_at IS NULL", product).Order("created_at DESC,id DESC").Limit(100).Scan(&rows).Error != nil {
			return ErrInput
		}
		for _, row := range rows {
			price, e := projectPrice(row)
			if e != nil {
				return e
			}
			out = append(out, price)
		}
		return nil
	})
	return out, e
}
func (s Store) Estimate(ctx context.Context, in EstimateInput) (Estimate, error) {
	var out Estimate
	if !validID(in.ProductID, 100) || !capacityPattern.MatchString(in.Capacity) || len(in.Capacity) > 256 || in.Assessment.ValidatePartial() != nil {
		return out, ErrInput
	}
	e := s.read(ctx, func(tx *gorm.DB) error {
		var e error
		out, e = estimateInTransaction(tx, in.ProductID, in.Capacity, in.Assessment)
		return e
	})
	return out, e
}

// Reuse the caller's transaction when capturing prices for a new request.
func estimateInTransaction(tx *gorm.DB, productID, inputCapacity string, assessment Assessment) (Estimate, error) {
	capacity := NormalizeCapacity(inputCapacity)
	var rows []priceRecord
	if tx.Table("buyback_prices").Select(priceFields).Where("product_id=? AND active_capacity_digest=UNHEX(SHA2(?,256)) AND BINARY capacity=BINARY ? AND deleted_at IS NULL", productID, capacity, capacity).Limit(2).Scan(&rows).Error != nil || len(rows) > 1 {
		return Estimate{}, ErrInput
	}
	var rules *Rules
	var legacy *json.Number
	if len(rows) == 1 {
		price, e := projectPrice(rows[0])
		if e != nil {
			return Estimate{}, e
		}
		legacy = &price.Amount
		if rows[0].PricingRules != nil && strings.TrimSpace(*rows[0].PricingRules) != "null" {
			parsed, e := ParseRules([]byte(*rows[0].PricingRules))
			if e != nil {
				return Estimate{}, e
			}
			rules = &parsed
		}
	}
	return Assess(rules, assessment, legacy)
}
