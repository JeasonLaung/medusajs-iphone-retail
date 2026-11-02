package historyimport

import (
	"context"
	"database/sql"

	"github.com/eurorefurb/retail/internal/catalogimport"
)

type DevicePrerequisiteReport struct {
	SourceRows               int  `json:"source_rows"`
	TrackingRows             int  `json:"tracking_rows"`
	SupplyRows               int  `json:"supply_rows"`
	DeviceIdentitiesImported bool `json:"device_identities_imported"`
	CommandsEnabled          bool `json:"commands_enabled"`
}

// StageDevicePrerequisites follows Stage in the same caller transaction. The
// gate row lock serializes against device operations; runtime gates must remain
// off. All references are required, including parents of deleted facts. This
// does not import unresolved device identities or infer quantity inventory.
func (p *Snapshot) StageDevicePrerequisites(ctx context.Context, tx *sql.Tx) (DevicePrerequisiteReport, error) {
	if p == nil || tx == nil || len(p.devicePrerequisites) != 3 {
		return DevicePrerequisiteReport{}, ErrPolicy
	}
	var version int
	var revision uint64
	var identity, commands bool
	if tx.QueryRowContext(ctx, "SELECT version,revision,identity_enabled,commands_enabled FROM device_runtime_schema WHERE id=1 FOR UPDATE").Scan(&version, &revision, &identity, &commands) != nil || version != 1 || revision == 0 || identity || commands {
		return DevicePrerequisiteReport{}, ErrPolicy
	}
	var result DevicePrerequisiteReport
	for _, name := range catalogimport.DevicePrerequisiteTables() {
		rows := p.devicePrerequisites[name]
		var count int
		if tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+p.targets[name]+"`").Scan(&count) != nil {
			return DevicePrerequisiteReport{}, ErrPolicy
		}
		if count != 0 && count != len(rows) {
			return DevicePrerequisiteReport{}, ErrConflict
		}
		for _, row := range rows {
			if err := catalogimport.ImportDevicePrerequisite(ctx, tx, p.report.SnapshotSHA, name, row); err != nil {
				return DevicePrerequisiteReport{}, err
			}
		}
		switch name {
		case "device_source":
			result.SourceRows = len(rows)
		case "device_variant":
			result.TrackingRows = len(rows)
		case "device_supply":
			result.SupplyRows = len(rows)
		}
	}
	return result, nil
}
