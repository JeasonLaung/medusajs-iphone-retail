package catalogimport

import (
	"context"
	"database/sql"

	"github.com/eurorefurb/retail/internal/rowimport"
)

// Only source/tracking/supply facts are included. Device identities, assignments,
// events, photos and runtime identity claims require their separate audit.
var devicePrerequisiteOrder = []string{"device_source", "device_variant", "device_supply"}

func DevicePrerequisiteTables() []string { return append([]string(nil), devicePrerequisiteOrder...) }

// ImportDevicePrerequisite requires complete verified source and closed device
// runtime gates in the caller transaction. It does not change quantities,
// create devices, claim SN/IMEI or enable device commands. Any error requires
// rolling back the complete caller transaction.
func ImportDevicePrerequisite(ctx context.Context, tx *sql.Tx, sha, name string, row rowimport.SourceRow) error {
	if tx == nil {
		return ErrPolicy
	}
	allowed := false
	for _, n := range devicePrerequisiteOrder {
		if name == n {
			allowed = true
			break
		}
	}
	if !allowed {
		return ErrPolicy
	}
	all, err := historyLayouts()
	if err != nil {
		return ErrPolicy
	}
	l, ok := all[name]
	if !ok || len(l.Primary) != 1 || l.Primary[0] != "id" {
		return ErrPolicy
	}
	if _, err = rowimport.Convert(name, row); err != nil {
		return ErrPolicy
	}
	return importReferencedHistory(ctx, tx, sha, name, row, l, all)
}
