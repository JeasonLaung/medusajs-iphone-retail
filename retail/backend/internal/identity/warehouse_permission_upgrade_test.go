package identity_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Runs only from the owned MySQL rehearsal, never against the development DB.
func checkWarehousePermissionUpgrade(t *testing.T) {
	t.Helper()
	cfg := driver.NewConfig()
	cfg.User = "root"
	cfg.Passwd = os.Getenv("MYSQL_PWD")
	cfg.Net = "unix"
	cfg.Addr = "/var/run/mysqld/mysqld.sock"
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	db, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal("permission upgrade connection")
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	const name = "warehouse_permission_upgrade"
	if db.Exec("CREATE DATABASE "+name).Error != nil {
		t.Fatal("owned permission upgrade DB")
	}
	defer db.Exec("DROP DATABASE " + name)
	cfg.DBName = name
	upgrade, e := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	target, e := upgrade.DB()
	if e != nil {
		t.Fatal(e)
	}
	defer target.Close()
	earlier := fstest.MapFS{}
	entries, e := fs.ReadDir(migrations.Files, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if entry.Name() < "0082" {
			data, e := fs.ReadFile(migrations.Files, entry.Name())
			if e != nil {
				t.Fatal(e)
			}
			earlier[entry.Name()] = &fstest.MapFile{Data: data}
		}
	}
	before, e := schema.Load(earlier)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = schema.Apply(t.Context(), target, before); e != nil {
		t.Fatal("earlier permission schema", e)
	}
	for _, q := range []string{
		"INSERT INTO authorization_roles(id,name,enabled) VALUES('pre-upgrade-warehouse-reader','Existing reader',TRUE)",
		"INSERT INTO authorization_role_actions(role_id,action_code) VALUES('pre-upgrade-warehouse-reader','order.fulfillment.read')",
	} {
		if upgrade.Exec(q).Error != nil {
			t.Fatal("earlier role fixture")
		}
	}
	if authorization.Ready(t.Context(), upgrade) == nil {
		t.Fatal("new runtime accepted old action model")
	}
	all, e := schema.Load(migrations.Files)
	if e != nil {
		t.Fatal(e)
	}
	result, e := schema.Apply(t.Context(), target, all)
	if e != nil || result.Applied != 1 || authorization.Ready(t.Context(), upgrade) != nil {
		t.Fatal("permission upgrade unavailable", e)
	}
	var grants []string
	if upgrade.Table("authorization_role_actions").Where("role_id='pre-upgrade-warehouse-reader'").Pluck("action_code", &grants).Error != nil || len(grants) != 1 || grants[0] != "order.fulfillment.read" {
		t.Fatal("migration silently granted warehouse write")
	}
	var count int64
	if upgrade.Table("authorization_actions").Where("code IN ('warehouse.allocation.write','warehouse.shipment.write')").Count(&count).Error != nil || count != 2 {
		t.Fatal("dedicated actions missing")
	}
	if upgrade.Exec("INSERT INTO authorization_actions(code) VALUES('warehouse.unreviewed.write')").Error == nil {
		t.Fatal("unknown action accepted by MySQL")
	}
	again, e := schema.Apply(t.Context(), target, all)
	if e != nil || again.Applied != 0 {
		t.Fatal("permission upgrade replay")
	}
	t.Log(`WAREHOUSE_PERMISSION_UPGRADE {"real_mysql":true,"old_roles_unchanged":true,"distinct_allocation_and_shipment_actions":true,"old_model_rejected":true,"new_model_ready":true,"unknown_action_sql_rejected":true,"migration_replay":true}`)
}
