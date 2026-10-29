package cart

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestOriginalPromotionCodeSelection(t *testing.T) {
	data, err := os.ReadFile("../../../../docs/migration/fixtures/cart-promotion-codes.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Cases []struct {
			Name, Action               string
			Existing                   []*string
			Requested, Known, Expected []string
			Error                      bool
		}
	}
	if json.Unmarshal(data, &f) != nil || len(f.Cases) != 12 {
		t.Fatal("selection fixture coverage changed")
	}
	for _, row := range f.Cases {
		t.Run(row.Name, func(t *testing.T) {
			existing := []string{}
			for _, code := range row.Existing {
				if code != nil {
					existing = append(existing, *code)
				}
			}
			got, err := PlanPromotionCodes(existing, row.Requested, row.Known, row.Action)
			if row.Error {
				if err == nil || got != nil {
					t.Fatal("unknown promotion accepted")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, row.Expected) {
				t.Fatalf("got %v %v want %v", got, err, row.Expected)
			}
		})
	}
	for _, action := range []string{"invalid", "ADD", "delete"} {
		if _, err := PlanPromotionCodes(nil, nil, nil, action); err == nil {
			t.Fatal("unknown action accepted")
		}
	}
}
