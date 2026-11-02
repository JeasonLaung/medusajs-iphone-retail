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
	markedTriggers := map[string]bool{}
	start := 0
	state := byte(0)
	hasCode := false
	compound := false
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
			// Explicit line markers keep a reviewed trigger body in one journaled
			// Exec. Markers in strings/comments are ordinary content, never control.
			if j == 0 || input[j-1] == '\n' {
				end := strings.IndexByte(input[j:], '\n')
				if end < 0 {
					end = len(input) - j
				}
				line := strings.TrimSuffix(input[j:j+end], "\r")
				if line == "-- retail:trigger-begin" {
					if compound || hasCode {
						return nil, ErrPlan
					}
					compound = true
					start = j + end
					j += end - 1
					continue
				}
				if line == "-- retail:trigger-end" {
					if !compound {
						return nil, ErrPlan
					}
					body := strings.TrimSpace(input[start:j])
					if !validTriggerBody(body) {
						return nil, ErrPlan
					}
					statements = append(statements, body)
					markedTriggers[body] = true
					compound = false
					hasCode = false
					start = j + end
					j += end - 1
					continue
				}
			}
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
			if c == ';' && !compound {
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
	if compound || state != 0 && state != 'l' {
		return nil, ErrPlan
	}
	if hasCode {
		statements = append(statements, input[start:])
	}
	// DDL subset deliberately excludes routines/delimiter and double-quoted
	// strings; ANSI_QUOTES must not change the meaning of an accepted plan.
	for _, stmt := range statements {
		clean := strings.ToUpper(strings.TrimSpace(stmt))
		if strings.Contains(clean, "DELIMITER") || strings.Contains(clean, "CREATE PROCEDURE") || strings.Contains(clean, "CREATE FUNCTION") || strings.Contains(clean, "CREATE TRIGGER") && !markedTriggers[stmt] || strings.Contains(stmt, "\"") {
			return nil, ErrPlan
		}
	}
	return statements, nil
}

var triggerHeader = regexp.MustCompile(`(?is)^CREATE TRIGGER [a-z][a-z0-9_]* BEFORE (INSERT|UPDATE) ON [a-z][a-z0-9_]* FOR EACH ROW BEGIN\s`)
var triggerBegin = regexp.MustCompile(`(?i)\bBEGIN\b`)
var triggerEnd = regexp.MustCompile(`(?i)\bEND\s*;`)

func validTriggerBody(body string) bool {
	// One outer compound block only. Nested IF...END IF is supported; nested
	// BEGIN, routines, custom delimiters and statements after END are rejected.
	return triggerHeader.MatchString(body) && strings.HasSuffix(body, "END;") && len(triggerBegin.FindAllStringIndex(body, -1)) == 1 && len(triggerEnd.FindAllStringIndex(body, -1)) == 1
}
