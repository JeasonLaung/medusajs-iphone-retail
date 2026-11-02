// Package seo publishes only the original storefront SEO projection.
// Imported merchant settings remain held; provider credentials are never read.
package seo

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var ErrUnavailable = errors.New("SEO publication unavailable")
var ErrInput = errors.New("invalid SEO path")

var publicKeys = []string{"seo.site_url", "seo.site_name", "seo.home_path", "seo.default_title", "seo.default_description", "seo.robots_txt", "seo.llms_txt", "seo.indexnow_enabled", "seo.indexnow_key", "bing.verification_enabled", "bing.site_auth_xml", "analytics.enabled", "ga4.measurement_id", "seo.google_search_console.enabled", "gsc.verification_code"}

type Store struct {
	DB     *gorm.DB
	Ring   *protected.Ring
	Origin string
}
type Page struct {
	Path           string          `json:"path"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Canonical      string          `json:"canonical"`
	OGImage        string          `json:"og_image"`
	Hreflang       json.RawMessage `json:"hreflang"`
	StructuredData json.RawMessage `json:"structured_data"`
	// Preserve JSON number tokens across JavaScript API decoding.
	StructuredDataJSON string `json:"structured_data_json" gorm:"-"`
	Robots             string `json:"robots"`
}
type Config struct {
	Settings map[string]string `json:"settings"`
	Page     *Page             `json:"page"`
}
type DiscoveryPage struct {
	Path      string          `json:"path"`
	Robots    string          `json:"robots"`
	Canonical string          `json:"canonical"`
	Hreflang  json.RawMessage `json:"hreflang"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type setting struct {
	ID, Key string
	Value   []byte
}

func NormalizePath(value string) (string, error) {
	if !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", ErrInput
	}
	for _, r := range value {
		if r < 32 || strings.ContainsRune("?#\\", r) {
			return "", ErrInput
		}
	}
	if value == "/" {
		return value, nil
	}
	return strings.TrimRight(value, "/"), nil
}
func siteOrigin(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ErrUnavailable
	}
	return u.Scheme + "://" + u.Host, nil
}
func (s Store) Config(ctx context.Context, path string) (Config, error) {
	path, err := NormalizePath(path)
	if err != nil {
		return Config{}, err
	}
	result := Config{Settings: map[string]string{}}
	if s.DB == nil {
		return Config{}, ErrUnavailable
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []setting
		// Full binary key matching excludes case/Unicode lookalikes. Holds and
		// secret flags are filtered BEFORE opening any encrypted value.
		if tx.Table("seo_settings AS s").Select("s.id,s.`key`,s.value").Where("s.deleted_at IS NULL AND s.is_secret=FALSE AND s.`key` IN ?", publicKeys).Where("NOT EXISTS (SELECT 1 FROM migration_merchant_dispositions d WHERE d.source_table=? AND d.source_id=CAST(s.id AS BINARY))", "seo_setting").Find(&rows).Error != nil {
			return ErrUnavailable
		}
		for _, row := range rows {
			if _, exists := result.Settings[row.Key]; exists || s.Ring == nil {
				return ErrUnavailable
			}
			plain, e := s.Ring.Open(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: row.ID}, row.Value)
			if e != nil || !utf8.Valid(plain) {
				clear(plain)
				return ErrUnavailable
			}
			result.Settings[row.Key] = string(plain)
			clear(plain)
		}
		var pages []Page
		if tx.Table("seo_pages").Select("path,title,description,canonical,og_image,hreflang,structured_data,robots").Where("path=? AND deleted_at IS NULL", path).Limit(2).Find(&pages).Error != nil || len(pages) > 1 {
			return ErrUnavailable
		}
		if len(pages) == 1 {
			result.Page = &pages[0]
			if result.Page.Hreflang == nil {
				result.Page.Hreflang = json.RawMessage("null")
			}
			if result.Page.StructuredData == nil {
				result.Page.StructuredData = json.RawMessage("null")
			}
			result.Page.StructuredDataJSON = string(result.Page.StructuredData)
		}
		return nil
	})
	if err != nil {
		return Config{}, ErrUnavailable
	}
	origin := result.Settings["seo.site_url"]
	if origin == "" {
		origin = s.Origin
	}
	// Like the source storefront's local fallback (localhost:8000), the
	// migrated storefront has a default local origin. SEO must not require
	// enabling authenticated commerce configuration on a read-only preview.
	if origin == "" {
		origin = "http://localhost:9300"
	}
	origin, err = siteOrigin(origin)
	if err != nil {
		return Config{}, ErrUnavailable
	}
	result.Settings["seo.site_url"] = origin
	return result, nil
}
func (s Store) Pages(ctx context.Context) ([]DiscoveryPage, error) {
	if s.DB == nil {
		return nil, ErrUnavailable
	}
	rows := []DiscoveryPage{}
	if s.DB.WithContext(ctx).Table("seo_pages").Select("path,robots,canonical,hreflang,updated_at").Where("deleted_at IS NULL").Order("created_at ASC,id ASC").Find(&rows).Error != nil {
		return nil, ErrUnavailable
	}
	seen := map[string]bool{}
	for i := range rows {
		if seen[rows[i].Path] {
			return nil, ErrUnavailable
		}
		seen[rows[i].Path] = true
		if rows[i].Hreflang == nil {
			rows[i].Hreflang = json.RawMessage("null")
		}
	}
	return rows, nil
}
func (s Store) Register(group *gin.RouterGroup) {
	group.GET("/seo", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		path := c.DefaultQuery("path", "/")
		if path == "" {
			path = "/"
		}
		result, err := s.Config(c.Request.Context(), path)
		if errors.Is(err, ErrInput) {
			platform.Fail(c, 400, "seo.invalid_path", "Invalid SEO path")
			return
		}
		if err != nil {
			platform.Fail(c, 503, "seo.unavailable", "SEO publication unavailable")
			return
		}
		c.JSON(200, result)
	})
	group.GET("/seo/pages", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		rows, err := s.Pages(c.Request.Context())
		if err != nil {
			platform.Fail(c, 503, "seo.unavailable", "SEO publication unavailable")
			return
		}
		c.JSON(200, gin.H{"pages": rows})
	})
}
