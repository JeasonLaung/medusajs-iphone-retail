// The v1 draft cannot preserve source NULLs or complete business facts.
// Fail before opening a file, loading dotenv or connecting to any database.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Legacy v1 import disabled. Use cmd/check-snapshot for v2 read-only diagnostics; the complete v2 write coordinator is not available yet.")
	os.Exit(1)
}
