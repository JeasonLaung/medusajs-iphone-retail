package catalog

import (
	"database/sql"
	"encoding/json"
	"net/mail"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ServiceContact struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	WhatsApp     string `json:"whatsapp"`
	Email        string `json:"email"`
	Timezone     string `json:"timezone"`
	Days         []int  `json:"days"`
	Start        string `json:"start"`
	End          string `json:"end"`
	Availability string `json:"availability"`
}

var servicePhone = regexp.MustCompile(`^\+?[1-9][0-9]{6,14}$`)
var serviceTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Match the source settings' whole-list validation: malformed configuration
// yields no published contacts. Unknown metadata is never a public response.
func serviceContacts(raw *string) ([]ServiceContact, error) {
	empty := []ServiceContact{}
	metadata, err := displayJSON(raw)
	if err != nil {
		return nil, err
	}
	value, exists := metadata["customer_service_contacts"]
	if !exists {
		return empty, nil
	}
	rows, ok := value.([]any)
	if !ok || len(rows) > 100 {
		return empty, nil
	}
	result := []ServiceContact{}
	seen := map[string]bool{}
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			return empty, nil
		}
		field := func(key string) (string, bool) { s, ok := row[key].(string); return s, ok }
		var out ServiceContact
		fields := []struct {
			key  string
			dest *string
		}{{"id", &out.ID}, {"name", &out.Name}, {"department", &out.Department}, {"whatsapp", &out.WhatsApp}, {"email", &out.Email}, {"timezone", &out.Timezone}, {"start", &out.Start}, {"end", &out.End}, {"availability", &out.Availability}}
		for _, f := range fields {
			s, ok := field(f.key)
			if !ok {
				return empty, nil
			}
			*f.dest = s
		}
		out.Name = strings.TrimSpace(out.Name)
		out.WhatsApp = strings.TrimSpace(out.WhatsApp)
		enabled, ok := row["enabled"].(bool)
		if !ok {
			return empty, nil
		}
		if out.ID == "" || utf8.RuneCountInString(out.ID) > 80 || seen[out.ID] || out.Name == "" || utf8.RuneCountInString(out.Name) > 100 || utf8.RuneCountInString(out.WhatsApp) > 30 || out.WhatsApp != "" && !servicePhone.MatchString(out.WhatsApp) || out.WhatsApp == "" && out.Email == "" || out.Department != "pre_sales" && out.Department != "after_sales" || out.Availability != "schedule" && out.Availability != "online" && out.Availability != "offline" || !serviceTime.MatchString(out.Start) || !serviceTime.MatchString(out.End) || out.Start >= out.End {
			return empty, nil
		}
		if out.Email != "" {
			address, e := mail.ParseAddress(out.Email)
			if e != nil || address.Address != out.Email || !strings.Contains(out.Email, "@") {
				return empty, nil
			}
		}
		if out.Timezone == "" || utf8.RuneCountInString(out.Timezone) > 80 || out.Timezone == "Local" {
			return empty, nil
		}
		if _, e := time.LoadLocation(out.Timezone); e != nil {
			return empty, nil
		}
		days, ok := row["days"].([]any)
		if !ok || len(days) < 1 || len(days) > 7 {
			return empty, nil
		}
		out.Days = []int{}
		for _, value := range days {
			n, ok := value.(json.Number)
			if !ok {
				return empty, nil
			}
			day, e := n.Int64()
			if e != nil || day < 0 || day > 6 {
				return empty, nil
			}
			out.Days = append(out.Days, int(day))
		}
		seen[out.ID] = true
		if enabled {
			result = append(result, out)
		}
	}
	return result, nil
}

func (h StoreHandler) Contacts(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "service.invalid_query", "Invalid contact request")
		return
	}
	if h.DB == nil || !validProductID(h.StoreID) {
		platform.Fail(c, 503, "service.unavailable", "Contacts unavailable")
		return
	}
	contacts := []ServiceContact{}
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if e := Ready(c.Request.Context(), tx); e != nil {
			return e
		}
		var row struct {
			Metadata *string
			Bytes    int
		}
		r := tx.Raw("SELECT LEFT(metadata,65537) AS metadata,COALESCE(OCTET_LENGTH(metadata),0) AS bytes FROM commerce_stores WHERE id=? AND deleted_at IS NULL", h.StoreID).Scan(&row)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 || row.Bytes > 65536 {
			return errStoreScope
		}
		var e error
		contacts, e = serviceContacts(row.Metadata)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		platform.Fail(c, 503, "service.unavailable", "Contacts unavailable")
		return
	}
	c.JSON(200, gin.H{"contacts": contacts})
}
