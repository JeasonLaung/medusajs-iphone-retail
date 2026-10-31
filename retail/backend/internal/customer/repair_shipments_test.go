package customer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRepairShipmentProjectionRejectsForeignOrUnsuccessfulReceipts(t *testing.T) {
	sn := "ORIGINAL-SN"
	row := repairShipmentRow{OperationID: "281ea420-4422-4bb6-86ee-f0c1b6cb09f5", RequestID: "asr_owned", AssignmentID: "assignment", DeviceID: "device", ItemID: "line", SerialNumber: &sn, CreatedAt: time.Now().UTC(), RequestJSON: `{"action":"repair_ship","tracking_number":"00012345678901234567890","notes":"PRIVATE","photos":["PRIVATE"]}`, ResultJSON: `{"state":"shipped","device_id":"device","assignment_id":"assignment","private":"PRIVATE"}`}
	request := AfterSalesRequest{ID: "asr_owned", Type: "repair", Items: []AfterSalesLine{{ItemID: "line", DeviceIDs: []string{"device"}}}}
	projected, e := projectRepairShipment(row, request)
	if e != nil || projected.TrackingNumber != "00012345678901234567890" || projected.SerialNumber == nil || *projected.SerialNumber != sn {
		t.Fatal("valid original repair receipt projection", e)
	}
	raw, _ := json.Marshal(projected)
	for _, private := range []string{"PRIVATE", "assignment", "device_id", "photos", "notes", "request_id"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private receipt field leaked", private)
		}
	}
	for name, alter := range map[string]func(*repairShipmentRow, *AfterSalesRequest){
		"other request":     func(r *repairShipmentRow, _ *AfterSalesRequest) { r.RequestID = "asr_other" },
		"other type":        func(_ *repairShipmentRow, q *AfterSalesRequest) { q.Type = "return" },
		"unselected device": func(r *repairShipmentRow, _ *AfterSalesRequest) { r.DeviceID = "other" },
		"other item":        func(r *repairShipmentRow, _ *AfterSalesRequest) { r.ItemID = "other" },
		"unsuccessful": func(r *repairShipmentRow, _ *AfterSalesRequest) {
			r.ResultJSON = `{"state":"repair_ready","device_id":"device","assignment_id":"assignment"}`
		},
		"wrong assignment": func(r *repairShipmentRow, _ *AfterSalesRequest) { r.AssignmentID = "other" },
		"inspection": func(r *repairShipmentRow, _ *AfterSalesRequest) {
			r.RequestJSON = `{"action":"inspect","tracking_number":"TRACK"}`
		},
		"bad json":         func(r *repairShipmentRow, _ *AfterSalesRequest) { r.RequestJSON = "{" },
		"missing tracking": func(r *repairShipmentRow, _ *AfterSalesRequest) { r.RequestJSON = `{"action":"repair_ship"}` },
		"whitespace": func(r *repairShipmentRow, _ *AfterSalesRequest) {
			r.RequestJSON = `{"action":"repair_ship","tracking_number":" TRACK "}`
		},
		"control": func(r *repairShipmentRow, _ *AfterSalesRequest) {
			r.RequestJSON = `{"action":"repair_ship","tracking_number":"TRACK\n"}`
		},
		"missing timestamp": func(r *repairShipmentRow, _ *AfterSalesRequest) { r.CreatedAt = time.Time{} },
		"bad operation":     func(r *repairShipmentRow, _ *AfterSalesRequest) { r.OperationID = "arbitrary" },
	} {
		t.Run(name, func(t *testing.T) {
			r, q := row, request
			alter(&r, &q)
			if _, e := projectRepairShipment(r, q); e == nil {
				t.Fatal("invalid repair receipt accepted")
			}
		})
	}
	row.SerialNumber = nil
	if _, e := projectRepairShipment(row, request); e != nil {
		t.Fatal("historically deleted device hides no durable tracking", e)
	}
}
