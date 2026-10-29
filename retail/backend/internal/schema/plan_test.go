package schema

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eurorefurb/retail/migrations"
)

func TestStatementBoundaries(t *testing.T) {
	input := "-- comment; not a statement\nCREATE TABLE `semi;name`(value TEXT);\nINSERT INTO `semi;name` VALUES('a;''b'); /* comment; */ # tail;\n"
	statements, err := splitSQL(input)
	if err != nil || len(statements) != 2 || !strings.Contains(statements[0], "`semi;name`") || !strings.Contains(statements[1], "'a;''b'") {
		t.Fatal("quoted/comment semicolon split incorrectly")
	}
	for _, bad := range []string{"CREATE TABLE `unfinished(x INT);", "SELECT 'unterminated;", "/* unterminated", "/*!80000 CREATE TABLE private_name(x INT) */;", "DELIMITER $$ CREATE PROCEDURE p() BEGIN SELECT 1; END $$", `INSERT INTO t VALUES('back\slash');`, `CREATE TABLE "ambiguous"(x INT);`} {
		if _, err := splitSQL(bad); err != ErrPlan {
			t.Fatal("unsupported/incomplete SQL accepted")
		}
	}
	if got, err := splitSQL("-- no code;\n /* trailing; */"); err != nil || len(got) != 0 {
		t.Fatal("comment-only file generated SQL")
	}
}

func TestPlanChecksAllFilesBeforeExecution(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	for _, source := range []fstest.MapFS{
		{}, {"bad.sql": file("SELECT 1;")},
		{"0001_a.sql": file("SELECT 1;"), "0001_b.sql": file("SELECT 2;")},
		{"0001_a.sql": file("-- empty")}, {"0001_a.sql": file("CREATE TABLE valid(x INT);"), "0002_b.sql": file("'unterminated")},
	} {
		if _, err := Load(source); err != ErrPlan {
			t.Fatal("invalid complete plan accepted")
		}
	}
	plan, err := Load(fstest.MapFS{"0002_second.sql": file("SELECT 2;"), "0001_first.sql": file("SELECT 1;")})
	if err != nil || len(plan.files) != 2 || plan.files[0].name != "0001_first.sql" {
		t.Fatal("plan order incorrect")
	}
	changed, err := Load(fstest.MapFS{"0001_first.sql": file("SELECT 1;\n")})
	if err != nil || changed.files[0].checksum == plan.files[0].checksum {
		t.Fatal("exact file byte change not detected")
	}
	if _, err := Load(nil); err != ErrPlan {
		t.Fatal("nil source accepted")
	}
	if _, err := Apply(t.Context(), nil, plan); err != ErrPlan {
		t.Fatal("nil target accepted")
	}
}

func TestEmbeddedMigrationPlanParses(t *testing.T) {
	plan, err := Load(migrations.Files)
	if err != nil || len(plan.files) != 36 {
		t.Fatal("explicit embedded migration plan invalid")
	}
	if !strings.HasPrefix(plan.files[0].name, "0001_") || !strings.HasPrefix(plan.files[35].name, "0036_") {
		t.Fatal("embedded version boundaries differ")
	}
}
