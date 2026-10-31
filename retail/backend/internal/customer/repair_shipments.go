package customer

import (
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RepairShipment is a private-account projection of an immutable successful
// service receipt. It deliberately excludes inspection notes/photos, actors,
// supplier/location details and the rest of the service command payload.
type RepairShipment struct {
	ID             string    `json:"id"`
	SerialNumber   *string   `json:"serial_number"`
	TrackingNumber string    `json:"tracking_number"`
	ShippedAt      time.Time `json:"shipped_at"`
}
type repairShipmentRow struct {
	OperationID, RequestID, AssignmentID, DeviceID, ItemID, RequestJSON, ResultJSON string
	SerialNumber                                                                    *string
	CreatedAt                                                                       time.Time
}

func projectRepairShipment(row repairShipmentRow, request AfterSalesRequest) (RepairShipment, error) {
	var out RepairShipment
	// The durable wire format uses snake_case, independently of Go field names.
	var wire struct {
		Action         string `json:"action"`
		TrackingNumber string `json:"tracking_number"`
	}
	var saved struct {
		DeviceID     string `json:"device_id"`
		AssignmentID string `json:"assignment_id"`
		State        string `json:"state"`
	}
	if json.Unmarshal([]byte(row.RequestJSON), &wire) != nil || json.Unmarshal([]byte(row.ResultJSON), &saved) != nil {
		return out, ErrUnavailable
	}
	operation, e := uuid.Parse(row.OperationID)
	if e != nil || operation.Version() != 4 || operation.String() != row.OperationID || request.ID != row.RequestID || request.Type != "repair" || wire.Action != "repair_ship" || saved.State != "shipped" || saved.DeviceID != row.DeviceID || saved.AssignmentID != row.AssignmentID || row.CreatedAt.IsZero() {
		return out, ErrUnavailable
	}
	selected := false
	for _, item := range request.Items {
		if item.ItemID == row.ItemID {
			for _, device := range item.DeviceIDs {
				selected = selected || device == row.DeviceID
			}
		}
	}
	if !selected || wire.TrackingNumber == "" || strings.TrimSpace(wire.TrackingNumber) != wire.TrackingNumber || len([]rune(wire.TrackingNumber)) > 200 || strings.ContainsFunc(wire.TrackingNumber, unicode.IsControl) {
		return out, ErrUnavailable
	}
	return RepairShipment{ID: row.OperationID, SerialNumber: row.SerialNumber, TrackingNumber: wire.TrackingNumber, ShippedAt: row.CreatedAt}, nil
}

// Called only after the account/order ownership check and the customer-filtered
// request read, in that same transaction. No email or legacy account inference.
func readRepairShipments(tx *gorm.DB, orderID string, requests []AfterSalesRequest) error {
	ids := []string{}
	byID := map[string]int{}
	for i, r := range requests {
		if r.Type == "repair" {
			ids = append(ids, r.ID)
			byID[r.ID] = i
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []repairShipmentRow
	if tx.Table("device_service_receipts r").Select("r.operation_id,r.request_id,r.assignment_id,a.device_id,a.item_id,r.request_json,r.result_json,d.serial_number,r.created_at").Joins("JOIN device_assignments a ON a.id=r.assignment_id AND a.order_id=r.order_id").Joins("LEFT JOIN devices d ON d.id=a.device_id AND d.deleted_at IS NULL").Where("r.order_id=? AND r.request_id IN ? AND JSON_UNQUOTE(JSON_EXTRACT(r.request_json,'$.action'))='repair_ship'", orderID, ids).Order("r.created_at,r.operation_id").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
		return ErrUnavailable
	}
	for _, row := range rows {
		i, ok := byID[row.RequestID]
		if !ok {
			return ErrUnavailable
		}
		shipment, e := projectRepairShipment(row, requests[i])
		if e != nil {
			return e
		}
		requests[i].RepairShipments = append(requests[i].RepairShipments, shipment)
	}
	return nil
}
