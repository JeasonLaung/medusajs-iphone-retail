// Offline v2 preflight: no dotenv, database, Redis, HTTP or source connection.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/eurorefurb/retail/internal/rowimport"
	"github.com/eurorefurb/retail/internal/snapshotcheck"
)

func run(args []string, output, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("check-snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	snapshot := flags.String("snapshot", "", "private v2 NDJSON snapshot")
	sha := flags.String("sha256", "", "independently reviewed complete file SHA256")
	rows := flags.Int64("max-rows", 100000, "maximum diagnostic rows")
	keys := flags.Int64("max-key-bytes", 64<<20, "maximum retained reference-key payload bytes (not a Go heap limit)")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *snapshot == "" || *sha == "" || *rows <= 0 || *keys <= 0 {
		fmt.Fprintln(diagnostic, "Required: --snapshot PATH --sha256 SHA256; positive diagnostic limits only.")
		return 1
	}
	file, err := os.Open(*snapshot)
	if err != nil {
		fmt.Fprintln(diagnostic, "Private snapshot unavailable.")
		return 1
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(diagnostic, "Regular snapshot file required.")
		return 1
	}
	report, err := snapshotcheck.Check(file, *sha, rowimport.RelationLimits{MaxRows: *rows, KeyBytes: *keys})
	if err != nil {
		fmt.Fprintln(diagnostic, "Snapshot diagnostics failed; invalid, incomplete or over-limit input. No writes performed.")
		return 1
	}
	if json.NewEncoder(output).Encode(report) != nil {
		return 1
	}
	if !report.ScalarAndReferenceChecksPassed {
		return 2
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
