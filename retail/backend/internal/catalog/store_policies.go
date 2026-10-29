package catalog

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PolicyNote struct {
	EN string `json:"en"`
	ZH string `json:"zh"`
}
type BuyPolicies struct {
	WarrantyMonths int        `json:"warranty_months"`
	ReturnDays     int        `json:"return_days"`
	WarrantyNote   PolicyNote `json:"warranty_note"`
	ReturnNote     PolicyNote `json:"return_note"`
}

func defaultBuyPolicies() BuyPolicies { return BuyPolicies{WarrantyMonths: 24, ReturnDays: 14} }

// Match the original whole-object validation and 24-month/14-day fallback.
// Missing or malformed settings never write defaults into persistent metadata.
func buyPolicies(raw *string) (BuyPolicies, error) {
	fallback := defaultBuyPolicies()
	m, e := displayJSON(raw)
	if e != nil {
		return BuyPolicies{}, e
	}
	value, ok := m["buy_policies"].(map[string]any)
	if !ok || len(value) != 4 {
		return fallback, nil
	}
	integer := func(key string, max int) (int, bool) {
		v, ok := value[key].(json.Number)
		if !ok {
			return 0, false
		}
		s, err := totals.Resolve(totals.Number{Decimal: string(v)})
		if err != nil {
			return 0, false
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return int(i), i >= 0 && i <= int64(max)
	}
	note := func(key string) (PolicyNote, bool) {
		v, ok := value[key].(map[string]any)
		if !ok {
			return PolicyNote{}, false
		}
		en, okEN := v["en"].(string)
		zh, okZH := v["zh"].(string)
		const jsWhitespace = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"
		en = strings.Trim(en, jsWhitespace)
		zh = strings.Trim(zh, jsWhitespace)
		if !okEN || !okZH || len(utf16.Encode([]rune(en))) > 200 || len(utf16.Encode([]rune(zh))) > 200 {
			return PolicyNote{}, false
		}
		return PolicyNote{en, zh}, true
	}
	months, okM := integer("warranty_months", 120)
	days, okD := integer("return_days", 365)
	warranty, okW := note("warranty_note")
	returns, okR := note("return_note")
	if !okM || !okD || !okW || !okR {
		return fallback, nil
	}
	return BuyPolicies{months, days, warranty, returns}, nil
}

func (h StoreHandler) Policies(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "policies.invalid_query", "Invalid policy request")
		return
	}
	if h.DB == nil || !validProductID(h.StoreID) {
		platform.Fail(c, 503, "policies.unavailable", "Policies unavailable")
		return
	}
	var policies BuyPolicies
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if e := Ready(c.Request.Context(), tx); e != nil {
			return e
		}
		var row struct {
			Metadata *string
			Bytes    int
		}
		result := tx.Raw("SELECT LEFT(metadata,65537) AS metadata,COALESCE(OCTET_LENGTH(metadata),0) AS bytes FROM commerce_stores WHERE id=? AND deleted_at IS NULL", h.StoreID).Scan(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 || row.Bytes > 65536 {
			return errStoreScope
		}
		var e error
		policies, e = buyPolicies(row.Metadata)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		platform.Fail(c, 503, "policies.unavailable", "Policies unavailable")
		return
	}
	c.JSON(200, gin.H{"policies": policies})
}
