package refunds

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCancellationBoundaryAndReasonMetadata(t *testing.T) {
	good := CancellationInput{OperationID: "00000000-0000-4000-8000-000000000001", ExpectedVersion: "1", Reason: "Customer requested cancellation"}
	if !validCancellation(good) {
		t.Fatal("valid instruction rejected")
	}
	for _, mutate := range []func(*CancellationInput){
		func(v *CancellationInput) { v.OperationID = "00000000-0000-1000-8000-000000000001" },
		func(v *CancellationInput) { v.ExpectedVersion = "0" },
		func(v *CancellationInput) { v.ExpectedVersion = "01" },
		func(v *CancellationInput) { v.ExpectedVersion = "2147483648" },
		func(v *CancellationInput) { v.Reason = " " },
		func(v *CancellationInput) { v.Reason = strings.Repeat("中", 1001) },
		func(v *CancellationInput) { v.Reason = string([]byte{0xff}) },
	} {
		bad := good
		mutate(&bad)
		if validCancellation(bad) {
			t.Fatal("unsafe instruction accepted")
		}
	}
	previous := `{"unrelated":{"amount":"1234567890.123456789012345678"},"cancellation":{"reason":"old"}}`
	reason := "客户要求\n\"取消\" <script>"
	root := orderCancellation{Reason: reason, ActorID: "current-admin"}
	now := time.Date(2026, 11, 3, 1, 2, 3, 4000, time.UTC)
	wire, err := cancellationMetadata(&previous, root, now)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if json.Unmarshal([]byte(wire), &out) != nil || string(out["unrelated"]) != `{"amount":"1234567890.123456789012345678"}` {
		t.Fatal("unrelated exact metadata changed")
	}
	var recorded struct{ Reason, ActorID, RecordedAt string }
	var fields map[string]string
	if json.Unmarshal(out["cancellation"], &fields) != nil {
		t.Fatal("reason is not JSON")
	}
	recorded.Reason, recorded.ActorID, recorded.RecordedAt = fields["reason"], fields["actor_id"], fields["recorded_at"]
	if recorded.Reason != reason || recorded.ActorID != root.ActorID || recorded.RecordedAt != now.Format(time.RFC3339Nano) {
		t.Fatal("reason/audit escaped or altered")
	}
	malformed := "[]"
	if _, err := cancellationMetadata(&malformed, root, now); err == nil {
		t.Fatal("malformed prior metadata overwritten")
	}
}
