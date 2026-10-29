// Package devices preserves physical identities independently of quantity stock.
package devices

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrIdentity = errors.New("invalid device identity")
var ErrDuplicate = errors.New("device identity already occupied")
var ErrUnavailable = errors.New("device identity unavailable")

type Identity struct {
	SerialNumber string `json:"serial_number"`
	IMEI         string `json:"imei"`
	IMEI2        string `json:"imei2"`
	EID          string `json:"eid"`
}

var imeiPattern = regexp.MustCompile(`^[0-9]{15}$`)
var eidPattern = regexp.MustCompile(`^[0-9]{32}$`)
var iphonePattern = regexp.MustCompile(`(?i)\biphone\b`)

// RequiresRadio follows the original product precedence, including false.
// Metadata must come from the verified product, never the intake request.
func RequiresRadio(metadata map[string]any, title, handle, productType string) bool {
	if value, ok := metadata["requires_imei"].(bool); ok {
		return value
	}
	if value, ok := metadata["device_type"].(string); ok {
		return strings.ToLower(value) == "iphone"
	}
	return iphonePattern.MatchString(strings.Join([]string{title, handle, productType}, " "))
}
func NormalizeIdentity(in Identity, requiresRadio bool) (Identity, error) {
	in.SerialNumber = cases.Upper(language.Und).String(strings.TrimSpace(in.SerialNumber))
	in.IMEI = strings.TrimSpace(in.IMEI)
	in.IMEI2 = strings.TrimSpace(in.IMEI2)
	in.EID = strings.TrimSpace(in.EID)
	if !utf8.ValidString(in.SerialNumber) || in.SerialNumber == "" || utf8.RuneCountInString(in.SerialNumber) > 100 {
		return Identity{}, ErrIdentity
	}
	for _, r := range in.SerialNumber {
		if unicode.IsControl(r) {
			return Identity{}, ErrIdentity
		}
	}
	radios := map[string]bool{}
	for _, v := range []string{in.IMEI, in.IMEI2} {
		if v != "" && !imeiPattern.MatchString(v) {
			return Identity{}, ErrIdentity
		}
	}
	if in.EID != "" && !eidPattern.MatchString(in.EID) {
		return Identity{}, ErrIdentity
	}
	for _, v := range []string{in.IMEI, in.IMEI2, in.EID} {
		if v == "" {
			continue
		}
		if radios[v] {
			return Identity{}, ErrIdentity
		}
		radios[v] = true
	}
	if requiresRadio && len(radios) == 0 {
		return Identity{}, ErrIdentity
	}
	return in, nil
}

type key struct{ Kind, Value, Field string }

func keys(in Identity) []key {
	out := []key{{"serial", in.SerialNumber, "serial_number"}}
	for _, v := range []struct{ field, value string }{{"imei", in.IMEI}, {"imei2", in.IMEI2}, {"eid", in.EID}} {
		if v.value != "" {
			out = append(out, key{"radio", v.value, v.field})
		}
	}
	return out
}

// ClaimIdentityTx is the identity step inside a caller-owned intake transaction.
// The caller MUST authorize the actor, SKU, source, category, files and warehouse
// before LockIdentityTx, inserting the device and calling this step; it grants no business right.
// It requires the newly inserted active row to match the canonical identity.
// The global device row serializes absent-key conflicts with other intake/import
// transactions. Imports, identity edits and soft deletion must use that same lock.
// A caller failure must roll back device, claims and events together.
func ClaimIdentityTx(ctx context.Context, tx *gorm.DB, deviceID string, input Identity, requiresRadio bool) error {
	if tx == nil || tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	in, e := NormalizeIdentity(input, requiresRadio)
	if e != nil {
		return e
	}
	if in != input || !validID(deviceID) {
		return ErrIdentity
	}
	if e := LockIdentityTx(ctx, tx); e != nil {
		return e
	}
	var stored struct {
		SerialNumber *string `gorm:"column:serial_number"`
		IMEI         *string `gorm:"column:imei"`
		IMEI2        *string `gorm:"column:imei2"`
		EID          *string `gorm:"column:eid"`
	}
	if tx.WithContext(ctx).Table("devices").Select("serial_number,imei,imei2,eid").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", deviceID).Take(&stored).Error != nil {
		return ErrIdentity
	}
	value := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	if (Identity{value(stored.SerialNumber), value(stored.IMEI), value(stored.IMEI2), value(stored.EID)}) != in {
		return ErrIdentity
	}
	for _, k := range keys(in) {
		// Check source-compatible rows too: imported history need not have claims yet.
		query := tx.WithContext(ctx).Table("devices").Select("id").Clauses(clause.Locking{Strength: "SHARE"}).Where("deleted_at IS NULL AND id<>?", deviceID)
		if k.Kind == "serial" {
			query = query.Where("serial_number=?", k.Value)
		} else {
			query = query.Where("imei=? OR imei2=? OR eid=?", k.Value, k.Value, k.Value)
		}
		var duplicate struct{ ID string }
		e := query.Take(&duplicate).Error
		if e == nil {
			return ErrDuplicate
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		var old struct{ DeviceID, Field string }
		e = tx.Table("device_identity_claims").Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND value=?", k.Kind, k.Value).Take(&old).Error
		if e == nil {
			if old.DeviceID == deviceID && old.Field == k.Field {
				continue
			}
			var previous struct{ ID string }
			if tx.Table("devices").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NOT NULL", old.DeviceID).Take(&previous).Error != nil {
				return ErrDuplicate
			}
			if tx.Exec("DELETE FROM device_identity_claims WHERE kind=? AND value=? AND device_id=?", k.Kind, k.Value, old.DeviceID).Error != nil {
				return ErrUnavailable
			}
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrUnavailable
		}
		if tx.Exec("INSERT INTO device_identity_claims(kind,value,device_id,field) VALUES(?,?,?,?)", k.Kind, k.Value, deviceID, k.Field).Error != nil {
			return ErrUnavailable
		}
	}
	if tx.Exec("UPDATE device_runtime_schema SET revision=revision+1 WHERE id=1").Error != nil {
		return ErrUnavailable
	}
	return nil
}
func validID(id string) bool {
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// LockIdentityTx must run after current business authorization and before any
// device row insert/update/delete. It serializes complete identity transactions.
func LockIdentityTx(ctx context.Context, tx *gorm.DB) error {
	if tx == nil || tx.Statement == nil {
		return ErrUnavailable
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrUnavailable
	}
	var gate struct {
		Version         int
		Revision        uint64
		IdentityEnabled bool
	}
	if tx.WithContext(ctx).Table("device_runtime_schema").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=1").Take(&gate).Error != nil || gate.Version != 1 || gate.Revision == 0 || gate.Revision == math.MaxUint64 || !gate.IdentityEnabled {
		return ErrUnavailable
	}
	return nil
}
