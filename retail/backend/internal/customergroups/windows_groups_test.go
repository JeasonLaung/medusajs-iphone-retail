package customergroups

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestWindowsCustomerGroupGuards(t *testing.T) {
	if os.Getenv("WINDOWS_CUSTOMER_GROUPS") != "1" {
		t.Skip("owned customer group checkpoint required")
	}
	cfg, err := driver.ParseDSN(os.Getenv("MYSQL_DSN"))
	owner, logs := os.Getenv("ACCOUNT_TEST_OWNER"), os.Getenv("ACCOUNT_TEST_LOGS")
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(owner) || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13307" || cfg.DBName != "retail_account_"+owner || filepath.Base(logs) != "account-"+owner {
		t.Fatal("owned loopback database required")
	}
	db, err := platform.Open()
	if err != nil {
		t.Fatal("owned database unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var actor identity.User
	if db.Where("role='admin' AND enabled=TRUE").Order("id").First(&actor).Error != nil {
		t.Fatal("owned admin unavailable")
	}
	fingerprint := func() string {
		t.Helper()
		var rows []json.RawMessage
		for _, table := range []string{"customer_groups", "customer_group_members", "customer_group_commands", "customer_group_runtime", "customer_accounts", "admin_users"} {
			var values []map[string]any
			if db.Table(table).Order("1").Find(&values).Error != nil {
				t.Fatal("fingerprint unavailable")
			}
			raw, _ := json.Marshal(values)
			rows = append(rows, raw)
		}
		raw, _ := json.Marshal(rows)
		return string(raw)
	}
	rollback := errors.New("owned guard rollback")
	checks := []string{"gate", "actor", "stale", "changed replay", "rename delete", "foreign account", "unverified account", "partial remove", "receipt failure"}
	for _, name := range checks {
		t.Run(name, func(t *testing.T) {
			before := fingerprint()
			err := db.Transaction(func(tx *gorm.DB) error {
				h := Handler{DB: tx}
				directory, e := h.Read(t.Context(), actor)
				if e != nil {
					t.Fatal(e)
				}
				command := Command{OperationID: uuid.NewString(), ExpectedRevision: directory.Revision, Kind: "create", Name: "Guard group 客户", CustomerIDs: []string{}}
				if name == "gate" {
					if tx.Exec("UPDATE customer_group_runtime SET enabled=FALSE WHERE id=1").Error != nil {
						t.Fatal("gate setup")
					}
					if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, ErrUnavailable) {
						t.Fatal("disabled gate accepted", e)
					}
					return rollback
				}
				if name == "actor" {
					if tx.Exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID).Error != nil {
						t.Fatal("actor setup")
					}
					if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, authorization.ErrDenied) {
						t.Fatal("disabled actor accepted", e)
					}
					return rollback
				}
				if name == "stale" {
					command.ExpectedRevision = "18446744073709551614"
					if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, ErrConflict) {
						t.Fatal("stale revision accepted", e)
					}
					return rollback
				}
				if name == "receipt failure" {
					command.OperationID = "00000000-0000-4000-8000-000000000001"
					if _, e = h.Change(t.Context(), actor, command); e == nil {
						t.Fatal("receipt fault accepted")
					}
					after, e := h.Read(t.Context(), actor)
					if e != nil || after.Revision != directory.Revision || len(after.Groups) != len(directory.Groups) {
						t.Fatal("late failure did not roll back", e)
					}
					return rollback
				}
				created, e := h.Change(t.Context(), actor, command)
				if e != nil {
					t.Fatal(e)
				}
				if name == "changed replay" {
					command.Name = "Changed replay"
					if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, ErrConflict) {
						t.Fatal("changed replay accepted", e)
					}
					return rollback
				}
				command = Command{OperationID: uuid.NewString(), ExpectedRevision: created.Revision, Kind: "rename", GroupID: created.GroupID, Name: "Renamed 客户", CustomerIDs: []string{}}
				if name == "rename delete" {
					renamed, e := h.Change(t.Context(), actor, command)
					if e != nil {
						t.Fatal(e)
					}
					command.OperationID = uuid.NewString()
					command.ExpectedRevision = renamed.Revision
					command.Kind = "delete"
					command.Name = ""
					deleted, e := h.Change(t.Context(), actor, command)
					if e != nil {
						t.Fatal(e)
					}
					replay, e := h.Change(t.Context(), actor, command)
					if e != nil || !replay.Replayed || replay.Revision != deleted.Revision {
						t.Fatal("deleted command replay", e)
					}
					groups, e := ForAccountTx(tx, "")
					if e != nil || len(groups) != 0 {
						t.Fatal("anonymous groups", e)
					}
					return rollback
				}
				command.Kind = "add_members"
				command.Name = ""
				command.CustomerIDs = []string{"foreign_new_account"}
				if name == "unverified account" || name == "partial remove" {
					var account struct{ ID string }
					if tx.Table("customer_accounts").Select("id").Where("enabled=TRUE AND verified_at IS NOT NULL").Order("id").Take(&account).Error != nil {
						t.Fatal("verified fixture missing")
					}
					command.CustomerIDs = []string{account.ID}
					if name == "unverified account" {
						if tx.Exec("UPDATE customer_accounts SET verified_at=NULL WHERE id=?", account.ID).Error != nil {
							t.Fatal("unverified setup")
						}
					} else {
						added, e := h.Change(t.Context(), actor, command)
						if e != nil {
							t.Fatal(e)
						}
						command.OperationID = uuid.NewString()
						command.ExpectedRevision = added.Revision
						command.Kind = "remove_members"
						other := "guard_" + uuid.NewString()
						if tx.Exec("INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) SELECT ?,?,password_hash,'Guard','Only',TRUE,1,verified_at,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM customer_accounts WHERE id=?", other, other+"@example.invalid", account.ID).Error != nil {
							t.Fatal("second account fixture failed")
						}
						command.CustomerIDs = append(command.CustomerIDs, other)
						if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, ErrConflict) {
							t.Fatal("partial removal accepted", e)
						}
						groups, e := ForAccountTx(tx, account.ID)
						if e != nil || len(groups) != 1 || groups[0] != created.GroupID {
							t.Fatal("partial removal lost membership", e)
						}
						return rollback
					}
				}
				if _, e = h.Change(t.Context(), actor, command); !errors.Is(e, ErrConflict) {
					t.Fatal("untrusted account accepted", e)
				}
				return rollback
			})
			if !errors.Is(err, rollback) || fingerprint() != before {
				t.Fatal("guard changed durable facts", err)
			}
		})
	}
	raw, _ := json.Marshal(map[string]any{"passed": true, "checks": checks, "all_changes_rolled_back": true})
	if os.WriteFile(filepath.Join(logs, "group-guards.json"), raw, 0600) != nil {
		t.Fatal("evidence write failed")
	}
}
