package seo

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AdminSetting struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Group     string `json:"group"`
	Protected bool   `json:"protected"`
}
type AdminPage struct {
	ID string `json:"id"`
	PageInput
}
type Directory struct {
	Revision        string         `json:"revision"`
	Enabled         bool           `json:"enabled"`
	EncryptionReady bool           `json:"encryption_ready"`
	Settings        []AdminSetting `json:"settings"`
	Pages           []AdminPage    `json:"pages"`
	Count           int64          `json:"count"`
	Limit           int            `json:"limit"`
	Offset          int            `json:"offset"`
}
type CommandResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}
type commandGate struct {
	Version int
	Enabled bool
}
type settingRecord struct {
	ID, Key, Group string
	Value          []byte
	IsSecret       bool
	Held           bool
}

func access(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.SEORead}
	if write {
		actions = append(actions, authorization.SEOWrite)
	}
	return authorization.CheckTx(ctx, tx, actor, actions...)
}
func lockGate(tx *gorm.DB, write bool) (commandGate, error) {
	var gate commandGate
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	if tx.Table("seo_command_state").Where("id=1").Clauses(clause.Locking{Strength: strength}).Take(&gate).Error != nil || gate.Version != 1 {
		return gate, ErrUnavailable
	}
	return gate, nil
}

