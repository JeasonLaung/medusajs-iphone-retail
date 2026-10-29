package identity_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/promotion"
	"gorm.io/gorm"
)

// Runs inside the owned promotion schema fixture, before its destructive cases.
func verifyPromotionActiveRead(t *testing.T, ctx context.Context, gdb *gorm.DB) {
	t.Helper()
	verifyPromotionAttributeRead(t, ctx, gdb)
	exec := func(query string, args ...any) {
		t.Helper()
		if err := gdb.Exec(query, args...).Error; err != nil {
			t.Fatalf("artificial promotion read setup: %v", err)
		}
	}
	at := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	read := func(codes []string, automatic bool, wantErr bool) []promotion.ActiveConfiguration {
		t.Helper()
		var out []promotion.ActiveConfiguration
		err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = promotion.ReadActive(tx, promotion.ActiveRequest{Codes: codes, IncludeAutomatic: automatic, At: at})
			return e
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if wantErr {
			if err == nil || out != nil {
				t.Fatal("invalid promotion configuration returned partial candidates")
			}
			return nil
		}
		if err != nil || out == nil {
			t.Fatalf("promotion config read: %v", err)
		}
		return out
	}
	if out, e := promotion.ReadActive(gdb, promotion.ActiveRequest{Codes: []string{"SAVE"}, At: at}); e != promotion.ErrUsageInput || out != nil {
		t.Fatal("autocommit promotion read accepted")
	}
	first := read([]string{"SAVE", "SAVE", "unknown"}, false, false)
	if len(first) != 1 || first[0].ID != "promo" || !first[0].TaxInclusive || first[0].Method == nil || first[0].Method.Value != "0.123456789012345678901" || first[0].Budget == nil || first[0].Budget.Used != "9007199254740993.123456789012345678901" || first[0].Budget.Limit != nil {
		t.Fatalf("precision/config relationship lost: %+v", first)
	}
	p := first[0]
	// The schema fixture intentionally has a null attribute on its attribute
	// budget. It must not compute until an explicit valid configuration and
	// lookup fact are supplied. Configure only this artificial integration read.
	exec("UPDATE promotion_campaign_budgets SET attribute='customer_id' WHERE id='budget'")
	var computeCandidates []promotion.ActiveConfiguration
	computeContext := json.RawMessage(`{"currency_code":"eur","customer_id":"artificial","items":[{"id":"line","product_id":"00123","subtotal":"10","original_total":"12","quantity":1}]}`)
	var computeUsages map[string]promotion.AttributeUsageSnapshot
	if err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		computeCandidates, err = promotion.ReadActive(tx, promotion.ActiveRequest{Codes: []string{"SAVE"}, At: at})
		if err != nil {
			return err
		}
		computeUsages, err = promotion.ReadAttributeUsages(tx, computeCandidates, computeContext)
		return err
	}, &sql.TxOptions{ReadOnly: true}); err != nil {
		t.Fatal("SQL attributes to computation setup failed")
	}
	exec("UPDATE promotion_campaign_budgets SET attribute=NULL WHERE id='budget'")
	computed, computeErr := promotion.ComputeCandidates(promotion.ComputeInput{Candidates: computeCandidates, Context: computeContext, Epsilon: "0.0001", AttributeUsages: computeUsages})
	if computeErr != nil || len(computed.Actions) != 1 || computed.Actions[0].PromotionID != "promo" || computed.Actions[0].ItemID != "line" || computed.Actions[0].Amount != "0.123456789012345678901" || !computed.Actions[0].TaxInclusive {
		t.Fatalf("SQL promotion to exact computation failed: %+v %v", computed, computeErr)
	}
	if len(p.Rules) != 1 || len(p.Method.BuyRules) != 1 || len(p.Method.TargetRules) != 1 || string(p.Rules[0].Values[0].Value) != `"00123"` {
		t.Fatal("three rule scopes not loaded as strings")
	}
	for _, sample := range []struct {
		context string
		want    []string
	}{
		{`{"currency_code":"eur","items":[{"id":"line","product_id":"00123","subtotal":"10","quantity":1}]}`, []string{"line"}},
		{`{"currency_code":"eur","items":[{"id":"line","product_id":"123","subtotal":"10","quantity":1}]}`, []string{}},
	} {
		qualified, err := promotion.QualifyTargets(p, json.RawMessage(sample.context))
		if err != nil || !reflect.DeepEqual(qualified.Items, sample.want) {
			t.Fatalf("SQL rules to qualification lost exact string: %+v %v", qualified, err)
		}
	}
	// Mutating one linked copy must not change another scope or another read.
	p.Rules[0].Values[0].Value[1] = '9'
	if string(p.Method.BuyRules[0].Values[0].Value) != `"00123"` {
		t.Fatal("shared linked rule aliases scope")
	}
	if again := read([]string{"SAVE"}, false, false); string(again[0].Rules[0].Values[0].Value) != `"00123"` {
		t.Fatal("read mutated persistent rules")
	}
	if rows := read(nil, false, false); len(rows) != 0 {
		t.Fatal("no codes unexpectedly loaded manual promotions")
	}
	if rows := read([]string{"save"}, false, false); len(rows) != 0 {
		t.Fatal("coupon case was coerced")
	}
	exec(`INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('auto','AUTOMATIC',TRUE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('no-method','NO-METHOD',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('draft','DRAFT',TRUE,'standard','draft',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO promotion_application_methods(id,value,raw_value,type,target_type,allocation,promotion_id,created_at,updated_at) VALUES('auto-method',10,'{"value":"10","precision":20}','percentage','order','across','auto',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	if rows := read([]string{"SAVE"}, true, false); len(rows) != 2 || rows[0].ID != "auto" || rows[1].ID != "promo" {
		t.Fatal("automatic merge or exact value ordering changed")
	}
	if rows := read([]string{"NO-METHOD"}, false, false); len(rows) != 1 || rows[0].Method != nil {
		t.Fatal("missing method fabricated")
	}
	if rows := read(nil, true, false); len(rows) != 1 || rows[0].Code != "AUTOMATIC" {
		t.Fatal("manual promo leaked into automatic query")
	}
	exec("UPDATE promotion_campaigns SET starts_at=?,ends_at=? WHERE id='campaign'", at, at.Add(time.Hour))
	if len(read([]string{"SAVE"}, false, false)) != 1 {
		t.Fatal("campaign inclusive start rejected")
	}
	exec("UPDATE promotion_campaigns SET ends_at=? WHERE id='campaign'", at)
	if len(read([]string{"SAVE"}, false, false)) != 0 {
		t.Fatal("campaign exclusive end accepted")
	}
	exec("UPDATE promotion_campaigns SET starts_at=?,ends_at=NULL WHERE id='campaign'", at.Add(time.Second))
	if len(read([]string{"SAVE"}, false, false)) != 0 {
		t.Fatal("future campaign active")
	}
	exec("UPDATE promotion_campaigns SET starts_at=NULL,deleted_at=UTC_TIMESTAMP(6) WHERE id='campaign'")
	if len(read([]string{"SAVE"}, false, false)) != 0 {
		t.Fatal("deleted campaign active")
	}
	exec("UPDATE promotion_campaigns SET deleted_at=NULL WHERE id='campaign'")
	exec("UPDATE promotions SET deleted_at=UTC_TIMESTAMP(6) WHERE id='promo'")
	if len(read([]string{"SAVE"}, false, false)) != 0 {
		t.Fatal("deleted promotion active")
	}
	exec("UPDATE promotions SET deleted_at=NULL WHERE id='promo'")
	exec("UPDATE promotion_application_methods SET deleted_at=UTC_TIMESTAMP(6) WHERE id='method'")
	if rows := read([]string{"SAVE"}, false, false); rows[0].Method != nil {
		t.Fatal("deleted method active")
	}
	exec("UPDATE promotion_application_methods SET deleted_at=NULL WHERE id='method'")
	exec("UPDATE promotion_rules SET deleted_at=UTC_TIMESTAMP(6) WHERE id='rule'")
	read([]string{"SAVE"}, false, true)
	exec("UPDATE promotion_rules SET deleted_at=NULL WHERE id='rule'")
	exec("UPDATE promotion_rule_values SET deleted_at=UTC_TIMESTAMP(6) WHERE id='value'")
	read([]string{"SAVE"}, false, true)
	exec("UPDATE promotion_rule_values SET deleted_at=NULL WHERE id='value'")
	exec("UPDATE promotion_rules SET operator='gt' WHERE id='rule'")
	exec("UPDATE promotion_rule_values SET value='not-a-number' WHERE id='value'")
	read([]string{"SAVE"}, false, true)
	exec("UPDATE promotion_rules SET operator='in' WHERE id='rule'")
	exec("UPDATE promotion_rule_values SET value='00123' WHERE id='value'")
	exec(`UPDATE promotion_application_methods SET raw_value='{"value":"NaN","precision":20}' WHERE id='method'`)
	read([]string{"SAVE"}, false, true)
	exec(`UPDATE promotion_application_methods SET raw_value='{"value":"0.123456789012345678901","precision":30}' WHERE id='method'`)
	exec("UPDATE promotion_application_methods SET max_quantity=-1 WHERE id='method'")
	read([]string{"SAVE"}, false, true)
	exec("UPDATE promotion_application_methods SET max_quantity=NULL WHERE id='method'")
	exec("UPDATE promotion_campaign_budgets SET type='spend_by_attribute' WHERE id='budget'")
	if rows := read([]string{"SAVE"}, false, false); rows[0].Budget.Type != "spend_by_attribute" {
		t.Fatal("unsupported preserved type converted into another budget")
	}
	exec("UPDATE promotion_campaign_budgets SET type='use_by_attribute' WHERE id='budget'")
	exec("UPDATE promotion_campaign_budgets SET deleted_at=UTC_TIMESTAMP(6) WHERE id='budget'")
	if rows := read([]string{"SAVE"}, false, false); rows[0].Budget != nil {
		t.Fatal("deleted budget fabricated")
	}
	exec("UPDATE promotion_campaign_budgets SET deleted_at=NULL WHERE id='budget'")
	// Internal projection does not include attribute usage identities/values.
	encoded, _ := json.Marshal(read([]string{"SAVE"}, false, false))
	var tree any
	if json.Unmarshal(encoded, &tree) != nil {
		t.Fatal("projection JSON invalid")
	}
	if reflect.DeepEqual(tree, nil) {
		t.Fatal("missing projection")
	}
	// Deliberately exceed the relation cap; read must reject, not truncate rules.
	exec(`INSERT INTO promotion_rule_values(id,promotion_rule_id,value,created_at,updated_at) SELECT CONCAT('overflow-',n), 'rule', 'a',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) FROM (WITH RECURSIVE seq AS (SELECT 1 AS n UNION ALL SELECT n+1 FROM seq WHERE n<1000) SELECT n FROM seq) numbers`)
	read([]string{"SAVE"}, false, true)
	exec("DELETE FROM promotion_rule_values WHERE id LIKE 'overflow-%'")
	exec("UPDATE promotion_runtime_schema SET reads_enabled=FALSE")
	read([]string{"SAVE"}, false, true)
	exec("UPDATE promotion_runtime_schema SET reads_enabled=TRUE")
	// Snapshot remains internally consistent until transaction end.
	var before, after []promotion.ActiveConfiguration
	err := gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		before, e = promotion.ReadActive(tx, promotion.ActiveRequest{Codes: []string{"SAVE"}, At: at})
		if e != nil {
			return e
		}
		exec("UPDATE promotion_rule_values SET value='changed-after-snapshot' WHERE id='value'")
		after, e = promotion.ReadActive(tx, promotion.ActiveRequest{Codes: []string{"SAVE"}, At: at})
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("promotion read escaped caller snapshot")
	}
	exec("UPDATE promotion_rule_values SET value='00123' WHERE id='value'")
	// Remove artificial unrelated promotions before the original schema cases.
	exec("DELETE FROM promotions WHERE id IN ('auto','no-method','draft')")
	t.Log(`PROMOTION_READ_REHEARSAL {"migrations":56,"actual_sql_configuration":true,"manual_automatic_merge":true,"case_sensitive_codes":true,"active_campaign_time_and_soft_delete":true,"raw_precision_and_nullable_limit":true,"three_rule_scopes":true,"scope_rule_copy_isolation":true,"missing_methods_not_fabricated":true,"deleted_restriction_and_empty_values_rejected":true,"invalid_range_raw_quantity_rejected":true,"unsupported_budget_preserved_not_activated":true,"relation_overflow_rejected":true,"caller_snapshot_preserved":true,"schema_gate_and_autocommit_rejected":true,"qualification_allocation_or_checkout_integrated":false}`)
}
