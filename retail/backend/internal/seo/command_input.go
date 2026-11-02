package seo

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/google/uuid"
)

var ErrCommandInput = errors.New("invalid SEO command")
var ErrConflict = errors.New("SEO state or operation changed")
var ErrProtected = errors.New("SEO record requires reconciliation")

type SettingInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Group string `json:"group"`
}

// Admin JSON text stays text across the browser boundary, so editing a page
// never rounds JSON-LD numbers via JavaScript Number parsing.
type PageInput struct {
	Path               string `json:"path"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	Canonical          string `json:"canonical"`
	OGImage            string `json:"og_image"`
	Robots             string `json:"robots"`
	HreflangJSON       string `json:"hreflang_json"`
	StructuredDataJSON string `json:"structured_data_json"`
}
type Command struct {
	OperationID      string        `json:"operation_id"`
	ExpectedRevision string        `json:"expected_revision"`
	Resource         string        `json:"resource"`
	Action           string        `json:"action"`
	ID               string        `json:"id"`
	Setting          *SettingInput `json:"setting,omitempty"`
	Page             *PageInput    `json:"page,omitempty"`
}

var revisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var languagePattern = regexp.MustCompile(`^[a-z]{2}(?:-[A-Z]{2})?$|^x-default$`)
var indexKeyPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,128}$`)
var measurementPattern = regexp.MustCompile(`^G-[A-Z0-9]+$`)

const jsSpace = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

func inputText(value string, max int) (string, error) {
	value = strings.Trim(value, jsSpace)
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(utf16.Encode([]rune(value))) > max {
		return "", ErrCommandInput
	}
	return value, nil
}
func httpURL(value string) (*url.URL, error) {
	u, e := url.Parse(value)
	if e != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, ErrCommandInput
	}
	return u, nil
}
func publicKey(key string) bool {
	for _, k := range publicKeys {
		if key == k {
			return true
		}
	}
	return false
}

// Reject duplicate object keys at every nesting level, including JSON-LD.
// UseNumber preserves integers beyond JavaScript's safe Number range.
func unambiguousJSON(raw []byte) error {
	if len(raw) > 256*1024 || !utf8.Valid(raw) {
		return ErrCommandInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return ErrCommandInput
		}
		t, e := d.Token()
		if e != nil {
			return ErrCommandInput
		}
		if delimiter, ok := t.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, e := d.Token()
					k, ok := key.(string)
					if e != nil || !ok || seen[k] {
						return ErrCommandInput
					}
					seen[k] = true
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim('}') {
					return ErrCommandInput
				}
			case '[':
				for d.More() {
					if e := walk(depth + 1); e != nil {
						return e
					}
				}
				t, e = d.Token()
				if e != nil || t != json.Delim(']') {
					return ErrCommandInput
				}
			default:
				return ErrCommandInput
			}
		}
		return nil
	}
	if e := walk(0); e != nil || d.Decode(new(any)) != io.EOF {
		return ErrCommandInput
	}
	return nil
}
func decodeCommand(raw []byte) (Command, error) {
	var in Command
	if unambiguousJSON(raw) != nil {
		return in, ErrCommandInput
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		return in, ErrCommandInput
	}
	return normalizeCommand(in)
}

func validBingXML(value string) bool {
	d := xml.NewDecoder(strings.NewReader(value))
	depth, roots := 0, 0
	user, nested, found := false, false, false
	var content strings.Builder
	for {
		token, e := d.Token()
		if e == io.EOF {
			return roots == 1 && depth == 0 && found
		}
		if e != nil {
			return false
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				roots++
				if t.Name.Local != "users" {
					return false
				}
			}
			if depth == 2 && t.Name.Local == "user" {
				user, nested = true, false
				content.Reset()
			} else if depth > 2 && user {
				nested = true
			}
		case xml.CharData:
			if user && depth == 2 {
				content.Write(t)
			} else if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return false
			}
		case xml.EndElement:
			if depth == 2 && user {
				found = found || (!nested && strings.TrimSpace(content.String()) != "")
				user = false
			}
			depth--
		}
	}
}