// Hash full SQL values, including null/deleted rows and ciphertext, without
// decrypting unrelated or protected settings. The database returns only row
// digests. The gate lock makes list+revision and all managed writes coherent.
func stateRevision(tx *gorm.DB) (string, error) {
	h := sha256.New()
	for _, source := range []struct{ table, fields string }{
		{"seo_pages", "id,path,title,description,canonical,og_image,hreflang,structured_data,robots,created_at,updated_at,deleted_at"},
		{"seo_settings", "id,`key`,HEX(value),is_secret,`group`,created_at,updated_at,deleted_at"},
		{"migration_merchant_dispositions", "HEX(source_table),HEX(source_id),HEX(snapshot_sha256),HEX(source_row_sha256),HEX(target_row_sha256),HEX(initial_attempt),policy_version,HEX(disposition),HEX(reason)"},
	} {
		// Dispositions participate in revisions so release/hold changes cannot
		// be overwritten by a stale operator. Exact source rows stay intact.
		query := "SELECT SHA2(JSON_ARRAY(" + source.fields + "),256) FROM " + source.table
		if source.table == "migration_merchant_dispositions" {
			query += " WHERE source_table='seo_setting' ORDER BY source_id"
		} else {
			query += " ORDER BY id"
		}
		rows, e := tx.Raw(query).Rows()
		if e != nil {
			return "", ErrUnavailable
		}
		h.Write([]byte(source.table + "\n"))
		for rows.Next() {
			var digest string
			if rows.Scan(&digest) != nil || !revisionPattern.MatchString(digest) {
				rows.Close()
				return "", ErrUnavailable
			}
			h.Write([]byte(digest + "\n"))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return "", ErrUnavailable
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func settingsQuery(tx *gorm.DB) *gorm.DB {
	return tx.Table("seo_settings AS s").Select("s.id,s.`key`,s.value,s.is_secret,s.`group`,EXISTS(SELECT 1 FROM migration_merchant_dispositions d WHERE d.source_table='seo_setting' AND d.source_id=CAST(s.id AS BINARY)) AS held")
}
func checkSetting(row settingRecord) error {
	if !publicKey(row.Key) || row.IsSecret || row.Held {
		return ErrProtected
	}
	return nil
}

func (s Store) Directory(ctx context.Context, actor identity.User, resource string, limit, offset int) (Directory, error) {
	out := Directory{Settings: []AdminSetting{}, Pages: []AdminPage{}, EncryptionReady: s.Ring != nil, Limit: limit, Offset: offset}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	if (resource != "settings" && resource != "pages") || limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return out, ErrCommandInput
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := access(ctx, tx, actor, false); e != nil {
			return e
		}
		gate, e := lockGate(tx, false)
		if e != nil {
			return e
		}
		out.Enabled = gate.Enabled
		if resource == "settings" {
			var size struct{ Total, Largest int64 }
			base := tx.Table("seo_settings").Where("deleted_at IS NULL AND `key` IN ?", publicKeys)
			if base.Select("COALESCE(SUM(OCTET_LENGTH(value)),0) AS total,COALESCE(MAX(OCTET_LENGTH(value)),0) AS largest").Scan(&size).Error != nil || size.Total > 4*1024*1024 || size.Largest > protected.MaxPlaintext+256 {
				return ErrUnavailable
			}
			var rows []settingRecord
			if settingsQuery(tx).Where("s.deleted_at IS NULL AND s.`key` IN ?", publicKeys).Order("s.`key` ASC,s.id ASC").Limit(1001).Find(&rows).Error != nil || len(rows) > 1000 {
				return ErrUnavailable
			}
			for _, row := range rows {
				item := AdminSetting{ID: row.ID, Key: row.Key, Group: row.Group, Protected: checkSetting(row) != nil}
				if !item.Protected {
					if s.Ring == nil {
						return ErrUnavailable
					}
					plain, e := s.Ring.Open(protected.Context{Purpose: "merchant-config", Table: "seo_settings", Column: "value", Row: row.ID}, row.Value)
					if e != nil || !utf8.Valid(plain) {
						clear(plain)
						return ErrUnavailable
					}
					item.Value = string(plain)
					clear(plain)
				}
				out.Settings = append(out.Settings, item)
			}
			out.Count = int64(len(out.Settings))
		} else {
			base := tx.Table("seo_pages").Where("deleted_at IS NULL")
			if base.Count(&out.Count).Error != nil {
				return ErrUnavailable
			}
			var size struct{ Total, Largest int64 }
			if tx.Raw("SELECT COALESCE(SUM(bytes),0) AS total,COALESCE(MAX(bytes),0) AS largest FROM (SELECT OCTET_LENGTH(CONCAT_WS('',id,path,title,description,canonical,og_image,hreflang,structured_data,robots)) AS bytes FROM seo_pages WHERE deleted_at IS NULL ORDER BY created_at DESC,id ASC LIMIT ? OFFSET ?) x", limit, offset).Scan(&size).Error != nil || size.Total > 4*1024*1024 || size.Largest > 1024*1024 {
				return ErrUnavailable
			}
			if base.Select("id,path,title,description,canonical,og_image,COALESCE(hreflang,'{}') AS hreflang_json,COALESCE(structured_data,'[]') AS structured_data_json,robots").Order("created_at DESC,id ASC").Limit(limit).Offset(offset).Scan(&out.Pages).Error != nil {
				return ErrUnavailable
			}
		}
		out.Revision, e = stateRevision(tx)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

func mutableTarget(tx *gorm.DB, resource, id string, deleted bool) error {
	if resource == "settings" {
		var row settingRecord
		q := settingsQuery(tx).Where("s.id=?", id).Clauses(clause.Locking{Strength: "UPDATE"})
		if !deleted {
			q = q.Where("s.deleted_at IS NULL")
		}
		if e := q.Take(&row).Error; e != nil {
			return e
		}
		return checkSetting(row)
	}
	q := tx.Table("seo_pages").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"})
	if !deleted {
		q = q.Where("deleted_at IS NULL")
	}
	var found struct{ ID string }
	return q.Select("id").Take(&found).Error
}
func uniqueSetting(tx *gorm.DB, key, except string) error {
	var count int64
	if tx.Table("seo_settings").Where("`key`=? AND id<>? AND deleted_at IS NULL", key, except).Count(&count).Error != nil {
		return ErrUnavailable
	}
	if count > 0 {
		return ErrConflict
	}
	return nil
}

func (s Store) Save(ctx context.Context, actor identity.User, in Command) (CommandResult, error) {
	var out CommandResult
	raw, e := json.Marshal(in)
	if e != nil {
		return out, ErrCommandInput
	}
	in, e = decodeCommand(raw)
	if e != nil {
		return out, e
	}
	canonical, _ := json.Marshal(in)
	digest := sha256.Sum256(canonical)
	requestSHA := hex.EncodeToString(digest[:])
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := access(ctx, tx, actor, true); e != nil {
			return e
		}
		gate, e := lockGate(tx, true)
		if e != nil {
			return e
		}
		var receipt struct{ ActorID, Resource, Action, TargetID, RequestSHA256, ResultRevision string }
		e = tx.Table("seo_command_receipts").Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.ActorID != actor.ID || receipt.Resource != in.Resource || receipt.Action != in.Action || receipt.RequestSHA256 != requestSHA {
				return ErrConflict
			}
			if e := mutableTarget(tx, in.Resource, receipt.TargetID, in.Action == "delete"); e != nil {
				return e
			}
			out = CommandResult{receipt.TargetID, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !gate.Enabled {
			return ErrUnavailable
		}
		before, e := stateRevision(tx)
		if e != nil {
			return e
		}
		if in.ExpectedRevision != before {
			return ErrConflict
		}
		id := in.ID
		if id != "" {
			if e := mutableTarget(tx, in.Resource, id, false); e != nil {
				return e
			}
		}
		table := "seo_pages"
		if in.Resource == "settings" {
			table = "seo_settings"
			if in.Action == "save" && id == "" {
				var rows []settingRecord
				if settingsQuery(tx).Where("s.`key`=? AND s.deleted_at IS NULL", in.Setting.Key).Limit(2).Find(&rows).Error != nil {
					return ErrUnavailable
				}
				if len(rows) > 1 {
					return ErrConflict
				}
				if len(rows) == 1 {
					if e := checkSetting(rows[0]); e != nil {
						return e
					}
					id = rows[0].ID
				}
			}
		}
		create := id == ""
		if create {
			id = "seo-" + in.OperationID
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		values := map[string]any{"updated_at": now}
		if in.Action == "delete" {
			values["deleted_at"] = now
		} else if in.Resource == "settings" {
			if e := uniqueSetting(tx, in.Setting.Key, id); e != nil {
				return e
			}
			if s.Ring == nil {
				return ErrUnavailable
			}
			envelope, e := s.Ring.Seal(protected.Context{Purpose: "merchant-config", Table: table, Column: "value", Row: id}, []byte(in.Setting.Value))
			if e != nil {
				return ErrUnavailable
			}
			values["key"], values["value"], values["is_secret"], values["group"] = in.Setting.Key, envelope, false, in.Setting.Group
		} else {
			p := in.Page
			var count int64
			if tx.Table(table).Where("path=? AND id<>? AND deleted_at IS NULL", p.Path, id).Count(&count).Error != nil {
				return ErrUnavailable
			}
			if count > 0 {
				return ErrConflict
			}
			values["path"], values["title"], values["description"], values["canonical"], values["og_image"], values["robots"] = p.Path, p.Title, p.Description, p.Canonical, p.OGImage, p.Robots
			values["hreflang"], values["structured_data"] = p.HreflangJSON, p.StructuredDataJSON
		}
		if create {
			values["id"], values["created_at"] = id, now
			e = tx.Table(table).Create(values).Error
		} else {
			q := tx.Table(table).Where("id=? AND deleted_at IS NULL", id).Updates(values)
			e = q.Error
			if e == nil && q.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		if e != nil {
			return e
		}
		after, e := stateRevision(tx)
		if e != nil {
			return e
		}
		// Never persist plaintext setting values in a second audit location.
		if e = tx.Table("seo_command_receipts").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "resource": in.Resource, "action": in.Action, "target_id": id, "request_sha256": requestSHA, "before_revision": before, "result_revision": after, "created_at": now}).Error; e != nil {
			return e
		}
		out = CommandResult{id, after, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
