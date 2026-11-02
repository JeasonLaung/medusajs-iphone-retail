package productreviews

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// This tests participants against real imported catalog rows and explicit 0094.
// Customer session/HTTP/browser acceptance is intentionally a separate test.
func TestNativeProductReviews(t *testing.T) {
	if os.Getenv("PRODUCT_REVIEWS_NATIVE_REHEARSAL") != "1" {
		t.Skip("dedicated product review MySQL fixture required")
	}
	db, e := platform.Open()
	if e != nil {
		t.Fatal("fixture unavailable")
	}
	pool, _ := db.DB()
	defer pool.Close()
	var name string
	if db.Raw("SELECT DATABASE()").Scan(&name).Error != nil || !strings.HasPrefix(name, "eurorefurb_product_create_") {
		t.Fatal("owned dedicated database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if e := db.Exec(sql, args...).Error; e != nil {
			t.Fatal("owned fixture statement failed", e)
		}
	}
	var store, product string
	if db.Table("commerce_stores").Select("id").Where("deleted_at IS NULL").Order("id").Limit(1).Scan(&store).Error != nil || store == "" {
		t.Fatal("imported trusted store required")
	}
	if db.Table("catalog_products p").Select("p.id").Joins("JOIN catalog_product_channels c ON c.product_id=p.id AND c.deleted_at IS NULL").Joins("JOIN commerce_stores s ON s.default_sales_channel_id=c.sales_channel_id AND s.id=?", store).Where("p.status='published' AND p.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM product_reviews r WHERE r.product_id=p.id)").Order("p.id").Limit(1).Scan(&product).Error != nil || product == "" {
		t.Fatal("imported published product required")
	}
	s := Service{DB: db, StoreID: store}
	var actor identity.User
	if db.Where("email=?", "native-product-create@example.test").Take(&actor).Error != nil {
		t.Fatal("fixture admin required")
	}
	newAccount := func() string {
		t.Helper()
		id := "customer-review_" + uuid.NewString()
		exec("INSERT INTO customer_accounts(id,email,password_hash,first_name,last_name,enabled,auth_version,verified_at,created_at,updated_at) VALUES(?,?,?,'小明','Private family',TRUE,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, id+"@example.test", "fixture-not-a-login-hash")
		return id
	}
	submit := func(account string, in Submission) (Result, error) {
		var out Result
		e := db.Transaction(func(tx *gorm.DB) error {
			var row struct{ ID string }
			if e := tx.Table("customer_accounts").Select("id").Where("id=?", account).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error; e != nil {
				return e
			}
			var e error
			out, e = s.SubmitTx(ctx, tx, account, "  小明  ", in)
			return e
		})
		return out, e
	}
	in := func() Submission {
		return Submission{OperationID: uuid.NewString(), ProductID: product, Rating: 5, Title: "真实评价", Content: "这是独立数据库中的商品评价验证内容。"}
	}
	count := func(table string) int64 {
		t.Helper()
		var n int64
		if db.Table(table).Count(&n).Error != nil {
			t.Fatal("count failed")
		}
		return n
	}
	exec("UPDATE product_review_runtime SET reads_enabled=FALSE,writes_enabled=FALSE WHERE id=1")
	account := newAccount()
	input := in()
	if _, e := s.SubmitTx(ctx, db, account, "First", input); !errors.Is(e, ErrUnavailable) {
		t.Fatal("autocommit participant accepted", e)
	}
	if _, e := submit(account, input); !errors.Is(e, ErrUnavailable) {
		t.Fatal("closed default gate accepted submission", e)
	}
	if _, e := s.PublicPage(ctx, ListQuery{ProductID: product, Limit: 5}); !errors.Is(e, ErrUnavailable) {
		t.Fatal("closed default read gate accepted")
	}
	exec("UPDATE product_review_runtime SET reads_enabled=TRUE,writes_enabled=TRUE WHERE id=1")
	physical := map[string]string{}
	for _, table := range []string{"inventory_levels", "inventory_reservations", "pricing_prices", "device_assignments", "payments", "payment_captures"} {
		physical[table], e = nativeFactsSHA(db, table)
		if e != nil {
			t.Fatal("physical snapshot failed", table, e)
		}
	}
	// Historical actors remain public snapshots, never new account ownership or
	// a duplicate-submission veto based on matching actor ID/email.
	legacyID := "rev-legacy_" + uuid.NewString()
	exec("INSERT INTO product_reviews(id,product_id,customer_id,author,rating,title,content,status,verified_purchase,created_at,updated_at) VALUES(?,?,?,'Historical',1,'历史评价','保留历史评价内容','approved',FALSE,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", legacyID, product, account)
	results := make([]Result, 4)
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = submit(account, input) }(i)
	}
	wg.Wait()
	fresh := 0
	for i, r := range results {
		if errs[i] != nil || r.ID != results[0].ID || r.Status != "pending" {
			t.Fatal("concurrent original command failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("submission did not commit exactly once")
	}
	// Different operation IDs still serialize on the authenticated account.
	raceAccount := newAccount()
	raceErrors := make([]error, 4)
	for i := range raceErrors {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, raceErrors[i] = submit(raceAccount, in()) }(i)
	}
	wg.Wait()
	winners := 0
	for _, e := range raceErrors {
		if e == nil {
			winners++
		} else if !errors.Is(e, ErrDuplicate) {
			t.Fatal("different-command race failed", e)
		}
	}
	if winners != 1 {
		t.Fatal("multiple active reviews for one account/product")
	}
	reviewID := results[0].ID
	if _, e := submit(account, in()); !errors.Is(e, ErrDuplicate) {
		t.Fatal("second pending review accepted", e)
	}
	other := newAccount()
	if _, e := submit(other, input); !errors.Is(e, ErrConflict) {
		t.Fatal("cross-account receipt leaked", e)
	}
	changed := input
	changed.Rating = 3
	if _, e := submit(account, changed); !errors.Is(e, ErrConflict) {
		t.Fatal("changed payload replayed", e)
	}
	page, e := s.PublicPage(ctx, ListQuery{ProductID: product, Limit: 5})
	if e != nil || page.Count != 1 || page.Average != 1 || len(page.Reviews) != 1 || page.Reviews[0].ID != legacyID {
		t.Fatal("pending review exposed publicly", e)
	}
	admin, e := s.AdminPage(ctx, actor, ListQuery{ProductID: product, Limit: 5})
	if e != nil || admin.Count != 3 {
		t.Fatal("admin review list failed", e)
	}
	find := func(id string) AdminReview {
		t.Helper()
		p, e := s.AdminPage(ctx, actor, ListQuery{ProductID: product, Limit: 20})
		if e != nil {
			t.Fatal("admin reload failed", e)
		}
		for _, r := range p.Reviews {
			if r.ID == id {
				return r
			}
		}
		t.Fatal("admin row missing")
		return AdminReview{}
	}
	moderation := Moderation{OperationID: uuid.NewString(), ExpectedRevision: find(reviewID).Revision, Status: "approved"}
	originalModeration := moderation
	for i := range results {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = s.Moderate(ctx, actor, reviewID, moderation) }(i)
	}
	wg.Wait()
	fresh = 0
	for i, r := range results {
		if errs[i] != nil || r.Status != "approved" {
			t.Fatal("concurrent moderation failed", errs[i])
		}
		if !r.Replayed {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatal("moderation not unique")
	}
	page, e = s.PublicPage(ctx, ListQuery{ProductID: product, Limit: 1})
	if e != nil || page.Count != 2 || len(page.Reviews) != 1 || page.Average != 3 || page.Distribution[0].Count != 1 || page.Distribution[4].Count != 1 {
		t.Fatal("approved aggregate/pagination incorrect", e)
	}
	moderation.OperationID = uuid.NewString()
	moderation.Status = "rejected"
	if _, e := s.Moderate(ctx, actor, reviewID, moderation); !errors.Is(e, ErrConflict) {
		t.Fatal("stale moderation overwritten", e)
	}
	// A late command/audit write failure must remove the review/status change.
	fail := errors.New("injected late review receipt failure")
	if e := db.Callback().Create().Before("gorm:create").Register("reviews:late", func(tx *gorm.DB) {
		if tx.Statement.Table == "product_review_commands" {
			tx.AddError(fail)
		}
	}); e != nil {
		t.Fatal(e)
	}
	rollbackAccount := newAccount()
	rollbackInput := in()
	before := count("product_reviews")
	if _, e := submit(rollbackAccount, rollbackInput); e == nil || count("product_reviews") != before {
		t.Fatal("late receipt failure left a review")
	}
	moderation.ExpectedRevision = find(reviewID).Revision
	if _, e := s.Moderate(ctx, actor, reviewID, moderation); e == nil || find(reviewID).Status != "approved" {
		t.Fatal("late receipt failure changed approval")
	}
	if e := db.Callback().Create().Remove("reviews:late"); e != nil {
		t.Fatal(e)
	}
	if _, e := submit(rollbackAccount, rollbackInput); e != nil {
		t.Fatal("rolled-back operation cannot retry", e)
	}
	for _, status := range []string{"rejected", "pending", "approved"} {
		current := find(reviewID)
		_, e := s.Moderate(ctx, actor, reviewID, Moderation{OperationID: uuid.NewString(), ExpectedRevision: current.Revision, Status: status})
		if e != nil {
			t.Fatal("source moderation transition failed", status, e)
		}
		public, e := s.PublicPage(ctx, ListQuery{ProductID: product, Limit: 5})
		want := int64(1)
		if status == "approved" {
			want = 2
		}
		if e != nil || public.Count != want {
			t.Fatal("moderation/public visibility mismatch", status, e)
		}
	}
	var duplicate reviewRecord
	if db.Where("id=?", reviewID).Take(&duplicate).Error != nil {
		t.Fatal("review reload failed")
	}
	duplicate.ID = "rev-duplicate_" + uuid.NewString()
	if db.Create(&duplicate).Error == nil {
		t.Fatal("physical unique active-account review constraint missing")
	}
	duplicate.AccountID = nil
	duplicate.CustomerID = account
	duplicate.ID = "rev-duplicate-legacy_" + uuid.NewString()
	if db.Create(&duplicate).Error == nil {
		t.Fatal("physical unique active-legacy review constraint missing")
	}
	exec("UPDATE product_reviews SET deleted_at=UTC_TIMESTAMP(6) WHERE account_id=? AND product_id=? AND deleted_at IS NULL", raceAccount, product)
	if _, e := submit(raceAccount, in()); e != nil {
		t.Fatal("soft-deleted review still occupies submission eligibility", e)
	}
	// Caller rollback also removes the durable idempotency receipt.
	rollbackAccount = newAccount()
	rollbackInput = in()
	before = count("product_reviews")
	beforeCommands := count("product_review_commands")
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := s.SubmitTx(ctx, tx, rollbackAccount, "First", rollbackInput); e != nil {
			return e
		}
		return fail
	})
	if !errors.Is(e, fail) || count("product_reviews") != before || count("product_review_commands") != beforeCommands {
		t.Fatal("participant committed outside caller transaction")
	}
	if _, e := submit(rollbackAccount, rollbackInput); e != nil {
		t.Fatal("caller rollback receipt survived", e)
	}
	// Fixed new-account ownership, source payment statuses, current order version.
	seedOrder := func(owner string, withOwnership bool) (string, string) {
		t.Helper()
		id := "order-review_" + uuid.NewString()
		pc := "pc-review_" + uuid.NewString()
		line := "line-review_" + uuid.NewString()
		stamp := time.Now().UTC()
		if e := db.Table("orders").Create(map[string]any{"id": id, "customer_id": owner, "version": 1, "status": "pending", "is_draft_order": false, "currency_code": "eur", "created_at": stamp, "updated_at": stamp}).Error; e != nil {
			t.Fatal("order seed failed", e)
		}
		if withOwnership {
			exec("INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) VALUES(?,?,?,?,UTC_TIMESTAMP(6))", id, owner, "cart-review_"+uuid.NewString(), store)
		}
		if e := db.Table("payment_collections").Create(map[string]any{"id": pc, "currency_code": "eur", "amount": "5", "raw_amount": `{"value":"5","precision":20}`, "status": "completed", "created_at": stamp, "updated_at": stamp}).Error; e != nil {
			t.Fatal("payment seed failed", e)
		}
		exec("INSERT INTO order_payment_collections(order_id,payment_collection_id,id,created_at,updated_at) VALUES(?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", id, pc, uuid.NewString())
		if e := db.Table("order_line_items").Create(map[string]any{"id": line, "product_id": product, "title": "Review purchase", "requires_shipping": true, "is_discountable": true, "is_tax_inclusive": false, "unit_price": "5", "raw_unit_price": `{"value":"5","precision":20}`, "is_custom_price": false, "is_giftcard": false, "created_at": stamp, "updated_at": stamp}).Error; e != nil {
			t.Fatal("line seed failed", e)
		}
		item := map[string]any{"id": "item-review_" + uuid.NewString(), "order_id": id, "version": 1, "item_id": line, "created_at": stamp, "updated_at": stamp}
		for _, key := range []string{"quantity", "fulfilled_quantity", "shipped_quantity", "return_requested_quantity", "return_received_quantity", "return_dismissed_quantity", "written_off_quantity", "delivered_quantity"} {
			v := "0"
			if key == "quantity" {
				v = "1"
			}
			item[key] = v
			item["raw_"+key] = `{"value":"` + v + `","precision":20}`
		}
		if e := db.Table("order_items").Create(item).Error; e != nil {
			t.Fatal("order item seed failed", e)
		}
		return id, pc
	}
	purchaser := newAccount()
	order, pc := seedOrder(purchaser, true)
	purchaseInput := in()
	purchaseInput.OrderID = &order
	if _, e := submit(other, purchaseInput); !errors.Is(e, ErrOrder) {
		t.Fatal("foreign order accepted", e)
	}
	legacyOrder, _ := seedOrder(other, false)
	legacyInput := in()
	legacyInput.OrderID = &legacyOrder
	if _, e := submit(other, legacyInput); !errors.Is(e, ErrOrder) {
		t.Fatal("legacy orders.customer_id granted ownership", e)
	}
	exec("UPDATE payment_collections SET status='awaiting' WHERE id=?", pc)
	if _, e := submit(purchaser, purchaseInput); !errors.Is(e, ErrIneligible) {
		t.Fatal("pending payment granted verified badge", e)
	}
	exec("UPDATE payment_collections SET status='completed' WHERE id=?", pc)
	exec("UPDATE orders SET status='canceled' WHERE id=?", order)
	if _, e := submit(purchaser, purchaseInput); !errors.Is(e, ErrIneligible) {
		t.Fatal("canceled purchase accepted", e)
	}
	exec("UPDATE orders SET status='pending',version=2 WHERE id=?", order)
	if _, e := submit(purchaser, purchaseInput); !errors.Is(e, ErrIneligible) {
		t.Fatal("old version item granted purchase eligibility", e)
	}
	exec("UPDATE orders SET version=1 WHERE id=?", order)
	verified, e := submit(purchaser, purchaseInput)
	if e != nil {
		t.Fatal("paid unshipped order rejected", e)
	}
	if r := find(verified.ID); !r.VerifiedPurchase || r.OrderID == nil || *r.OrderID != order {
		t.Fatal("verified snapshot missing")
	}
	e = db.Transaction(func(tx *gorm.DB) error {
		p, e := PurchasesTx(tx, purchaser, store, ListQuery{ProductID: product, Limit: 20})
		if e != nil {
			return e
		}
		if p.Count != 1 || len(p.Orders) != 1 || !p.Orders[0].Eligible || p.Review == nil || p.Review.ID != verified.ID {
			return errors.New("private purchase selector mismatch")
		}
		o, e := OrderContextTx(tx, purchaser, store, order)
		if e != nil {
			return e
		}
		if !o.Eligible || len(o.Reviews) != 1 || o.Reviews[0].ProductID != product {
			return errors.New("private order context mismatch")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	// Current staff authority is still checked when replaying an existing receipt.
	exec("UPDATE admin_users SET enabled=FALSE WHERE id=?", actor.ID)
	if _, e := s.Moderate(ctx, actor, reviewID, originalModeration); e == nil {
		t.Fatal("disabled staff allowed review command")
	}
	if _, e := s.AdminPage(ctx, actor, ListQuery{ProductID: product, Limit: 5}); !errors.Is(e, authorization.ErrDenied) {
		t.Fatal("disabled staff allowed private review read", e)
	}
	exec("UPDATE admin_users SET enabled=TRUE WHERE id=?", actor.ID)
	exec("UPDATE product_review_runtime SET writes_enabled=FALSE WHERE id=1")
	if r, e := submit(account, input); e != nil || !r.Replayed {
		t.Fatal("closing writes prevents original receipt recovery", e)
	}
	if _, e := submit(newAccount(), in()); !errors.Is(e, ErrUnavailable) {
		t.Fatal("new write accepted while disabled", e)
	}
	exec("UPDATE catalog_products SET status='draft' WHERE id=?", product)
	if _, e := s.PublicPage(ctx, ListQuery{ProductID: product, Limit: 5}); !errors.Is(e, ErrProduct) {
		t.Fatal("draft product reviews publicly exposed", e)
	}
	exec("UPDATE catalog_products SET status='published' WHERE id=?", product)
	for table, n := range physical {
		if after, e := nativeFactsSHA(db, table); e != nil || after != n {
			t.Fatal("review modified unrelated physical facts", table)
		}
	}
	t.Log(`PRODUCT_REVIEWS_NATIVE {"migrations":94,"concurrent_submit":4,"concurrent_moderation":4,"pending_private":true,"approval_aggregate":true,"late_receipt_rollback":true,"caller_rollback":true,"explicit_new_ownership":true,"legacy_actor_separate":true,"current_order_version":true,"paid_unshipped_eligible":true,"stock_untouched":true,"customer_http_browser":false}`)
}

// Compare every stored column, including exact raw quantities/reservations and
// money, rather than relying on row counts to detect unintended mutation.
func nativeFactsSHA(db *gorm.DB, table string) (string, error) {
	rows, e := db.Table(table).Order("id").Rows()
	if e != nil {
		return "", e
	}
	defer rows.Close()
	columns, e := rows.Columns()
	if e != nil {
		return "", e
	}
	facts := [][]*string{}
	for rows.Next() {
		values := make([]sql.RawBytes, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if e := rows.Scan(dest...); e != nil {
			return "", e
		}
		row := make([]*string, len(columns))
		for i, v := range values {
			if v != nil {
				s := string(v)
				row[i] = &s
			}
		}
		facts = append(facts, row)
	}
	if e := rows.Err(); e != nil {
		return "", e
	}
	return digest(struct {
		Columns []string
		Rows    [][]*string
	}{columns, facts}), nil
}
