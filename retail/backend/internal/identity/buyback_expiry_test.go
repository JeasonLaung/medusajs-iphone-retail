package identity_test

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/buyback"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"testing"
	"time"
)

func TestVerifyBuybackPhotoExpiry(t *testing.T) {
	if os.Getenv("BROWSER_IDENTITY_FIXTURE") != "1" {
		t.Skip("owned fixture required")
	}
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.DBName = "browser_identity"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("fixture SQL")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	cutoff := now.Add(-180 * 24 * time.Hour)
	var request struct{ ID string }
	if db.Table("buyback_requests").Select("id").Where("status='received'").Take(&request).Error != nil {
		t.Fatal("received fixture")
	}
	var photos []struct{ ID string }
	if db.Table("buyback_photos").Select("id").Where("request_id=?", request.ID).Order("id").Find(&photos).Error != nil || len(photos) != 3 {
		t.Fatal("three photos fixture")
	}
	sentinel := errors.New("rollback expiry fixture")
	for _, mode := range []string{"open", "exact", "recent", "expired", "dry", "legacy", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			e := db.Transaction(func(tx *gorm.DB) error {
				closed := any(cutoff.Add(-time.Microsecond))
				if mode == "open" {
					closed = nil
				}
				if mode == "exact" {
					closed = cutoff
				}
				if mode == "recent" {
					closed = cutoff.Add(time.Microsecond)
				}
				if tx.Exec("UPDATE buyback_requests SET closed_at=? WHERE id=?", closed, request.ID).Error != nil {
					t.Fatal("closed fixture")
				}
				if mode == "legacy" {
					if tx.Exec("UPDATE buyback_photos SET storage_key='legacy-private-object' WHERE id=?", photos[0].ID).Error != nil {
						t.Fatal("legacy fixture")
					}
				}
				apply := mode != "dry"
				v, e := buyback.ExpirePhotoPage(ctx, tx, now, "", 1, apply)
				if e != nil {
					t.Fatal("expiry", e)
				}
				eligible := mode == "expired" || mode == "rollback" || mode == "legacy" || mode == "dry"
				want := 0
				if eligible && apply {
					want = 3
				}
				if mode == "legacy" {
					want = 2
					if v.LegacyFiles != 1 {
						t.Fatal("legacy not counted")
					}
				}
				if v.Expired != want {
					t.Fatal("expiry count", mode, v)
				}
				var bytes, expired int64
				tx.Table("buyback_photo_bytes").Count(&bytes)
				tx.Table("buyback_photos").Where("request_id=? AND review_status='expired' AND storage_key=''", request.ID).Count(&expired)
				if bytes != int64(3-want) || expired != int64(want) {
					t.Fatal("blob/metadata mismatch", mode, bytes, expired)
				}
				if eligible && apply {
					repeat, e := buyback.ExpirePhotoPage(ctx, tx, now, "", 1, true)
					if e != nil || repeat.Expired != 0 {
						t.Fatal("repeat", e)
					}
				}
				return sentinel
			})
			if !errors.Is(e, sentinel) {
				t.Fatal("fixture rollback", e)
			}
		})
	}
	t.Run("metadata-failure-restores-private-bytes", func(t *testing.T) {
		callback := "expiry-fixture-failure"
		if e := db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
			if tx.Statement.Table == "buyback_photos" {
				tx.AddError(errors.New("artificial metadata failure"))
			}
		}); e != nil {
			t.Fatal("failure callback")
		}
		defer db.Callback().Update().Remove(callback)
		e := db.Transaction(func(tx *gorm.DB) error {
			if tx.Exec("UPDATE buyback_requests SET closed_at=? WHERE id=?", cutoff.Add(-time.Microsecond), request.ID).Error != nil {
				t.Fatal("fault fixture")
			}
			if _, e := buyback.ExpirePhotoPage(ctx, tx, now, "", 1, true); e == nil {
				t.Fatal("injected failure accepted")
			}
			var bytes, expired int64
			tx.Table("buyback_photo_bytes").Count(&bytes)
			tx.Table("buyback_photos").Where("request_id=? AND review_status='expired'", request.ID).Count(&expired)
			if bytes != 3 || expired != 0 {
				t.Fatal("failed metadata left missing private bytes")
			}
			return sentinel
		})
		if !errors.Is(e, sentinel) {
			t.Fatal("fault rollback", e)
		}
	})
	var bytes, expired int64
	db.Table("buyback_photo_bytes").Count(&bytes)
	db.Table("buyback_photos").Where("review_status='expired'").Count(&expired)
	if bytes != 3 || expired != 0 {
		t.Fatal("expiry rollback leaked")
	}
	t.Log(`BUYBACK_EXPIRY {"strict_180_day_boundary":true,"open_preserved":true,"dry_run":true,"private_bytes_deleted_atomically":true,"legacy_preserved":true,"repeat_safe":true,"rollback":true,"metadata_failure_restores_bytes":true}`)
}
