package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOptionDefinitionPaginationRejectsAmbiguousQueries(t *testing.T) {
	for query, want := range map[string]int{"": 0, "offset=0": 0, "offset=20": 20, "offset=100000": 100000} {
		got, err := parseDefinitionQuery(query)
		if err != nil || got.Offset != want {
			t.Fatalf("valid pagination rejected: %q %+v %v", query, got, err)
		}
	}
	for _, query := range []string{"offset=0&offset=20", "offset=", "offset=-1", "offset=100001", "offset=01", "offset=+1", "offset=1.0", "offset=999999999999999999999999", "offset=%FF", "offset=%", "offset=0;offset=20", "limit=20", "offset=0&is_exclusive=1"} {
		if _, err := parseDefinitionQuery(query); err == nil {
			t.Fatalf("ambiguous pagination accepted: %q", query)
		}
	}
}

func TestOptionDefinitionFilters(t *testing.T) {
	q, e := parseDefinitionQuery("q=%E5%AE%B9%E9%87%8F%25_!&title=Exact+Title&id=opt_1&sort=updated_at_desc&offset=20")
	if e != nil || q.Search != "容量%_!" || q.Title != "Exact Title" || q.ID != "opt_1" || q.Sort != "updated_at_desc" || q.Offset != 20 {
		t.Fatalf("filters rejected: %+v %v", q, e)
	}
	for _, raw := range []string{"q=", "q=+", "q=%FF", "q=a%00b", "q=a&q=b", "title=a&title=b", "id=../x", "sort=title;DROP", "sort=", "unknown=x", "q=" + strings.Repeat("x", 201), "title=" + strings.Repeat("中", 101)} {
		if _, e := parseDefinitionQuery(raw); e == nil {
			t.Fatalf("invalid filter accepted: %q", raw)
		}
	}
}

func TestOptionDefinitionRemovalRequiresOnlyCanonicalOperation(t *testing.T) {
	in := definitionInputFixture()
	raw, _ := json.Marshal(OptionDefinitionRemoveInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision})
	body := string(raw)
	if _, err := decodeDefinitionRemoval(strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(body, `"operation_id":`, `"operation_id":null,"operation_id":`, 1),
		strings.Replace(body, `"expected_revision":`, `"other":false,"expected_revision":`, 1),
		strings.Replace(body, in.ExpectedRevision, strings.Repeat("A", 64), 1),
		strings.Replace(body, in.OperationID, strings.ToUpper(in.OperationID), 1),
		strings.Replace(body, `"expected_revision":"`+in.ExpectedRevision+`"`, `"expected_revision":null`, 1),
		`{}`, `null`, `[]`, body + `{}`, strings.Repeat(" ", 1025),
	} {
		if _, err := decodeDefinitionRemoval(strings.NewReader(invalid)); err == nil {
			t.Fatalf("ambiguous removal accepted: %.180s", invalid)
		}
	}
}

func TestDefinitionMultiValueAndDateFilters(t *testing.T) {
	q, e := parseDefinitionQuery("id[]=a&id[]=b&title[]=One&title[]=Two&created_at[$gte]=2026-01-02T08:00:00.123456%2B08:00&updated_at[$lt]=2026-01-03T00:00:00Z")
	if e != nil || len(q.IDs) != 2 || len(q.Titles) != 2 || len(q.Dates) != 2 || q.Dates[0].Value.Format("2006-01-02T15:04:05.999999Z") != "2026-01-02T00:00:00.123456Z" {
		t.Fatalf("multi/date filters rejected: %+v %v", q, e)
	}
	for _, raw := range []string{
		"id=a&id[]=b", "title=a&title[]=b", "id[]=a&id[]=a", "title[]=", "id[]=../x", "id[]=%FF",
		"created_at[$gte]=2026-02-30T00:00:00Z", "created_at[$gte]=2026-01-01", "created_at[$gte]=2026-01-01T00:00:00", "created_at[$gte]=2026-01-01T00:00:00.1234567Z",
		"created_at[$gte]=2026-01-01T00:00:00Z&created_at[$gte]=2026-01-02T00:00:00Z", "created_at[$like]=2026-01-01T00:00:00Z", "deleted_at[$gte]=not-a-date",
		"updated_at[$gt]=0001-01-01T00:00:00Z", strings.Repeat("q=x&", 10000),
	} {
		if _, e := parseDefinitionQuery(raw); e == nil {
			t.Fatal("ambiguous date/multi filter accepted", raw)
		}
	}
}

func TestDefinitionScopeAndCompoundQueryBoundaries(t *testing.T) {
	for _, raw := range []string{
		"is_exclusive=true&with_deleted=true&deleted_at[$gt]=2026-01-01T00:00:00Z",
		"$or[0][id]=one&$or[1][id]=two",
		"$and[0][$or][0][title]=One&$and[0][$or][1][title]=Two&$and[1][created_at][$gte]=2026-01-01T00:00:00Z",
	} {
		if _, e := parseDefinitionQuery(raw); e != nil {
			t.Fatal("valid source filter rejected", raw, e)
		}
	}
	for _, raw := range []string{
		"is_exclusive=TRUE", "is_exclusive=true&is_exclusive=false", "with_deleted=1",
		"$or[1][id]=hole", "$or[00][id]=x", "$or[16][id]=x", "$or[0][unknown]=x",
		"$and[0][with_deleted]=true", "$and[0][offset]=20", "$or[0][id]=x&$or[0][id]=y",
		"$and[0][$and][0][$and][0][$and][0][$and][0][id]=x",
	} {
		if _, e := parseDefinitionQuery(raw); e == nil {
			t.Fatal("ambiguous compound filter accepted", raw)
		}
	}
}
