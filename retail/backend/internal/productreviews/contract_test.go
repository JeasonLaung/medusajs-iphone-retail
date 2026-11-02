package productreviews

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const operation = "e41c68ca-38eb-4c51-aa4d-99160e57ab02"

func submissionJSON() string {
	return `{"operation_id":"` + operation + `","product_id":"prod-fixture","rating":5,"title":"手机很好","content":"这是测试客户提交的真实评价内容。"}`
}
func TestSubmissionKeepsSourceLimitsAndRejectsAuthorityFields(t *testing.T) {
	valid := submissionJSON()
	in, e := DecodeSubmission(strings.NewReader(valid))
	if e != nil || in.OrderID != nil || in.Rating != 5 {
		t.Fatal("ordinary review without purchase rejected", e)
	}
	for name, raw := range map[string]string{
		"forged author":         strings.Replace(valid, `"rating":5`, `"author":"someone else","rating":5`, 1),
		"forged verified":       strings.Replace(valid, `"rating":5`, `"verified_purchase":true,"rating":5`, 1),
		"forged customer":       strings.Replace(valid, `"rating":5`, `"customer_id":"legacy","rating":5`, 1),
		"forged account":        strings.Replace(valid, `"rating":5`, `"account_id":"other","rating":5`, 1),
		"self approve":          strings.Replace(valid, `"rating":5`, `"status":"approved","rating":5`, 1),
		"duplicate field":       strings.Replace(valid, `"rating":5`, `"rating":1,"rating":5`, 1),
		"escaped duplicate":     strings.Replace(valid, `"rating":5`, `"rat\u0069ng":1,"rating":5`, 1),
		"fraction":              strings.Replace(valid, `"rating":5`, `"rating":4.5`, 1),
		"zero":                  strings.Replace(valid, `"rating":5`, `"rating":0`, 1),
		"six":                   strings.Replace(valid, `"rating":5`, `"rating":6`, 1),
		"string rating":         strings.Replace(valid, `"rating":5`, `"rating":"5"`, 1),
		"null rating":           strings.Replace(valid, `"rating":5`, `"rating":null`, 1),
		"null optional order":   strings.Replace(valid, `"rating":5`, `"order_id":null,"rating":5`, 1),
		"empty optional order":  strings.Replace(valid, `"rating":5`, `"order_id":"  ","rating":5`, 1),
		"foreign request shape": `[]`,
		"trailing":              valid + `{}`,
		"oversize":              strings.Repeat(" ", 32769),
		"bad utf8":              valid + string([]byte{0xff}),
		"uuid wrong version":    strings.Replace(valid, operation, "e41c68ca-38eb-1c51-aa4d-99160e57ab02", 1),
		"uuid noncanonical":     strings.Replace(valid, operation, strings.ToUpper(operation), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := DecodeSubmission(strings.NewReader(raw)); e == nil {
				t.Fatal("ambiguous or privileged review accepted")
			}
		})
	}
	for _, tc := range []struct {
		title, content string
		ok             bool
	}{
		{"a", strings.Repeat("c", 10), false}, {"ab", strings.Repeat("c", 9), false},
		{strings.Repeat("a", 120), strings.Repeat("c", 3000), true},
		{strings.Repeat("a", 121), strings.Repeat("c", 3000), false},
		{strings.Repeat("a", 120), strings.Repeat("c", 3001), false},
		{strings.Repeat("😀", 60), strings.Repeat("😀", 1500), true},
		{strings.Repeat("😀", 61), strings.Repeat("😀", 1500), false},
		{"\ufeff  中文\u3000", "\ufeff  十个以上的中文评价内容\u3000", true},
	} {
		candidate := in
		candidate.Title = tc.title
		candidate.Content = tc.content
		_, e := normalize(candidate)
		if (e == nil) != tc.ok {
			t.Fatal("source UTF16 or trim limits changed", units(tc.title), units(tc.content), e)
		}
	}
}
func TestPurchaseEligibilityMatchesOriginalWithoutShippingRequirement(t *testing.T) {
	completed, pending, authorized := "completed", "awaiting", "authorized"
	for _, tc := range []struct {
		name, status string
		states       []*string
		want         bool
	}{
		{"paid unshipped", "pending", []*string{&completed}, true},
		{"completed", "completed", []*string{&completed}, true},
		{"all paid", "pending", []*string{&completed, &completed}, true},
		{"partial supplemental", "pending", []*string{&completed, &pending}, false},
		{"authorized only", "pending", []*string{&authorized}, false},
		{"empty", "pending", []*string{}, false},
		{"missing collection", "pending", []*string{nil}, false},
		{"canceled paid", "canceled", []*string{&completed}, false},
		{"draft paid", "draft", []*string{&completed}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if OrderEligible(tc.status, tc.states) != tc.want {
				t.Fatal("purchase eligibility changed")
			}
		})
	}
}
func TestPublicProjectionExcludesPrivateOwnershipAndModeration(t *testing.T) {
	account, order := "private-new-account", "private-order"
	row := reviewRecord{PublicReview: PublicReview{ID: "rev-fixture", Author: PublicAuthor("  小明  "), Rating: 4, Title: "原有标题", Content: "公开评价正文", CreatedAt: time.Unix(0, 0).UTC(), VerifiedPurchase: true}, ProductID: "prod-fixture", CustomerID: "legacy-secret", AccountID: &account, OrderID: &order, Status: "approved"}
	data, e := json.Marshal(row.PublicReview)
	if e != nil {
		t.Fatal(e)
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || len(values) != 7 {
		t.Fatal("public review projection changed")
	}
	for _, field := range []string{"customer_id", "account_id", "order_id", "product_id", "status", "email", "auth_version"} {
		if _, ok := values[field]; ok {
			t.Fatal("private field exposed", field)
		}
	}
	for _, secret := range []string{account, order, row.CustomerID} {
		if strings.Contains(string(data), secret) {
			t.Fatal("private identity exposed")
		}
	}
	if row.Author != "小明" {
		t.Fatal("author must be first-name snapshot only")
	}
	if a := PublicAuthor(strings.Repeat("😀", 29) + "a😀x"); units(a) != 59 || strings.HasSuffix(a, "�") {
		t.Fatal("author truncation splits UTF16 pair")
	}
}
func TestListAndModerationBoundary(t *testing.T) {
	q, e := ParseListQuery("product_id=prod-fixture", false)
	if e != nil || q.Offset != 0 || q.Limit != 5 {
		t.Fatal("source public defaults changed")
	}
	q, e = ParseListQuery("product_id=prod-fixture&offset=20", true)
	if e != nil || q.Offset != 20 || q.Limit != 20 {
		t.Fatal("source purchases pagination changed")
	}
	for _, raw := range []string{"", "product_id=p&product_id=q", "product_id=p&offset=01", "product_id=p&offset=-1", "product_id=p&offset=100001", "product_id=p&limit=21", "product_id=p&limit=0", "product_id=p&offset=1.5", "product_id=p&customer_id=other", "product_id=p&status=pending", "product_id=p&limit=5&limit=5", "product_id=%00", "product_id=p&broken=%ZZ"} {
		if _, e := ParseListQuery(raw, false); e == nil {
			t.Fatal("invalid query accepted", raw)
		}
	}
	if _, e := ParseListQuery("product_id=p&limit=20", true); e == nil {
		t.Fatal("client overrode fixed purchases page size")
	}
	for _, status := range []string{"pending", "approved", "rejected"} {
		body := `{"operation_id":"` + operation + `","expected_revision":"` + strings.Repeat("a", 64) + `","status":"` + status + `"}`
		if _, e := DecodeModeration(strings.NewReader(body)); e != nil {
			t.Fatal("source moderation state rejected")
		}
		for _, bad := range []string{strings.Replace(body, `"status":"`+status+`"`, `"status":"deleted"`, 1), strings.Replace(body, `"status":"`+status+`"`, `"status":null`, 1), strings.Replace(body, `"status":"`+status+`"`, `"status":"pending","status":"approved"`, 1), strings.Replace(body, strings.Repeat("a", 64), strings.Repeat("A", 64), 1)} {
			if _, e := DecodeModeration(strings.NewReader(bad)); e == nil {
				t.Fatal("invalid moderation accepted")
			}
		}
	}
}
