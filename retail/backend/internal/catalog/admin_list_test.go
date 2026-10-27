package catalog

import (
	"net/url"
	"testing"
)

func TestAdminListRejectsAmbiguousAndUnknownInput(t *testing.T) {
	for _, raw := range []string{"limit=0", "limit=101", "offset=-1", "offset=100001", "limit=1&limit=2", "status=deleted", "sort=title;DROP", "fields=*", "q=", "q=%20", "q=%00", "collection_id=%20bad", "limit=+1", "q=%FF"} {
		v, err := url.ParseQuery(raw)
		if err != nil {
			continue
		}
		if _, err = parseAdminList(v); err == nil {
			t.Fatalf("accepted invalid query %q", raw)
		}
	}
	q, err := parseAdminList(url.Values{"q": {"phone"}, "status": {"draft"}, "limit": {"100"}, "offset": {"20"}, "sort": {"title_asc"}, "collection_id": {"collection-1"}})
	if err != nil || q.Limit != 100 || q.Offset != 20 || q.Order != "p.title ASC, p.id ASC" || q.Search != "phone" || q.Status != "draft" || q.CollectionID != "collection-1" {
		t.Fatal("valid query changed")
	}
	if literalLike("50%_!") != "%50!%!_!!%" {
		t.Fatal("wildcards not literal")
	}
}
