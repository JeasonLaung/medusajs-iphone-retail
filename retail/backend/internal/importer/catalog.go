// Package importer retains the old draft API only to reject accidental calls.
// Reviewed row writers live in catalogimport, merchantimport and rowimport;
// those components are not a complete, accepted import coordinator.
package importer

import (
	"errors"
	"gorm.io/gorm"
)

var ErrLegacyDisabled = errors.New("legacy v1 importer disabled; complete v2 coordinator unavailable")

// ImportCatalog never opens path or uses db. The former implementation lost
// SQL NULL provenance, fabricated timestamps and omitted business columns.
func ImportCatalog(db *gorm.DB, path string) (map[string]int, error) {
	return nil, ErrLegacyDisabled
}
