package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/eurorefurb/retail/internal/historyimport"
)

func run(args []string, out, diagnostic io.Writer) int {
	f := flag.NewFlagSet("audit-device-history", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	path := f.String("snapshot", "", "private complete v2 snapshot")
	sha := f.String("sha256", "", "independent full snapshot SHA256")
	fail := func() int {
		fmt.Fprintln(diagnostic, "Device audit rejected: complete private snapshot, pinned SHA256 and valid source facts required.")
		return 1
	}
	if f.Parse(args) != nil || f.NArg() != 0 || *path == "" || *sha == "" {
		return fail()
	}
	input, e := os.Open(*path)
	if e != nil {
		return fail()
	}
	defer input.Close()
	info, e := input.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return fail()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, e := historyimport.AuditDeviceHistory(ctx, input, *sha, 100000, 64<<20)
	if e != nil {
		return fail()
	}
	if json.NewEncoder(out).Encode(r) != nil {
		return 1
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
