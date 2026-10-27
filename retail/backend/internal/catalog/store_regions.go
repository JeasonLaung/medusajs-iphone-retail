package catalog

import (
	"database/sql"
	"errors"
	"regexp"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Public routing data is projected from the reviewed relational schema, never
// from a source payload, metadata, payment-provider configuration or a cookie.
type StoreCountry struct {
	ISO2        string `json:"iso_2" gorm:"column:iso_2"`
	ISO3        string `json:"iso_3" gorm:"column:iso_3"`
	NumCode     string `json:"num_code"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	RegionID    string `json:"region_id"`
}
type StoreRegion struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	CurrencyCode   string         `json:"currency_code"`
	AutomaticTaxes bool           `json:"automatic_taxes"`
	Countries      []StoreCountry `json:"countries" gorm:"-"`
}

var regionCurrency = regexp.MustCompile(`^[a-z]{3}$`)
var regionCountry = regexp.MustCompile(`^[a-z]{2}$`)
var errRegionLimit = errors.New("region routing model unavailable")

func (h Handler) Regions(c *gin.Context)     { h.storeRegions(c, false) }
func (h Handler) StoreRegion(c *gin.Context) { h.storeRegions(c, true) }
func (h Handler) storeRegions(c *gin.Context, detail bool) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" || detail && !validProductID(c.Param("id")) {
		platform.Fail(c, 400, "regions.invalid_query", "Invalid region request")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "regions.unavailable", "Regions unavailable")
		return
	}
	regions := []StoreRegion{}
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := Ready(c.Request.Context(), tx); err != nil {
			return err
		}
		q := tx.Table("commerce_regions").Select("id,name,currency_code,automatic_taxes").Where("deleted_at IS NULL")
		if detail {
			q = q.Where("id=?", c.Param("id"))
		}
		if err := q.Order("id ASC").Limit(101).Scan(&regions).Error; err != nil {
			return err
		}
		if len(regions) > 100 {
			return errRegionLimit
		}
		if len(regions) == 0 {
			if detail {
				return gorm.ErrRecordNotFound
			}
			return nil
		}
		index := map[string]int{}
		ids := make([]string, 0, len(regions))
		for i := range regions {
			r := &regions[i]
			if !regionCurrency.MatchString(r.CurrencyCode) {
				return errRegionLimit
			}
			r.Countries = []StoreCountry{}
			index[r.ID] = i
			ids = append(ids, r.ID)
		}
		countries := []StoreCountry{}
		if err := tx.Table("commerce_countries").Select("iso_2,iso_3,num_code,name,display_name,region_id").Where("deleted_at IS NULL AND region_id IN ?", ids).Order("region_id ASC,iso_2 ASC").Limit(1001).Scan(&countries).Error; err != nil {
			return err
		}
		if len(countries) > 1000 {
			return errRegionLimit
		}
		for _, country := range countries {
			i, ok := index[country.RegionID]
			if !ok || !regionCountry.MatchString(country.ISO2) {
				return errRegionLimit
			}
			regions[i].Countries = append(regions[i].Countries, country)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "regions.not_found", "Region not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "regions.unavailable", "Regions unavailable")
		return
	}
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	if detail {
		c.JSON(200, gin.H{"region": regions[0]})
	} else {
		c.JSON(200, gin.H{"regions": regions, "count": len(regions)})
	}
}
