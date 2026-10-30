package identity_test

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/buyback"
	"gorm.io/gorm"
	"testing"
)

func checkBuybackPreparation(t *testing.T, db *gorm.DB) {
	t.Helper()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal("buyback preparation transaction")
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) {
		t.Helper()
		if tx.Exec(q, args...).Error != nil {
			t.Fatal("buyback preparation setup")
		}
	}
	exec("UPDATE catalog_products SET title='iPhone 13',handle='swappie-iphone-13',status='published' WHERE id='public-a'")
	exec(`INSERT INTO catalog_option_values(id,value,option_id,created_at,updated_at) VALUES('buyback-256','256 GB','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('buyback-1024','1024 GB','public-storage',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO catalog_product_option_values(id,product_product_option_id,product_option_value_id,created_at,updated_at) VALUES('buyback-member-256','public-membership','buyback-256',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)),('buyback-member-1024','public-membership','buyback-1024',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	raw := `{"submission_key":"123e4567-e89b-42d3-a456-426614174000","contact_name":"Customer","phone":"123456","return_address":{"name":"Customer","address_1":"Street 1","city":"City","postal_code":"001","country_code":"dk"},"reference_acknowledged":true,"privacy_acknowledged":true,"items":[{"model":"iphone-13","capacity":"256GB","assessment":{"battery":"90","checks":[true,true,true,true,true],"faults":["none"],"glass":["none"],"screen":"none","frame":"none","back":"none"}}]}`
	in, e := buyback.ParseSubmission([]byte(raw))
	if e != nil {
		t.Fatal("buyback preparation input")
	}
	prepare := func() buyback.PreparedSubmission {
		t.Helper()
		out, e := buyback.PrepareSubmission(context.Background(), tx, in)
		if e != nil {
			t.Fatal("buyback preparation failed")
		}
		return out
	}
	out := prepare()
	if len(out.Items) != 1 || out.Items[0].ProductID != "public-a" || out.Items[0].ModelName != "iPhone 13" || out.Items[0].ReferenceAmount == nil || *out.Items[0].ReferenceAmount != "294" || out.ReferenceAmount == nil || *out.ReferenceAmount != "294" || out.Items[0].FinalAmount != nil || out.Items[0].DeviceID != nil {
		t.Fatal("buyback trusted snapshot differs")
	}
	if second := prepare(); second.Items[0].ID == out.Items[0].ID {
		t.Fatal("buyback item identities collided")
	}
	in.Items = append(in.Items, in.Items[0])
	out = prepare()
	if out.ReferenceAmount == nil || *out.ReferenceAmount != "588" || out.Items[0].ID == out.Items[1].ID {
		t.Fatal("multiple phone snapshot")
	}
	in.Items[1].Capacity = "1TB"
	out = prepare()
	if out.ReferenceAmount == nil || *out.ReferenceAmount != "314" {
		t.Fatal("capacity equivalent option not accepted")
	}
	exec("UPDATE buyback_prices SET deleted_at=UTC_TIMESTAMP(6) WHERE product_id='public-a' AND capacity='1TB'")
	out = prepare()
	if out.ReferenceAmount != nil || out.Items[1].ReferenceAmount != nil {
		t.Fatal("unconfigured estimate became zero")
	}
	in.Items = in.Items[:1]
	in.Items[0].Capacity = "512GB"
	if _, e := buyback.PrepareSubmission(context.Background(), tx, in); !errors.Is(e, buyback.ErrInput) {
		t.Fatal("unlinked capacity accepted")
	}
	in.Items[0].Capacity = "256GB"
	// Complete wire schema allows irrelevant grade text on skipped branches.
	in.Items[0].Assessment.Faults = []string{"pixels"}
	in.Items[0].Assessment.Screen = "unused"
	in.Items[0].Assessment.Frame = "damaged"
	in.Items[0].Assessment.Back = "unused"
	prepare()
	in.Items[0].Assessment.Faults = []string{"none"}
	in.Items[0].Assessment.Screen = "none"
	in.Items[0].Assessment.Frame = "none"
	in.Items[0].Assessment.Back = "none"
	for _, change := range []string{"UPDATE catalog_products SET status='draft' WHERE id='public-a'", "UPDATE catalog_products SET title='iPhones 13' WHERE id='public-a'", "UPDATE catalog_product_option_values SET deleted_at=UTC_TIMESTAMP(6) WHERE id='buyback-member-256'"} {
		exec("SAVEPOINT invalid_model")
		exec(change)
		if _, e := buyback.PrepareSubmission(context.Background(), tx, in); !errors.Is(e, buyback.ErrInput) {
			t.Fatal("ineligible model snapshot accepted")
		}
		exec("ROLLBACK TO SAVEPOINT invalid_model")
	}
	if tx.Rollback().Error != nil {
		t.Fatal("buyback preparation rollback")
	}
	var count int64
	if db.Table("catalog_option_values").Where("id='buyback-256'").Count(&count).Error != nil || count != 0 {
		t.Fatal("buyback preparation escaped transaction")
	}
	t.Log(`BUYBACK_PREPARE_REHEARSAL {"trusted_catalog":true,"current_rules":true,"multiple_phones":true,"capacity_membership":true,"unconfigured_null":true,"caller_rollback":true,"application_writes":false}`)
}
