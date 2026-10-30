package customer

import "testing"

func TestAfterSalesCompletionGuards(t *testing.T) {
	state := "available"
	original := reviewAssignment{ID: "original", DeviceID: "device", Status: "returned", State: &state}
	request := AfterSalesRequest{ID: "request", Type: "return", Status: "processing", Items: []AfterSalesLine{{DeviceIDs: []string{"device"}}}}
	for _, status := range []string{"completed", "rejected"} {
		if checkReviewCompletion(request, nil, false, status) == nil && status == "completed" {
			t.Fatal("unreceived device completed")
		}
	}
	if checkReviewCompletion(request, []reviewAssignment{original}, false, "completed") == nil {
		t.Fatal("unrefunded return completed")
	}
	if e := checkReviewCompletion(request, []reviewAssignment{original}, true, "completed"); e != nil {
		t.Fatal(e)
	}
	if checkReviewCompletion(request, []reviewAssignment{original}, true, "rejected") == nil {
		t.Fatal("received request rejected")
	}
	for _, s := range []string{"inspection", "repair", "repair_ready"} {
		r := original
		r.State = &s
		if checkReviewCompletion(request, []reviewAssignment{r}, true, "completed") == nil {
			t.Fatal("unfinished device completed", s)
		}
	}
	request.Type = "repair"
	if checkReviewCompletion(request, []reviewAssignment{original}, false, "completed") == nil {
		t.Fatal("repair not returned")
	}
	original.Status = "shipped"
	if e := checkReviewCompletion(request, []reviewAssignment{original}, false, "completed"); e != nil {
		t.Fatal(e)
	}
	request.Type = "exchange"
	if checkReviewCompletion(request, []reviewAssignment{original}, false, "completed") == nil {
		t.Fatal("replacement not shipped")
	}
	replacement := reviewAssignment{ID: "replacement", ReplacementOf: &original.ID, Status: "shipped"}
	if e := checkReviewCompletion(request, []reviewAssignment{original, replacement}, false, "completed"); e != nil {
		t.Fatal(e)
	}
	request.Items = nil
	if e := checkReviewCompletion(request, nil, false, "completed"); e != nil {
		t.Fatal("unshipped return source behavior", e)
	}
	request.Status = "completed"
	if checkReviewCompletion(request, nil, false, "processing") == nil {
		t.Fatal("closed request reopened")
	}
}