func normalizeCommand(in Command) (Command, error) {
	id, e := uuid.Parse(in.OperationID)
	if e != nil || id.String() != in.OperationID || !revisionPattern.MatchString(in.ExpectedRevision) || (in.Resource != "settings" && in.Resource != "pages") || (in.Action != "save" && in.Action != "delete") || !utf8.ValidString(in.ID) || len(in.ID) > 191 || strings.ContainsAny(in.ID, "\x00\r\n") {
		return in, ErrCommandInput
	}
	if in.Action == "delete" {
		if in.ID == "" || in.Setting != nil || in.Page != nil {
			return in, ErrCommandInput
		}
		return in, nil
	}
	if in.Resource == "settings" {
		if in.Setting == nil || in.Page != nil || !publicKey(in.Setting.Key) {
			return in, ErrCommandInput
		}
		s := *in.Setting
		if s.Value, e = inputText(s.Value, 65536); e != nil {
			return in, e
		}
		if s.Group, e = inputText(s.Group, 32); e != nil {
			return in, e
		}
		if s.Group == "" {
			s.Group = "site"
		}
		if strings.HasSuffix(s.Key, "enabled") && s.Value != "true" && s.Value != "false" {
			return in, ErrCommandInput
		}
		switch s.Key {
		case "seo.site_url":
			u, err := httpURL(s.Value)
			if err != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
				return in, ErrCommandInput
			}
			s.Value = u.Scheme + "://" + u.Host
		case "seo.home_path":
			if s.Value, e = NormalizePath(s.Value); e != nil {
				return in, ErrCommandInput
			}
		case "seo.indexnow_key":
			if s.Value != "" && !indexKeyPattern.MatchString(s.Value) {
				return in, ErrCommandInput
			}
		case "ga4.measurement_id":
			if s.Value != "" && !measurementPattern.MatchString(s.Value) {
				return in, ErrCommandInput
			}
		case "bing.site_auth_xml":
			if s.Value != "" && !validBingXML(s.Value) {
				return in, ErrCommandInput
			}
		case "gsc.verification_code":
			if strings.ContainsAny(s.Value, "<>\"'") {
				return in, ErrCommandInput
			}
		}
		in.Setting = &s
		return in, nil
	}
	if in.Page == nil || in.Setting != nil {
		return in, ErrCommandInput
	}
	p := *in.Page
	for _, field := range []struct {
		value *string
		max   int
	}{{&p.Path, 2048}, {&p.Title, 512}, {&p.Description, 4000}, {&p.Canonical, 2048}, {&p.OGImage, 2048}, {&p.Robots, 128}} {
		if *field.value, e = inputText(*field.value, field.max); e != nil {
			return in, e
		}
	}
	if p.Path, e = NormalizePath(p.Path); e != nil {
		return in, ErrCommandInput
	}
	for _, value := range []string{p.Canonical, p.OGImage} {
		if value != "" {
			if _, e := httpURL(value); e != nil {
				return in, ErrCommandInput
			}
		}
	}
	if p.Robots == "" {
		p.Robots = "index,follow"
	}
	for _, robot := range strings.Split(p.Robots, ",") {
		switch strings.TrimSpace(robot) {
		case "index", "noindex", "follow", "nofollow", "noarchive", "nosnippet", "noimageindex":
		default:
			return in, ErrCommandInput
		}
	}
	if p.HreflangJSON == "" {
		p.HreflangJSON = `{}`
	}
	var languages map[string]string
	if unambiguousJSON([]byte(p.HreflangJSON)) != nil || json.Unmarshal([]byte(p.HreflangJSON), &languages) != nil || languages == nil {
		return in, ErrCommandInput
	}
	for language, value := range languages {
		if !languagePattern.MatchString(language) {
			return in, ErrCommandInput
		}
		if _, e := httpURL(value); e != nil {
			return in, ErrCommandInput
		}
	}
	if p.StructuredDataJSON == "" {
		p.StructuredDataJSON = `[]`
	}
	structured := bytes.TrimSpace([]byte(p.StructuredDataJSON))
	if unambiguousJSON(structured) != nil || len(structured) == 0 || (structured[0] != '{' && structured[0] != '[') {
		return in, ErrCommandInput
	}
	in.Page = &p
	return in, nil
}
