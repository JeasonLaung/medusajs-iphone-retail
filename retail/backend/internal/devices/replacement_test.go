package devices

import "testing"

func TestReplacementMustReferToOriginal(t *testing.T) {
	parent := "original"
	service := ServiceContext{Type: "exchange", RequestStatus: "processing", Status: "returned"}
	if replacementReason(service, true, true, replacementOriginal{ReplacementOf: &parent}, 0) != "not_original" {
		t.Fatal("replacement may not be its own original")
	}
	if replacementReason(service, true, true, replacementOriginal{}, 0) != "eligible" {
		t.Fatal("valid exchange")
	}
}
