package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/promotion"
	"gorm.io/gorm"
)

// Artificial rows in the owned PromotionSchema fixture only. Never application
// customer records; values deliberately exercise exact binary comparisons.
func verifyPromotionAttributeRead(t *testing.T, ctx context.Context, gdb *gorm.DB) {
	t.Helper()
	exec := func(query string, args ...any) {
		t.Helper()
		if gdb.Exec(query, args...).Error != nil {
			t.Fatal("artificial promotion attribute setup failed")
		}
	}
	attribute := "customer_id"
	candidate := promotion.ActiveConfiguration{ID: "promo", Code: "SAVE", Type: "standard", Method: &promotion.ApplicationMethod{}, Budget: &promotion.CampaignBudget{ID: "budget", Type: "use_by_attribute", Used: "0", Attribute: &attribute}}
	contextFor := func(value string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"customer_id": value})
		return raw
	}
	read := func(candidates []promotion.ActiveConfiguration, context json.RawMessage, wantErr bool) map[string]promotion.AttributeUsageSnapshot {
		t.Helper()
		var result map[string]promotion.AttributeUsageSnapshot
		err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			result, err = promotion.ReadAttributeUsages(tx, candidates, context)
			return err
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if wantErr {
			if err == nil || result != nil {
				t.Fatal("attribute error returned partial facts")
			}
		} else if err != nil || result == nil {
			t.Fatalf("attribute preview read: %v", err)
		}
		return result
	}
	if got, err := promotion.ReadAttributeUsages(gdb, nil, json.RawMessage(`{}`)); got != nil || err != promotion.ErrUsageInput {
		t.Fatal("autocommit attribute preview accepted")
	}
	if len(read(nil, json.RawMessage(`{}`), false)) != 0 {
		t.Fatal("no candidates unexpectedly loaded private usage facts")
	}
	for _, raw := range []string{`[]`, `{}`, `{"customer_id":1}`, `{"customer_id":""}`} {
		read([]promotion.ActiveConfiguration{candidate}, json.RawMessage(raw), true)
	}
	value := "same-prefix-" + strings.Repeat("x", 512) + "A "
	queryContext := contextFor(value)
	missing := read([]promotion.ActiveConfiguration{candidate}, queryContext, false)
	if missing["budget"].Found || missing["budget"].Value != value || missing["budget"].Used != "" {
		t.Fatal("missing attribute usage was fabricated or not explicit")
	}
	for _, row := range []struct{ id, value string }{
		{"attr-main", value}, {"attr-case", strings.TrimSuffix(value, "A ") + "a "}, {"attr-space", strings.TrimSuffix(value, " ")},
	} {
		exec(`INSERT INTO promotion_budget_usages(id,attribute_value,used,budget_id,raw_used,created_at,updated_at) VALUES(?,?,1,'budget','{"value":"9007199254740993.123456789012345678901","precision":40}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, row.id, row.value)
	}
	got := read([]promotion.ActiveConfiguration{candidate, candidate}, queryContext, false)
	if len(got) != 1 || !got["budget"].Found || got["budget"].Used != "9007199254740993.123456789012345678901" {
		t.Fatal("exact attribute/raw or shared-budget dedup lost")
	}
	fallback, _ := json.Marshal(map[string]any{"customer_id": nil, "customer": map[string]string{"id": value}})
	if !reflect.DeepEqual(got, read([]promotion.ActiveConfiguration{candidate}, fallback, false)) {
		t.Fatal("nested customer fallback changed")
	}
	read([]promotion.ActiveConfiguration{candidate}, json.RawMessage(`{"customer_id":"","customer":{"id":"fallback"}}`), true)
	// Same complete value + same budget is ambiguous, even if sorted IDs differ.
	exec(`INSERT INTO promotion_budget_usages(id,attribute_value,used,budget_id,raw_used,created_at,updated_at) VALUES('attr-duplicate',?,0,'budget','{"value":"0","precision":20}',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, value)
	read([]promotion.ActiveConfiguration{candidate}, queryContext, true)
	exec("UPDATE promotion_budget_usages SET deleted_at=UTC_TIMESTAMP(6) WHERE id='attr-duplicate'")
	read([]promotion.ActiveConfiguration{candidate}, queryContext, false)
	exec("UPDATE promotion_budget_usages SET deleted_at=UTC_TIMESTAMP(6) WHERE id='attr-main'")
	if read([]promotion.ActiveConfiguration{candidate}, queryContext, false)["budget"].Found {
		t.Fatal("deleted usage record matched preview")
	}
	exec("UPDATE promotion_budget_usages SET deleted_at=NULL,raw_used='{\"value\":\"NaN\",\"precision\":20}' WHERE id='attr-main'")
	read([]promotion.ActiveConfiguration{candidate}, queryContext, true)
	exec("UPDATE promotion_budget_usages SET raw_used='{\"value\":\"1.000000000000000000001\",\"precision\":40}' WHERE id='attr-main'")
	var before, after map[string]promotion.AttributeUsageSnapshot
	err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		before, err = promotion.ReadAttributeUsages(tx, []promotion.ActiveConfiguration{candidate}, queryContext)
		if err != nil {
			return err
		}
		exec("UPDATE promotion_budget_usages SET raw_used='{\"value\":\"2\",\"precision\":20}' WHERE id='attr-main'")
		after, err = promotion.ReadAttributeUsages(tx, []promotion.ActiveConfiguration{candidate}, queryContext)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil || !reflect.DeepEqual(before, after) || before["budget"].Used != "1.000000000000000000001" {
		t.Fatal("attribute preview escaped caller snapshot")
	}
	email := "customer_email"
	conflicting := candidate
	conflicting.Budget = &promotion.CampaignBudget{ID: "budget", Type: "use_by_attribute", Attribute: &email}
	read([]promotion.ActiveConfiguration{candidate, conflicting}, json.RawMessage(`{"customer_id":"a","email":"b"}`), true)
	if facts := read([]promotion.ActiveConfiguration{conflicting}, json.RawMessage(`{"email":"artificial@example.test"}`), false); facts["budget"].Attribute != email || facts["budget"].Found {
		t.Fatal("email budget uses different fact source")
	}
	unsupported := "metadata.customer_id"
	candidate.Budget.Attribute = &unsupported
	read([]promotion.ActiveConfiguration{candidate}, queryContext, true)
	candidate.Method = nil
	if len(read([]promotion.ActiveConfiguration{candidate}, json.RawMessage(`{}`), false)) != 0 {
		t.Fatal("unapplied candidate caused attribute read")
	}
	exec("UPDATE promotion_runtime_schema SET reads_enabled=FALSE")
	read(nil, json.RawMessage(`{}`), true)
	exec("UPDATE promotion_runtime_schema SET reads_enabled=TRUE")
	exec("DELETE FROM promotion_budget_usages WHERE id LIKE 'attr-%'")
	t.Log(`PROMOTION_ATTRIBUTE_READ_REHEARSAL {"actual_sql":true,"explicit_missing_row":true,"full_binary_case_space_and_long_text":true,"exact_raw":true,"shared_budget_deduplicated":true,"duplicate_effective_usage_rejected":true,"soft_delete":true,"invalid_raw_and_missing_context_rejected":true,"caller_snapshot":true,"customer_id_and_email_only":true,"schema_and_transaction_gate":true,"customer_values_reported":false,"cart_or_checkout_integrated":false}`)
}
