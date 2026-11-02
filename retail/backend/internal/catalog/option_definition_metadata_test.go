package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefinitionMetadataSourceMergeAndExactJSON(t *testing.T) {
	before := `{"large":9007199254740993123,"nested":{"a":1,"b":2},"remove":"old","keep":false}`
	out, e := mergeDefinitionMetadata(&before, json.RawMessage(`{"nested":{"a":3},"remove":"","zero":0,"null":null}`))
	if e != nil || out == nil || *out != `{"keep":false,"large":9007199254740993123,"nested":{"a":3},"null":null,"zero":0}` {
		t.Fatalf("source shallow merge or precision changed: %v %v", out, e)
	}
	same, e := mergeDefinitionMetadata(out, json.RawMessage(`{}`))
	if e != nil || *same != *out {
		t.Fatal("empty object erased metadata", e)
	}
	clear, e := mergeDefinitionMetadata(out, json.RawMessage(`null`))
	if e != nil || clear != nil {
		t.Fatal("null did not clear", e)
	}
}
func TestDefinitionMetadataStrictInput(t *testing.T) {
	fixture := definitionInputFixture()
	prefix := `{"operation_id":"` + fixture.OperationID + `","expected_revision":"` + fixture.ExpectedRevision + `","metadata":`
	for _, value := range []string{`null`, `{}`, `{"large":9007199254740993123,"a":null,"nested":[1,true]}`} {
		if _, e := decodeDefinitionMetadata(strings.NewReader(prefix + value + `}`)); e != nil {
			t.Fatal(value, e)
		}
	}
	for _, value := range []string{`[]`, `1`, `"text"`, `{"a":1,"a":2}`} {
		if _, e := decodeDefinitionMetadata(strings.NewReader(prefix + value + `}`)); e == nil {
			t.Fatal("ambiguous metadata accepted", value)
		}
	}
	for _, raw := range []string{prefix + `{},"metadata":null}`, prefix + `{},"extra":1}`, prefix + `{}}{}`, strings.Repeat(" ", 262145)} {
		if _, e := decodeDefinitionMetadata(strings.NewReader(raw)); e == nil {
			t.Fatal("ambiguous envelope accepted")
		}
	}
}
