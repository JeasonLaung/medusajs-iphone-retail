package cart

import "testing"

func TestVerifiedPaymentTransitionsDoNotDowngradePaid(t *testing.T) {
	for _, test := range []struct {
		old, next, want string
		review          bool
	}{{"", "open", "open", false}, {"open", "pending", "pending", false}, {"pending", "paid", "paid", false}, {"authorized", "paid", "paid", false}, {"paid", "open", "paid", false}, {"paid", "authorized", "paid", false}, {"paid", "failed", "paid", true}, {"failed", "paid", "paid", true}, {"authorized", "pending", "authorized", false}, {"authorized", "expired", "authorized", true}, {"canceled", "open", "canceled", false}} {
		got, review := mergeObservedPayment(test.old, test.next)
		if got != test.want || review != test.review {
			t.Fatalf("%s -> %s: %s %v", test.old, test.next, got, review)
		}
	}
	paid := "paid"
	authorized := "authorized"
	if statusState(initialAttempt{ObservedStatus: &paid}) != "paid_pending_order" || statusState(initialAttempt{ObservedStatus: &authorized}) != "authorized" || statusState(initialAttempt{ObservedStatus: &paid, NeedsReview: true}) != "requires_review" {
		t.Fatal("payment facts confused with completion")
	}
}
