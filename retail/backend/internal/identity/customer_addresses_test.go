package identity_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

func checkCustomerAddresses(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, sessions *sessionstore.Store, token, owner string) {
	t.Helper()
	fields := func(street string) map[string]string {
		return map[string]string{"first_name": "新", "last_name": "Customer", "address_1": street, "city": "Copenhagen", "postal_code": "0012", "country_code": "dk", "phone": "+45 123", "is_default_billing": "true", "is_default_shipping": "true"}
	}
	empty, err := store.Addresses(ctx, token)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("new customer address list is not empty")
	}
	first, err := store.SaveAddress(ctx, token, "", fields("First 1"))
	if err != nil || !strings.HasPrefix(first.ID, "addr_") || !first.IsDefaultBilling || !first.IsDefaultShipping || first.PostalCode != "0012" || first.Company != "" {
		t.Fatal("new owned address creation failed")
	}
	second, err := store.SaveAddress(ctx, token, "", fields("Second 2"))
	if err != nil || second.ID == first.ID {
		t.Fatal("second address creation failed")
	}
	list, err := store.Addresses(ctx, token)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || list[0].IsDefaultBilling || list[0].IsDefaultShipping || !list[1].IsDefaultBilling {
		t.Fatal("default switch did not preserve address order/content")
	}
	changed, err := store.SaveAddress(ctx, token, first.ID, map[string]string{"phone": "", "is_default_billing": "true"})
	if err != nil || changed.Phone != "" || changed.Address1 != first.Address1 || changed.IsDefaultShipping || !changed.IsDefaultBilling {
		t.Fatal("partial address update/default roles failed")
	}
	other, err := store.Register(ctx, customer.Registration{Email: "address-other@example.invalid", Password: "fixture", FirstName: "Other", LastName: "Customer", Country: "dk", Locale: "en"})
	if err != nil {
		t.Fatal("second customer fixture failed")
	}
	// Issue only an artificial verified fixture. No real customers are modified.
	if db.Exec("UPDATE customer_accounts SET verified_at=UTC_TIMESTAMP(6) WHERE id=?", other.ID).Error != nil {
		t.Fatal("second customer fixture verification failed")
	}
	otherLogin, err := store.Login(ctx, other.Email, "fixture")
	if err != nil {
		t.Fatal("second fixture login failed")
	}
	for _, id := range []string{first.ID, "addr_00000000-0000-4000-8000-000000000001", "historical-address"} {
		if _, err := store.SaveAddress(ctx, otherLogin.Session.Token, id, map[string]string{"phone": "forbidden"}); !errors.Is(err, customer.ErrAddressMissing) {
			t.Fatal("foreign or missing address update disclosed ownership")
		}
		if err := store.DeleteAddress(ctx, otherLogin.Session.Token, id); !errors.Is(err, customer.ErrAddressMissing) {
			t.Fatal("foreign or missing address removal accepted")
		}
	}
	foreign, err := store.Addresses(ctx, otherLogin.Session.Token)
	if err != nil || len(foreign) != 0 {
		t.Fatal("foreign addresses exposed")
	}
	for _, bad := range []map[string]string{{}, {"customer_id": owner}, {"id": first.ID}, {"email": "x@y.invalid"}, {"first_name": " "}, {"phone": "x\x00y"}, {"country_code": "DK"}, {"country_code": "d"}, {"postal_code": strings.Repeat("1", 33)}, {"is_default_billing": "1"}} {
		if _, err := store.SaveAddress(ctx, token, first.ID, bad); !errors.Is(err, customer.ErrInvalid) {
			t.Fatal("invalid address fields accepted")
		}
	}
	if _, err := store.SaveAddress(ctx, token, "", map[string]string{"phone": "x"}); !errors.Is(err, customer.ErrInvalid) {
		t.Fatal("incomplete address created")
	}
	before, _ := store.Addresses(ctx, token)
	if db.Exec("CREATE TRIGGER customer_address_fixture_failure BEFORE INSERT ON customer_account_addresses FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial address failure'").Error != nil {
		t.Fatal("address late failure fixture failed")
	}
	_, failed := store.SaveAddress(ctx, token, "", fields("Must roll back"))
	if db.Exec("DROP TRIGGER customer_address_fixture_failure").Error != nil {
		t.Fatal("address late failure cleanup failed")
	}
	after, _ := store.Addresses(ctx, token)
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if !errors.Is(failed, customer.ErrUnavailable) || string(beforeJSON) != string(afterJSON) {
		t.Fatal("late address insert failure changed default/address state")
	}
	var group sync.WaitGroup
	outcomes := make(chan error, 8)
	for i := range 8 {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			id := first.ID
			if index%2 == 0 {
				id = second.ID
			}
			_, err := store.SaveAddress(ctx, token, id, map[string]string{"is_default_billing": "true"})
			outcomes <- err
		}(i)
	}
	group.Wait()
	close(outcomes)
	for err := range outcomes {
		if err != nil {
			t.Fatal("concurrent default switch failed")
		}
	}
	list, err = store.Addresses(ctx, token)
	defaults := 0
	for _, address := range list {
		if address.IsDefaultBilling {
			defaults++
		}
	}
	if err != nil || defaults != 1 {
		t.Fatal("concurrent switch produced multiple/no billing defaults")
	}
	// The SQL uniqueness guard also rejects a second default outside the command.
	if db.Exec("UPDATE customer_account_addresses SET is_default_billing=TRUE WHERE customer_id=?", owner).Error == nil {
		t.Fatal("physical multiple-default invariant absent")
	}
	if db.Exec("CREATE TRIGGER customer_address_delete_failure BEFORE DELETE ON customer_account_addresses FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial delete failure'").Error != nil {
		t.Fatal("address deletion failure fixture failed")
	}
	failed = store.DeleteAddress(ctx, token, first.ID)
	if db.Exec("DROP TRIGGER customer_address_delete_failure").Error != nil || !errors.Is(failed, customer.ErrUnavailable) {
		t.Fatal("address removal failure returned success")
	}
	list, err = store.Addresses(ctx, token)
	if err != nil || len(list) != 2 {
		t.Fatal("failed removal lost address")
	}
	if store.DeleteAddress(ctx, token, first.ID) != nil || store.DeleteAddress(ctx, token, second.ID) != nil {
		t.Fatal("owned address removal failed")
	}
	if !errors.Is(store.DeleteAddress(ctx, token, first.ID), customer.ErrAddressMissing) {
		t.Fatal("missing removal returned success")
	}
	admin, err := sessions.Issue(ctx, sessionstore.Admin, sessionstore.Principal{UserID: owner, Version: strings.Repeat("a", 64)})
	if err != nil {
		t.Fatal("admin realm fixture failed")
	}
	if _, err := store.Addresses(ctx, admin.Token); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("admin token accessed customer addresses")
	}
	if _, err := store.SaveAddress(ctx, strings.Repeat("f", 64), "", fields("Forbidden")); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("unknown session created address")
	}
	t.Log(`CUSTOMER_ADDRESSES_REHEARSAL {"real_mysql_redis":true,"owned_crud":true,"empty_list":true,"partial_update_and_phone_clear":true,"leading_zero_postal":true,"cross_customer_denied":true,"missing_same_404":true,"protected_fields_denied":true,"default_roles_independent":true,"concurrent_default_attempts":8,"single_billing_default":true,"physical_unique_default":true,"late_default_switch_rollback":true,"delete_failure_atomic":true,"admin_realm_denied":true,"unknown_session_denied":true,"mail_delivery":false,"checkout_integrated":false}`)
}
