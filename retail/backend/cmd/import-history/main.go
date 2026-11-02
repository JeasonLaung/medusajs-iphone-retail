// Explicit historical business and inventory import. No source connection, provider, worker,
// account migration or schema mutation. Default invocation is offline planning.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/historyimport"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func run(args []string, out, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("import-history", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("snapshot", "", "private complete v2 snapshot")
	sha := flags.String("sha256", "", "independent complete snapshot SHA256")
	apply := flags.Bool("apply", false, "commit reviewed historical customers, orders and inventory with catalog parents")
	rollback := flags.Bool("rollback", false, "stage and verify then roll back all writes")
	includePayments := flags.Bool("include-payment-history", false, "add eight encrypted historical payment tables; never call providers")
	includeLogistics := flags.Bool("include-logistics-history", false, "add reviewed fulfillment and tax history; never call providers")
	includeDevicePrerequisites := flags.Bool("include-device-prerequisites", false, "add reviewed device sources, SKU tracking and supply facts; never import device identities")
	includeCarts := flags.Bool("include-cart-history", false, "add historical cart facts without possession or execution authority")
	includeReviews := flags.Bool("include-product-reviews", false, "retain historical product reviews without new-account ownership or runtime activation")
	target := flags.String("target-database", "", "explicit dedicated eurorefurb_ database; must match MYSQL_DSN")
	fail := func(message string) int { fmt.Fprintln(diagnostic, message); return 1 }
	if flags.Parse(args) != nil || flags.NArg() != 0 || *path == "" || *sha == "" || (*apply && *rollback) {
		return fail("Required: --snapshot PATH --sha256 SHA256; choose offline plan, --rollback or --apply.")
	}
	f, err := os.Open(*path)
	if err != nil {
		return fail("Private snapshot unavailable.")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fail("Regular private snapshot required.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	prepared, err := historyimport.Prepare(ctx, f, *sha, 100000, 64<<20)
	if err != nil {
		return fail("History plan rejected: complete stream, exact values and closure references required. No writes performed.")
	}
	report := prepared.Report()
	if !*apply && !*rollback {
		if json.NewEncoder(out).Encode(report) != nil {
			return 1
		}
		return 0
	}
	_ = godotenv.Load()
	for _, name := range []string{"CUSTOMER_AUTH_ENABLED", "CHECKOUT_PAYMENT_ENABLED", "ORDER_REFUNDS_ENABLED"} {
		if value := os.Getenv(name); value != "" && value != "false" {
			return fail("Historical import requires customer, checkout and refund runtime gates disabled.")
		}
	}
	ring, err := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
	if err != nil {
		return fail("Explicit persistent historical-customer encryption keyring required.")
	}
	var paymentRing *protected.Ring
	if *includePayments {
		paymentRing, err = protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
		if err != nil {
			return fail("Explicit independent persistent historical-payment encryption keyring required.")
		}
	}
	cfg, err := mysql.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil || !validTarget(cfg, *target) {
		return fail("Explicit dedicated loopback MySQL database required; target must match MYSQL_DSN.")
	}
	db, err := platform.Open()
	if err != nil {
		return fail("Dedicated import target unavailable.")
	}
	pool, err := db.DB()
	if err != nil {
		return fail("Dedicated import target unavailable.")
	}
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil || schema.CheckApplied(ctx, pool, plan) != nil {
		return fail("Complete explicit schema migration plan required; importer never changes schema.")
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return fail("Dedicated target connection unavailable.")
	}
	defer conn.Close()
	var locked int
	if conn.QueryRowContext(ctx, "SELECT GET_LOCK('eurorefurb_catalog_snapshot',10)").Scan(&locked) != nil || locked != 1 {
		return fail("Another history import is active.")
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(release, "SELECT RELEASE_LOCK('eurorefurb_catalog_snapshot')")
	}()
	if _, err = conn.ExecContext(ctx, "SET SESSION time_zone='+00:00', SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'"); err != nil {
		return fail("Strict target session unavailable.")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fail("History transaction unavailable.")
	}
	defer tx.Rollback()
	if *includePayments {
		report, err = prepared.StageWithPayments(ctx, tx, ring, paymentRing)
	} else {
		report, err = prepared.Stage(ctx, tx, ring)
	}
	if err != nil {
		return fail("History source/target conflict or storage failure; entire batch rolled back. Existing business changes are never overwritten.")
	}
	if *includeLogistics {
		logistics, e := prepared.StageLogistics(ctx, tx)
		if e != nil {
			return fail("Historical fulfillment/tax conflict or storage failure; entire batch rolled back.")
		}
		report.Logistics = &logistics
	}
	if *includeCarts {
		carts, e := prepared.StageCarts(ctx, tx)
		if e != nil {
			return fail("Historical cart reference, authority or target conflict; entire batch rolled back.")
		}
		report.Carts = &carts
	}
	if *includeDevicePrerequisites {
		devices, e := prepared.StageDevicePrerequisites(ctx, tx)
		if e != nil {
			return fail("Device prerequisite gate, reference or target conflict; entire batch rolled back.")
		}
		report.DevicePrerequisites = &devices
	}
	if *includeReviews {
		reviews, e := prepared.StageProductReviews(ctx, tx)
		if e != nil {
			return fail("Historical product review gate, reference, authority or target conflict; entire batch rolled back.")
		}
		report.ProductReviews = &reviews
	}
	if *apply {
		if tx.Commit() != nil {
			return fail("History commit outcome uncertain; verify using the same snapshot before retrying.")
		}
		report.Applied = true
		report.Catalog.Applied = true
	} else if tx.Rollback() != nil {
		return fail("History rollback outcome unavailable.")
	}
	if json.NewEncoder(out).Encode(report) != nil {
		return 1
	}
	return 0
}

func validTarget(cfg *mysql.Config, target string) bool {
	valid := false
	if cfg != nil && strings.HasPrefix(target, "eurorefurb_") && len(target) > len("eurorefurb_") && cfg.DBName == target && cfg.Net == "tcp" {
		host, _, err := net.SplitHostPort(cfg.Addr)
		valid = err == nil && (host == "127.0.0.1" || host == "localhost" || host == "::1")
	}
	return valid
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
