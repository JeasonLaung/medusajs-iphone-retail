package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgumentAndPrivateInputFailuresDoNotLeakOrEmitReports(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "private-identity")
	if os.WriteFile(private, []byte("private-secret-malformed-input"), 0600) != nil {
		t.Fatal("fixture unavailable")
	}
	for _, args := range [][]string{
		nil, {"--unknown=private-secret"},
		{"--snapshot", private, "--sha256", strings.Repeat("0", 64)},
		{"--snapshot", dir, "--sha256", strings.Repeat("0", 64)},
		{"--snapshot", private, "--sha256", strings.Repeat("0", 64), "--max-rows", "0"},
	} {
		var output, diagnostic bytes.Buffer
		if run(args, &output, &diagnostic) != 1 || output.Len() != 0 || diagnostic.Len() == 0 {
			t.Fatal("incorrect failure output")
		}
		if bytes.Contains(diagnostic.Bytes(), []byte("private-")) {
			t.Fatal("private input disclosed")
		}
	}
}
