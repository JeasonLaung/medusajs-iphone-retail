package migrations

import "embed"

// Files are immutable once applied. New changes require a new numbered file.
//
//go:embed *.sql
var Files embed.FS
