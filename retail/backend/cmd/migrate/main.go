package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
)

func nilContext() context.Context { return context.Background() }

func main() {
	_ = godotenv.Load()
	db, err := platform.Open()
	if err != nil {
		log.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	// DDL in MySQL commits implicitly. Serialize and journal each completed file;
	// scripts are restartable, and changed applied checksums are rejected.
	conn, err := pool.Conn(nilContext())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	var locked int
	if err = conn.QueryRowContext(nilContext(), "SELECT GET_LOCK('eurorefurb_schema', 10)").Scan(&locked); err != nil || locked != 1 {
		log.Fatal("another schema migration is active")
	}
	defer conn.ExecContext(nilContext(), "SELECT RELEASE_LOCK('eurorefurb_schema')")
	_, err = conn.ExecContext(nilContext(), "CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(191) PRIMARY KEY, checksum CHAR(64) NOT NULL, applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)) ENGINE=InnoDB")
	if err != nil {
		log.Fatal(err)
	}
	names, err := migrations.Files.ReadDir(".")
	if err != nil {
		log.Fatal(err)
	}
	sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
	for _, entry := range names {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, _ := migrations.Files.ReadFile(entry.Name())
		checksum := fmt.Sprintf("%x", sha256.Sum256(data))
		var old string
		err = conn.QueryRowContext(nilContext(), "SELECT checksum FROM schema_migrations WHERE version=?", entry.Name()).Scan(&old)
		if err == nil {
			if old != checksum {
				log.Fatal("applied migration changed: ", entry.Name())
			}
			continue
		}
		if err != sql.ErrNoRows {
			log.Fatal(err)
		}
		for _, statement := range strings.Split(string(data), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err = conn.ExecContext(nilContext(), statement); err != nil {
				log.Fatalf("%s: %v", entry.Name(), err)
			}
		}
		if _, err = conn.ExecContext(nilContext(), "INSERT INTO schema_migrations(version,checksum) VALUES(?,?)", entry.Name(), checksum); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Applied", entry.Name())
	}
}
