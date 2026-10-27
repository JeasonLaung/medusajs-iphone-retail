package catalog

import (
	"net/url"
	"testing"
)

func TestPricePageRejectsImplicitConditions(t *testing.T) {
	for _, raw := range []string{"fields=*", "currency_code=eur", "region_id=other", "limit=0", "offset=-1", "limit=1&limit=2", "limit=101", "offset=100001", "sort=amount"} {
		values, err := url.ParseQuery(raw)
		if err != nil {
			continue
		}
		if _, err = parsePricePage(values); err == nil {
			t.Fatal("unsupported or invalid price filter accepted")
		}
	}
	q, err := parsePricePage(url.Values{"limit": {"100"}, "offset": {"20"}})
	if err != nil || q.Limit != 100 || q.Offset != 20 {
		t.Fatal("price pagination changed")
	}
}
