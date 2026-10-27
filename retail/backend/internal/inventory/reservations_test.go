package inventory

import (
	"strings"
	"testing"
)

func TestRequiredInventoryQuantity(t *testing.T) {
	for _, c := range []struct{ units, required, want string }{
		{"3", "2.5", "7.500000000000000000"},
		{"9007199254740993", "0.000000000000000001", "0.009007199254740993"},
		{"0.5", "2", "1.000000000000000000"},
		{"99999999999999999999.999999999999999999", "1", "99999999999999999999.999999999999999999"},
	} {
		got, err := RequiredInventoryQuantity(c.units, c.required)
		if err != nil || got != c.want {
			t.Fatal("inventory multiplication lost precision")
		}
	}
	for _, c := range [][2]string{{"0", "1"}, {"-1", "1"}, {"1", "0"}, {"1e3", "1"}, {"0.000000000000000001", "0.1"}, {"99999999999999999999", "2"}} {
		if _, err := RequiredInventoryQuantity(c[0], c[1]); err == nil {
			t.Fatal("invalid or overflowing quantity product accepted")
		}
	}
}

func TestReservationCommandBoundary(t *testing.T) {
	r := ReservationInput{ID: "reservation", InventoryItemID: "item", LocationID: "location", LineItemID: "line", Quantity: "1.000000000000000000"}
	cmd := ReservationCommand{OperationID: "8f8cbbd0-77d0-4c5a-9a3a-0b99111c0446", OwnerID: "order", ActorRef: "system", Kind: "reserve", Reserve: []ReservationInput{r}}
	if _, err := normalizedReservationCommand(cmd); err != nil {
		t.Fatal("valid reservation rejected")
	}
	for _, quantity := range []string{"1", "0.000000000000000000", "-1.000000000000000000", "1.0000000000000000001", "100000000000000000000.000000000000000000", "NaN"} {
		bad := cmd
		bad.Reserve = []ReservationInput{r}
		bad.Reserve[0].Quantity = quantity
		if _, err := normalizedReservationCommand(bad); err == nil {
			t.Fatal("invalid reservation amount accepted")
		}
	}
	duplicate := cmd
	duplicate.Reserve = []ReservationInput{r, r}
	if _, err := normalizedReservationCommand(duplicate); err == nil {
		t.Fatal("duplicate reservation IDs accepted")
	}
	mixed := cmd
	mixed.Release = []ReservationBinding{{ID: "other", InventoryItemID: "item", LocationID: "location", LineItemID: "line"}}
	if _, err := normalizedReservationCommand(mixed); err == nil {
		t.Fatal("mixed command accepted")
	}
	foreign := cmd
	foreign.OwnerID = strings.Repeat("x", 192)
	if _, err := normalizedReservationCommand(foreign); err == nil {
		t.Fatal("oversized owner accepted")
	}
	second := r
	second.ID = "aaa"
	unsorted := cmd
	unsorted.Reserve = []ReservationInput{r, second}
	sorted, err := normalizedReservationCommand(unsorted)
	if err != nil || sorted.Reserve[0].ID != "aaa" || unsorted.Reserve[0].ID != "reservation" {
		t.Fatal("sorting mutated shared caller request")
	}
}
