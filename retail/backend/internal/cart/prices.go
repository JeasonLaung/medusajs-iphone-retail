package cart

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/pricing"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PriceRefreshResult struct {
	Repriced, Retained int
}

// RefreshItemPrices is the item-price phase of a caller-owned refresh
// transaction, not a complete refresh command. It rechecks current cart
// authority/version. Callers MUST roll back the whole transaction after any
// error, and provide durable idempotency, version increment, context changes,
// product snapshot preparation, taxes, promotions and shipping before commit.
// No inventory reservation, provider call or checkout authority is granted.
func RefreshItemPrices(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, at time.Time) (PriceRefreshResult, error) {
	if at.IsZero() {
		return PriceRefreshResult{}, ErrInput
	}
	cart, err := LockForWrite(ctx, tx, store, ref, version)
	if err != nil {
		return PriceRefreshResult{}, err
	}
	var lines []struct {
		ID                                        string
		VariantID                                 *string
		Quantity                                  int64
		UnitPrice, RawUnitPrice                   string
		CompareAtUnitPrice, RawCompareAtUnitPrice *string
		IsCustomPrice, IsTaxInclusive             bool
	}
	if tx.Table("cart_line_items").Select("id,variant_id,quantity,CAST(unit_price AS CHAR) AS unit_price,raw_unit_price,CAST(compare_at_unit_price AS CHAR) AS compare_at_unit_price,raw_compare_at_unit_price,is_custom_price,is_tax_inclusive").Where("cart_id=? AND deleted_at IS NULL", cart.ID).Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).Limit(1001).Scan(&lines).Error != nil || len(lines) > 1000 {
		return PriceRefreshResult{}, ErrSchema
	}
	variants := []string{}
	seen := map[string]bool{}
	groups := map[int64][]string{}
	groupSeen := map[int64]map[string]bool{}
	for _, line := range lines {
		if !idPattern.MatchString(line.ID) || line.Quantity < 1 || line.Quantity > 1000000000 {
			return PriceRefreshResult{}, ErrSchema
		}
		if line.VariantID == nil {
			continue
		}
		id := *line.VariantID
		if !idPattern.MatchString(id) {
			return PriceRefreshResult{}, ErrItemUnavailable
		}
		if !seen[id] {
			seen[id] = true
			variants = append(variants, id)
		}
		if groupSeen[line.Quantity] == nil {
			groupSeen[line.Quantity] = map[string]bool{}
		}
		if !groupSeen[line.Quantity][id] {
			groups[line.Quantity] = append(groups[line.Quantity], id)
			groupSeen[line.Quantity][id] = true
		}
	}
	// Even a custom-priced variant must still exist, be published and belong
	// to the cart's server-selected channel. Manual lines may have no variant.
	if _, err := catalog.ReadCartVariants(tx, variants, cart.SalesChannelID); err != nil {
		return PriceRefreshResult{}, ErrItemUnavailable
	}
	quantities := make([]int64, 0, len(groups))
	for q := range groups {
		quantities = append(quantities, q)
	}
	sort.Slice(quantities, func(i, j int) bool { return quantities[i] < quantities[j] })
	quotes := map[int64]map[string]*pricing.Quote{}
	for _, q := range quantities {
		quotes[q], err = pricing.Read(tx, groups[q], pricing.Context{Region: cart.RegionID, Currency: cart.CurrencyCode, Channel: cart.SalesChannelID, Quantity: strconv.FormatInt(q, 10)}, at.UTC())
		if err != nil {
			return PriceRefreshResult{}, ErrItemUnavailable
		}
	}
	type prepared struct {
		id, scalar, raw                    string
		compareScalar, compareRaw          *string
		reprice, compareChanged, inclusive bool
	}
	updates := make([]prepared, 0, len(lines))
	result := PriceRefreshResult{}
	for _, line := range lines {
		p := prepared{id: line.ID, inclusive: line.IsTaxInclusive}
		var quote *pricing.Quote
		if line.VariantID != nil {
			quote = quotes[line.Quantity][*line.VariantID]
		}
		p.reprice = line.VariantID != nil && !line.IsCustomPrice
		if p.reprice {
			if quote == nil {
				return PriceRefreshResult{}, ErrItemUnavailable
			}
			p.scalar, p.raw, err = sqlAmount(quote.Amount)
			if err != nil {
				return PriceRefreshResult{}, err
			}
			p.inclusive = quote.CalculatedTaxInclusive
			result.Repriced++
		} else {
			if _, err := nonNegativeAmount(line.UnitPrice, &line.RawUnitPrice); err != nil {
				return PriceRefreshResult{}, ErrSchema
			}
			result.Retained++
		}
		// The source preserves a supplied compare-at (including zero), even if
		// a later quote is not a sale. Only a missing value gets sale fallback.
		if line.CompareAtUnitPrice != nil {
			if _, err := nonNegativeAmount(*line.CompareAtUnitPrice, line.RawCompareAtUnitPrice); err != nil {
				return PriceRefreshResult{}, ErrSchema
			}
		} else {
			if line.RawCompareAtUnitPrice != nil {
				return PriceRefreshResult{}, ErrSchema
			}
			if quote != nil && quote.CalculatedKind.Type != nil && *quote.CalculatedKind.Type == "sale" && quote.OriginalAmount != nil && *quote.OriginalAmount != quote.Amount {
				a, b, err := sqlAmount(*quote.OriginalAmount)
				if err != nil {
					return PriceRefreshResult{}, err
				}
				p.compareScalar, p.compareRaw, p.compareChanged = &a, &b, true
			}
		}
		updates = append(updates, p)
	}
	for _, p := range updates {
		fields := map[string]any{}
		if p.reprice {
			fields["unit_price"], fields["raw_unit_price"], fields["is_tax_inclusive"] = p.scalar, p.raw, p.inclusive
		}
		if p.compareChanged {
			fields["compare_at_unit_price"], fields["raw_compare_at_unit_price"] = p.compareScalar, p.compareRaw
		}
		if len(fields) == 0 {
			continue
		}
		fields["updated_at"] = gorm.Expr("UTC_TIMESTAMP(6)")
		r := tx.Table("cart_line_items").Where("id=? AND cart_id=? AND deleted_at IS NULL", p.id, cart.ID).Updates(fields)
		if r.Error != nil || r.RowsAffected != 1 {
			return PriceRefreshResult{}, ErrStorage
		}
	}
	return result, nil
}
