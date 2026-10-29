package identity_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/customer"
	"gorm.io/gorm"
)

func checkCustomerProfile(t *testing.T, ctx context.Context, db *gorm.DB, store *customer.Store, token, id string) {
	t.Helper()
	before, err := store.Current(ctx, token)
	if err != nil {
		t.Fatal("profile fixture session unavailable")
	}
	changed, err := store.UpdateProfile(ctx, token, map[string]string{"first_name": "新客户", "last_name": "Updated"})
	if err != nil || changed.ID != id || changed.FirstName != "新客户" || changed.LastName != "Updated" || changed.Phone != before.Phone || changed.Email != before.Email {
		t.Fatal("name update changed omitted or protected fields")
	}
	changed, err = store.UpdateProfile(ctx, token, map[string]string{"phone": "+45 123 456"})
	if err != nil || changed.FirstName != "新客户" || changed.Phone != "+45 123 456" {
		t.Fatal("partial phone update failed")
	}
	changed, err = store.UpdateProfile(ctx, token, map[string]string{"phone": ""})
	if err != nil || changed.Phone != "" {
		t.Fatal("explicit phone clearing rejected")
	}
	if _, err := store.Current(ctx, token); err != nil {
		t.Fatal("profile update invalidated valid session")
	}
	for _, fields := range []map[string]string{{}, {"email": "other@example.invalid"}, {"id": "old-customer-same-email"}, {"password": "x"}, {"role": "admin"}, {"first_name": " "}, {"last_name": strings.Repeat("字", 161)}, {"phone": "x\x00y"}} {
		if _, err := store.UpdateProfile(ctx, token, fields); !errors.Is(err, customer.ErrInvalid) {
			t.Fatal("unsupported profile mutation accepted")
		}
	}
	if _, err := store.UpdateProfile(ctx, strings.Repeat("f", 64), map[string]string{"phone": "forbidden"}); !errors.Is(err, customer.ErrUnauthorized) {
		t.Fatal("unknown customer profile authority accepted")
	}
	if db.WithContext(ctx).Exec("CREATE TRIGGER customer_profile_fixture_failure BEFORE UPDATE ON customer_accounts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='artificial profile failure'").Error != nil {
		t.Fatal("profile failure fixture unavailable")
	}
	_, failed := store.UpdateProfile(ctx, token, map[string]string{"first_name": "must roll back", "phone": "must roll back"})
	if db.WithContext(ctx).Exec("DROP TRIGGER customer_profile_fixture_failure").Error != nil {
		t.Fatal("profile failure fixture cleanup failed")
	}
	if !errors.Is(failed, customer.ErrUnavailable) {
		t.Fatal("profile storage failure returned success")
	}
	current, err := store.Current(ctx, token)
	if err != nil || current.FirstName != "新客户" || current.Phone != "" || current.ID != id {
		t.Fatal("failed profile transaction changed fields")
	}
	var historical string
	if db.Raw("SELECT email FROM historical_customer_subjects WHERE id='old-customer-same-email'").Scan(&historical).Error != nil || historical != before.Email {
		t.Fatal("same-email historical subject changed by profile write")
	}
	t.Log(`CUSTOMER_PROFILE_REHEARSAL {"real_mysql":true,"current_customer_only":true,"partial_name_phone_update":true,"explicit_empty_phone":true,"email_id_role_password_denied":true,"field_validation":true,"session_remains_valid":true,"unknown_session_denied":true,"write_failure_atomic":true,"historical_subject_unchanged":true,"profile_ui_integrated":false}`)
}
