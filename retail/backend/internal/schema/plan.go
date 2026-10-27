// Package schema runs explicit reviewed SQL migrations, never application startup DDL.
package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

var ErrPlan = errors.New("invalid explicit SQL migration plan")

type migration struct {
	name, checksum string
	statements     []string
}
type Plan struct{ files []migration }

var namePattern = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

// Load validates the entire plan before any connection or DDL. SQL bodies and
// statement text stay private. DELIMITER, procedures, versioned executable
// comments and unsupported quoting modes are not accepted by this runner.
func Load(source fs.FS) (*Plan, error) {
	if source == nil {
		return nil, ErrPlan
	}
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, ErrPlan
	}
	p := &Plan{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if e.IsDir() || !namePattern.MatchString(e.Name()) || len(p.files) >= 128 {
			return nil, ErrPlan
		}
		data, err := fs.ReadFile(source, e.Name())
		if err != nil || len(data) > 8<<20 {
			return nil, ErrPlan
		}
		statements, err := splitSQL(string(data))
		if err != nil || len(statements) == 0 || len(statements) > 10000 {
			return nil, ErrPlan
		}
		digest := sha256.Sum256(data)
		p.files = append(p.files, migration{e.Name(), hex.EncodeToString(digest[:]), statements})
	}
	if len(p.files) == 0 {
		return nil, ErrPlan
	}
	sort.Slice(p.files, func(i, j int) bool { return p.files[i].name < p.files[j].name })
	// A numeric version identifies exactly one file, not just one filename.
	for j := 1; j < len(p.files); j++ {
		if p.files[j-1].name[:4] == p.files[j].name[:4] {
			return nil, ErrPlan
		}
	}
	return p, nil
}

// Split only in normal lexical state. Keep the SQL byte-for-byte inside each
// statement, including comments and quoted semicolons. Reject ambiguous/incomplete
// constructs instead of executing a partial plan. No SQL values are logged.
func splitSQL(input string) ([]string, error) {
	var statements []string
	start := 0
	state := byte(0)
	hasCode := false
	for j := 0; j < len(input); j++ {
		c := input[j]
		switch state {
		case 'l':
			if c == '\n' {
				state = 0
			}
		case 'b':
			if c == '*' && j+1 < len(input) && input[j+1] == '/' {
				state = 0
				j++
			}
		case '\'', '"', '`':
			if c == '\\' {
				return nil, ErrPlan
			} // NO_BACKSLASH_ESCAPES changes semantics.
			if c == state {
				if j+1 < len(input) && input[j+1] == state {
					j++
				} else {
					state = 0
				}
			}
		default:
			if c == '#' {
				state = 'l'
				continue
			}
			if c == '-' && j+2 < len(input) && input[j+1] == '-' && input[j+2] <= 32 {
				state = 'l'
				j++
				continue
			}
			if c == '/' && j+1 < len(input) && input[j+1] == '*' {
				if j+2 < len(input) && (input[j+2] == '!' || input[j+2] == '+') {
					return nil, ErrPlan
				}
				state = 'b'
				j++
				continue
			}
			if c == ';' {
				if hasCode {
					statements = append(statements, input[start:j])
					hasCode = false
				}
				start = j + 1
				continue
			}
			if c == '\'' || c == '"' || c == '`' {
				state = c
				hasCode = true
				continue
			}
			if c > 32 {
				hasCode = true
			}
		}
	}
	if state != 0 && state != 'l' {
		return nil, ErrPlan
	}
	if hasCode {
		statements = append(statements, input[start:])
	}
	// DDL subset deliberately excludes routines/delimiter and double-quoted
	// strings; ANSI_QUOTES must not change the meaning of an accepted plan.
	for _, stmt := range statements {
		clean := strings.ToUpper(strings.TrimSpace(stmt))
		if strings.Contains(clean, "DELIMITER") || strings.Contains(clean, "CREATE PROCEDURE") || strings.Contains(clean, "CREATE FUNCTION") || strings.Contains(clean, "CREATE TRIGGER") || strings.Contains(stmt, "\"") {
			return nil, ErrPlan
		}
	}
	return statements, nil
}
