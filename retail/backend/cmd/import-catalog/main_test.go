package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestTargetRequiresExplicitOwnedLoopbackDatabase(t *testing.T) {
	for _, sample := range []struct {
		dsn, target string
		ok          bool
	}{
		{"u:p@tcp(127.0.0.1:3306)/eurorefurb_trial", "eurorefurb_trial", true},
		{"u:p@tcp([::1]:3306)/eurorefurb_trial", "eurorefurb_trial", true},
		{"u:p@tcp(127.0.0.1:3306)/iphone", "iphone", false},
		{"u:p@tcp(127.0.0.1:3306)/eurorefurb_trial", "eurorefurb_other", false},
		{"u:p@tcp(example.com:3306)/eurorefurb_trial", "eurorefurb_trial", false},
		{"u:p@tcp(127.0.0.1:3306)/eurorefurb_trial", "", false},
	} {
		cfg, err := mysql.ParseDSN(sample.dsn)
		if err != nil {
			t.Fatal(err)
		}
		if validTarget(cfg, sample.target) != sample.ok {
			t.Fatal("wrong target boundary")
		}
	}
	if validTarget(nil, "eurorefurb_trial") {
		t.Fatal("nil target accepted")
	}
}

func TestInvalidInputCannotReadEnvironmentOrExposeConfiguration(t *testing.T) {
	t.Setenv("MYSQL_DSN", "private-password-in-invalid-dsn")
	for _, args := range [][]string{nil, {"--snapshot", "missing-private-path", "--sha256", strings.Repeat("0", 64)}, {"--apply", "--rollback"}, {"--unknown"}} {
		var out, diagnostic bytes.Buffer
		if run(args, &out, &diagnostic) != 1 || out.Len() != 0 {
			t.Fatal("invalid input returned evidence")
		}
		if strings.Contains(diagnostic.String(), "private-") {
			t.Fatal("private details leaked")
		}
	}
}
