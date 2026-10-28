package catalog

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Original translation-module default names, not business/account data.
//
//go:embed default_locales.json
var defaultLocalesJSON []byte
var localeCode = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

type StoreLocale struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (h StoreHandler) Locales(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "locales.invalid_query", "Invalid locale request")
		return
	}
	if h.DB == nil || !validProductID(h.StoreID) {
		platform.Fail(c, 503, "locales.unavailable", "Locales unavailable")
		return
	}
	locales := []StoreLocale{}
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := Ready(c.Request.Context(), tx); err != nil {
			return err
		}
		var store string
		r := tx.Table("commerce_stores").Select("id").Where("id=? AND deleted_at IS NULL", h.StoreID).Scan(&store)
		if r.Error != nil || r.RowsAffected != 1 {
			return errStoreScope
		}
		// Read only the bounded public code; oversized LONGTEXT must not enter Go.
		var rows []struct {
			Code  string
			Bytes int
		}
		if tx.Raw(`SELECT LEFT(locale_code,36) AS code,OCTET_LENGTH(locale_code) AS bytes FROM commerce_store_locales WHERE store_id=? AND deleted_at IS NULL ORDER BY id LIMIT 101`, h.StoreID).Scan(&rows).Error != nil || len(rows) > 100 {
			return errStoreScope
		}
		names := map[string]string{}
		if json.Unmarshal(defaultLocalesJSON, &names) != nil || len(names) != 51 {
			return errors.New("locale name directory unavailable")
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if row.Bytes < 2 || row.Bytes > 35 || len(row.Code) != row.Bytes || !localeCode.MatchString(row.Code) {
				return errStoreScope
			}
			key := strings.ToLower(row.Code)
			if seen[key] {
				return errStoreScope
			}
			seen[key] = true
			name := ""
			for code, value := range names {
				if strings.EqualFold(code, row.Code) {
					name = value
					break
				}
			}
			// No invented names for custom translation registrations absent from the
			// reviewed schema. Their persisted directory remains an explicit task.
			if name == "" {
				return errStoreScope
			}
			locales = append(locales, StoreLocale{Code: row.Code, Name: name})
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		platform.Fail(c, 503, "locales.unavailable", "Locales unavailable")
		return
	}
	c.Header("Cache-Control", "public, max-age=0, must-revalidate")
	c.JSON(200, gin.H{"locales": locales, "count": len(locales)})
}
