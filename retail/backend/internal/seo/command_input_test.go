package seo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func settingCommand(key, value string) Command {
	return Command{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("a", 64), Resource: "settings", Action: "save", Setting: &SettingInput{Key: key, Value: value}}
}
func TestPublicSettingValidation(t *testing.T) {
	for _, c := range []Command{settingCommand("seo.site_url", "https://shop.example/"), settingCommand("seo.home_path", " /dk-zh/// "), settingCommand("seo.site_name", "\ufeff 翻新店 \u00a0"), settingCommand("bing.site_auth_xml", `<?xml version="1.0"?><users><user>sample</user></users>`), settingCommand("ga4.measurement_id", "G-ABC123")} {
		if _, e := normalizeCommand(c); e != nil {
			t.Fatal("valid public setting", e)
		}
	}
	got, _ := normalizeCommand(settingCommand("seo.home_path", " /dk-zh/// "))
	if got.Setting.Value != "/dk-zh" || got.Setting.Group != "site" {
		t.Fatal("source text/path normalization")
	}
	for _, c := range []Command{settingCommand("google.client_secret", "secret"), settingCommand("seo.site_url", "https://u:p@shop.example"), settingCommand("seo.site_url", "https://shop.example/path"), settingCommand("analytics.enabled", "yes"), settingCommand("seo.home_path", "//external"), settingCommand("seo.indexnow_key", "short"), settingCommand("ga4.measurement_id", "invalid"), settingCommand("gsc.verification_code", "<unsafe>"), settingCommand("bing.site_auth_xml", "<users><user><x>nested</x></user></users>"), settingCommand("bing.site_auth_xml", "<users><user>ok</user></users><extra/>"), settingCommand("seo.site_name", strings.Repeat("😀", 32769))} {
		if _, e := normalizeCommand(c); e != ErrCommandInput {
			t.Fatalf("accepted invalid setting %s", c.Setting.Key)
		}
	}
}
func TestPageValidationKeepsExactJSONNumbers(t *testing.T) {
	c := Command{OperationID: uuid.NewString(), ExpectedRevision: strings.Repeat("b", 64), Resource: "pages", Action: "save", Page: &PageInput{Path: " /dk-en/about/// ", Title: " 中文 ", HreflangJSON: `{"en":"https://shop.example/dk-en/about"}`, StructuredDataJSON: `{"value":900719925474099312345,"name":"</script>"}`}}
	in, e := normalizeCommand(c)
	if e != nil || in.Page.Path != "/dk-en/about" || in.Page.Title != "中文" || in.Page.Robots != "index,follow" || !strings.Contains(string(in.Page.StructuredDataJSON), "900719925474099312345") {
		t.Fatal("page normalization or precision", e)
	}
	for _, alter := range []func(*PageInput){func(p *PageInput) { p.Path = "//external" }, func(p *PageInput) { p.Canonical = "javascript:bad" }, func(p *PageInput) { p.OGImage = "https://u:p@example.com/i" }, func(p *PageInput) { p.Robots = "unknown" }, func(p *PageInput) { p.HreflangJSON = `[]` }, func(p *PageInput) { p.HreflangJSON = `{"en":"javascript:bad"}` }, func(p *PageInput) { p.StructuredDataJSON = `null` }, func(p *PageInput) { p.StructuredDataJSON = `{"value":1,"value":2}` }, func(p *PageInput) { p.Title = strings.Repeat("😀", 257) }} {
		p := *c.Page
		alter(&p)
		copy := c
		copy.Page = &p
		if _, e := normalizeCommand(copy); e != ErrCommandInput {
			t.Fatal("accepted invalid page")
		}
	}
}
func TestAmbiguousCommandAndResourceDataRejected(t *testing.T) {
	raw, _ := json.Marshal(settingCommand("seo.site_name", "Store"))
	if _, e := decodeCommand(raw); e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{string(raw) + `{}`, strings.Replace(string(raw), `"value":"Store"`, `"value":"Store","value":"Other"`, 1), strings.Replace(string(raw), `"group":""`, `"group":"","private":true`, 1), strings.Replace(string(raw), `"action":"save"`, `"action":"save","action":"delete"`, 1), `null`} {
		if _, e := decodeCommand([]byte(value)); e != ErrCommandInput {
			t.Fatal("ambiguous or unknown fields accepted")
		}
	}
	if _, e := decodeCommand([]byte(strings.Repeat(" ", 256*1024+1))); e != ErrCommandInput {
		t.Fatal("oversized input accepted")
	}
}
func TestDirectoryQueryIsBoundedAndCannotSelectAnotherScope(t *testing.T) {
	for _, q := range []string{"store_id=other", "limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "offset=01"} {
		if _, _, e := directoryQuery(q, "pages"); e != ErrCommandInput {
			t.Fatal("invalid page query accepted", q)
		}
	}
	if _, _, e := directoryQuery("offset=0", "settings"); e != ErrCommandInput {
		t.Fatal("settings accepted extra parameters")
	}
}
