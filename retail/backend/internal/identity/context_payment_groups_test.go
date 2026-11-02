package identity_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// These are explicit owned pricing/membership fixtures. Their control routes
// exist only behind the isolated fixture key, never in the application router.
func contextGroupFixture(t *testing.T, db *gorm.DB, fixture *gin.RouterGroup) func() {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if db.Exec(q, args...).Error != nil {
			t.Fatal("owned group context fixture")
		}
	}
	const group = "context-frozen-group"
	exec("UPDATE customer_group_runtime SET enabled=TRUE WHERE id=1")
	exec("INSERT INTO customer_groups(id,name,created_at,updated_at) VALUES(?,'Frozen member fixture',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", group)
	exec("INSERT INTO pricing_price_lists(id,title,description,type,status,rules_count,created_at,updated_at) VALUES('context-member-list','Owned member price','Fixture','override','active',1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO pricing_price_list_rules(id,price_list_id,attribute,value,created_at,updated_at) VALUES('context-member-price-rule','context-member-list','customer.groups.id','["context-frozen-group"]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec(`INSERT INTO pricing_prices(id,price_set_id,currency_code,amount,raw_amount,rules_count,price_list_id,created_at,updated_at) VALUES('context-member-price','add-set','eur',100,'{"value":"100","precision":100}',0,'context-member-list',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("INSERT INTO promotions(id,code,is_automatic,type,status,is_tax_inclusive,used,created_at,updated_at) VALUES('context-member-coupon','MEMBER10',FALSE,'standard','active',FALSE,0,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec(`INSERT INTO promotion_application_methods(id,promotion_id,type,target_type,allocation,value,raw_value,currency_code,created_at,updated_at) VALUES('context-member-method','context-member-coupon','fixed','items','across',10,'{"value":"10","precision":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`)
	exec("INSERT INTO promotion_rules(id,attribute,operator,created_at,updated_at) VALUES('context-member-coupon-rule','customer.groups.id','in',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
	exec("INSERT INTO promotion_rule_values(id,promotion_rule_id,value,created_at,updated_at) VALUES('context-member-coupon-value','context-member-coupon-rule',?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", group)
	exec("INSERT INTO promotion_order_rules(promotion_id,promotion_rule_id) VALUES('context-member-coupon','context-member-coupon-rule')")
	reset := func() { exec("DELETE FROM customer_group_members WHERE group_id=?", group) }
	fixture.POST("/group/:state", func(c *gin.Context) {
		state := c.Param("state")
		if state != "member" && state != "removed" {
			c.Status(400)
			return
		}
		if db.Transaction(func(tx *gorm.DB) error {
			if e := tx.Exec("DELETE FROM customer_group_members WHERE group_id=?", group).Error; e != nil {
				return e
			}
			if state == "member" {
				if e := tx.Exec("INSERT INTO customer_group_members(group_id,customer_id,created_at) SELECT ?,id,UTC_TIMESTAMP(6) FROM customer_accounts WHERE email='context-buyer@example.invalid' AND verified_at IS NOT NULL AND enabled=TRUE", group).Error; e != nil {
					return e
				}
			}
			return tx.Exec("UPDATE customer_group_runtime SET revision=revision+1 WHERE id=1").Error
		}) != nil {
			c.Status(503)
			return
		}
		c.Status(200)
	})
	return reset
}
