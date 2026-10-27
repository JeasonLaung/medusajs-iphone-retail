package catalog

import (
	"database/sql"
	"errors"
	"net/url"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// These are configured prices for staff inspection, never a calculated quote.
// Decimal columns scan directly into strings: no float or currency scaling.
type AdminPriceRule struct {
	ID        string `json:"id"`
	Attribute string `json:"attribute"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	Priority  int    `json:"priority"`
}
type AdminPriceListRule struct {
	ID        string  `json:"id"`
	Attribute string  `json:"attribute"`
	Value     *string `json:"value"`
}
type AdminPriceList struct {
	ID         string               `json:"id"`
	Title      string               `json:"title"`
	Status     string               `json:"status"`
	Type       string               `json:"type"`
	StartsAt   *time.Time           `json:"starts_at"`
	EndsAt     *time.Time           `json:"ends_at"`
	RulesCount *int                 `json:"rules_count"`
	Rules      []AdminPriceListRule `json:"rules" gorm:"-"`
}
type AdminConfiguredPrice struct {
	ID           string           `json:"id"`
	PriceSetID   string           `json:"price_set_id"`
	CurrencyCode string           `json:"currency_code"`
	Amount       string           `json:"amount"`
	MinQuantity  *string          `json:"min_quantity"`
	MaxQuantity  *string          `json:"max_quantity"`
	RulesCount   *int             `json:"rules_count"`
	PriceListID  *string          `json:"price_list_id"`
	PriceList    *AdminPriceList  `json:"price_list" gorm:"-"`
	Rules        []AdminPriceRule `json:"rules" gorm:"-"`
}

func parsePricePage(values url.Values) (adminListQuery, error) {
	for key := range values {
		if key != "limit" && key != "offset" {
			return adminListQuery{}, errListQuery
		}
	}
	return parseAdminList(values)
}

func (h Handler) AdminVariantPrices(c *gin.Context) {
	id, variantID := c.Param("id"), c.Param("variant_id")
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	q, queryErr := parsePricePage(values)
	if err != nil || queryErr != nil || !validProductID(id) || !validProductID(variantID) {
		platform.Fail(c, 400, "catalog.invalid_query", "Invalid variant price query")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "catalog.unavailable", "Catalog unavailable")
		return
	}
	prices := make([]AdminConfiguredPrice, 0)
	sets := make([]string, 0)
	var count int64
	err = h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var variant struct{ ID string }
		// The product path is an authority boundary, not an optional filter.
		if err := tx.Table("catalog_variants v").Select("v.id").Joins("JOIN catalog_products p ON p.id=v.product_id AND p.deleted_at IS NULL").Where("v.id=? AND v.product_id=? AND v.deleted_at IS NULL", variantID, id).Take(&variant).Error; err != nil {
			return err
		}
		if err := tx.Table("catalog_variant_price_sets vp").Select("vp.price_set_id").Joins("JOIN pricing_price_sets s ON s.id=vp.price_set_id AND s.deleted_at IS NULL").Where("vp.variant_id=? AND vp.deleted_at IS NULL", variantID).Order("vp.price_set_id ASC").Limit(1001).Scan(&sets).Error; err != nil {
			return err
		}
		if len(sets) > 1000 {
			return errDetailLimit
		}
		if len(sets) == 0 {
			return nil
		}
		base := func() *gorm.DB {
			return tx.Table("pricing_prices").Where("price_set_id IN ? AND deleted_at IS NULL", sets)
		}
		if err := base().Count(&count).Error; err != nil {
			return err
		}
		if err := base().Select("id,price_set_id,currency_code,amount,min_quantity,max_quantity,rules_count,price_list_id").Order("currency_code ASC,price_set_id ASC,id ASC").Limit(q.Limit).Offset(q.Offset).Scan(&prices).Error; err != nil {
			return err
		}
		if len(prices) == 0 {
			return nil
		}
		ids := make([]string, 0, len(prices))
		positions := map[string]int{}
		listIDs := make([]string, 0)
		for i := range prices {
			p := &prices[i]
			ids = append(ids, p.ID)
			positions[p.ID] = i
			p.Rules = []AdminPriceRule{}
			if p.PriceListID != nil {
				listIDs = append(listIDs, *p.PriceListID)
			}
		}
		var rules []struct {
			AdminPriceRule
			PriceID string
		}
		if err := tx.Table("pricing_price_rules").Select("id,attribute,operator,value,priority,price_id").Where("price_id IN ? AND deleted_at IS NULL", ids).Order("priority DESC,id ASC").Limit(10001).Scan(&rules).Error; err != nil {
			return err
		}
		if len(rules) > 10000 {
			return errDetailLimit
		}
		for _, r := range rules {
			i := positions[r.PriceID]
			prices[i].Rules = append(prices[i].Rules, r.AdminPriceRule)
		}
		if len(listIDs) == 0 {
			return nil
		}
		var lists []AdminPriceList
		if err := tx.Table("pricing_price_lists").Select("id,title,status,type,starts_at,ends_at,rules_count").Where("id IN ? AND deleted_at IS NULL", listIDs).Scan(&lists).Error; err != nil {
			return err
		}
		listIndex := map[string]*AdminPriceList{}
		for i := range lists {
			lists[i].Rules = []AdminPriceListRule{}
			listIndex[lists[i].ID] = &lists[i]
		}
		// Fetch rules only for visible lists; retain a price's unavailable list ID.
		activeListIDs := make([]string, 0, len(lists))
		for _, l := range lists {
			activeListIDs = append(activeListIDs, l.ID)
		}
		if len(activeListIDs) > 0 {
			var listRules []struct {
				AdminPriceListRule
				PriceListID string
			}
			if err := tx.Table("pricing_price_list_rules").Select("id,attribute,value,price_list_id").Where("price_list_id IN ? AND deleted_at IS NULL", activeListIDs).Order("id ASC").Limit(10001).Scan(&listRules).Error; err != nil {
				return err
			}
			if len(listRules) > 10000 {
				return errDetailLimit
			}
			for _, r := range listRules {
				listIndex[r.PriceListID].Rules = append(listIndex[r.PriceListID].Rules, r.AdminPriceListRule)
			}
		}
		for i := range prices {
			if prices[i].PriceListID != nil {
				prices[i].PriceList = listIndex[*prices[i].PriceListID]
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "catalog.not_found", "Product variant not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "catalog.unavailable", "Variant prices unavailable")
		return
	}
	c.JSON(200, gin.H{"product_id": id, "variant_id": variantID, "price_set_ids": sets, "prices": prices, "count": count, "limit": q.Limit, "offset": q.Offset})
}
