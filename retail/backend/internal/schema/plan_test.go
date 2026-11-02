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
	if err != nil || len(plan.files) != 107 {
		t.Fatal("explicit embedded migration plan invalid")
	}
	if !strings.HasPrefix(plan.files[0].name, "0001_") || !strings.HasPrefix(plan.files[35].name, "0036_") {
		t.Fatal("embedded version boundaries differ")
	}
}

func TestExplicitTriggerBoundaries(t *testing.T) {
	body := `CREATE TRIGGER test_guard BEFORE INSERT ON sample FOR EACH ROW BEGIN
 DECLARE existing VARCHAR(191);
 SELECT id INTO existing FROM sample WHERE id=NEW.id LIMIT 1 FOR SHARE;
 IF existing IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='duplicate; rejected'; END IF;
 END;`
	wrap := func(value string) string { return "-- retail:trigger-begin\n" + value + "\n-- retail:trigger-end\n" }
	input := "CREATE TABLE sample(id VARCHAR(191));\n" + wrap(body) + "INSERT INTO sample VALUES('one');"
	got, e := splitSQL(input)
	if e != nil || len(got) != 3 || got[1] != body {
		t.Fatalf("trigger boundaries lost: %d %v", len(got), e)
	}
	for _, raw := range []string{
		body, wrap("SELECT 1;"), wrap(strings.Replace(body, "BEFORE INSERT", "AFTER INSERT", 1)),
		"-- retail:trigger-begin\n" + body,
		"-- retail:trigger-end\n", wrap(body) + "-- retail:trigger-end\n",
		"SELECT 1\n" + wrap(body), wrap(strings.Replace(body, "DECLARE existing", "BEGIN DECLARE existing", 1)),
		wrap(body + "\nDROP TABLE sample;"), wrap(strings.Replace(body, " END;", " END; SELECT 1; END;", 1)),
		"-- retail:trigger-begin\n-- retail:trigger-begin\n" + body + "\n-- retail:trigger-end\n",
		wrap(strings.Replace(body, "'duplicate; rejected'", "'unterminated", 1)),
		wrap(strings.Replace(body, "DECLARE existing", "/*!80000 SELECT 1 */; DECLARE existing", 1)),
	} {
		if _, e := splitSQL(raw); e != ErrPlan {
			t.Fatal("ambiguous trigger block accepted")
		}
	}
	for _, raw := range []string{"SELECT '\n-- retail:trigger-begin\n';", "/*\n-- retail:trigger-begin\n*/ SELECT 1;"} {
		if got, e := splitSQL(raw); e != nil || len(got) != 1 {
			t.Fatal("literal marker treated as control", e)
		}
	}
	// Every statement is validated before any Apply can connect or run DDL.
	if _, e := Load(fstest.MapFS{"0001_trigger.sql": {Data: []byte(input + "\n-- retail:trigger-begin\nSELECT 1;")}}); e != ErrPlan {
		t.Fatal("partial compound migration accepted")
	}
}
