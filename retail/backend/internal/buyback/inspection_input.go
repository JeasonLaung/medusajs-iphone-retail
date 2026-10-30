package buyback

import "encoding/json"

// InspectionCommand is the original device-link/final-offer input contract.
// The HTTP handler routes these actions to their own transactional device checks.
type InspectionCommand struct {
	Action   string            `json:"action"`
	ItemID   string            `json:"item_id,omitempty"`
	DeviceID string            `json:"device_id,omitempty"`
	Items    []InspectionOffer `json:"items,omitempty"`
}

type InspectionOffer struct {
	ItemID string      `json:"item_id"`
	Amount json.Number `json:"amount"`
	Notes  string      `json:"notes"`
}

func ParseInspectionCommand(raw []byte) (InspectionCommand, error) {
	var out InspectionCommand
	// Ten valid 1000-UTF16-character inspection notes may exceed 16 KiB in UTF8.
	if decodeBounded(raw, &out, 65536, "action") != nil {
		return out, ErrInput
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	switch out.Action {
	case "link_device":
		if len(fields) != 3 || fields["item_id"] == nil || fields["device_id"] == nil {
			return InspectionCommand{}, ErrInput
		}
		var e error
		out.ItemID, e = trimSubmission(out.ItemID, 1, 100)
		if e != nil {
			return InspectionCommand{}, e
		}
		out.DeviceID, e = trimSubmission(out.DeviceID, 1, 100)
		if e != nil {
			return InspectionCommand{}, e
		}
	case "final_offer":
		if len(fields) != 2 || fields["items"] == nil || len(out.Items) < 1 || len(out.Items) > 10 {
			return InspectionCommand{}, ErrInput
		}
		var items []json.RawMessage
		if json.Unmarshal(fields["items"], &items) != nil {
			return InspectionCommand{}, ErrInput
		}
		for i, rawItem := range items {
			var item InspectionOffer
			if decode(rawItem, &item, "item_id", "amount", "notes") != nil {
				return InspectionCommand{}, ErrInput
			}
			var e error
			item.ItemID, e = trimSubmission(item.ItemID, 1, 100)
			if e != nil {
				return InspectionCommand{}, e
			}
			item.Notes, e = trimSubmission(item.Notes, 3, 1000)
			if e != nil {
				return InspectionCommand{}, e
			}
			var itemFields map[string]json.RawMessage
			_ = json.Unmarshal(rawItem, &itemFields)
			// Reuse the existing original-Zod binary64/cents money semantics.
			money, e := ParseCustomerCommand(append(append([]byte(`{"action":"accept_offer","final_amount":`), itemFields["amount"]...), '}'))
			if e != nil || money.FinalAmount == nil {
				return InspectionCommand{}, ErrInput
			}
			item.Amount = *money.FinalAmount
			out.Items[i] = item
		}
	default:
		return InspectionCommand{}, ErrInput
	}
	return out, nil
}
