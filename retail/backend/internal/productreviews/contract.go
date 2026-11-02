// Package productreviews preserves the existing product review workflow. It is
// separate from after-sales request review and never establishes order ownership.
package productreviews

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInput       = errors.New("invalid product review request")
	ErrUnavailable = errors.New("product reviews unavailable")
	ErrProduct     = errors.New("review product unavailable")
	ErrOrder       = errors.New("review order unavailable")
	ErrIneligible  = errors.New("review order ineligible")
	ErrDuplicate   = errors.New("review already submitted")
	ErrConflict    = errors.New("product review state changed")
)

type Submission struct {
	OperationID string  `json:"operation_id"`
	ProductID   string  `json:"product_id"`
	OrderID     *string `json:"order_id,omitempty"`
	Rating      int     `json:"rating"`
	Title       string  `json:"title"`
	Content     string  `json:"content"`
}
type Result struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Replayed bool   `json:"replayed"`
}
type PublicReview struct {
	ID               string    `json:"id"`
	Author           string    `json:"author"`
	Rating           int       `json:"rating"`
	Title            string    `json:"title"`
	Content          string    `json:"content"`
	CreatedAt        time.Time `json:"created_at"`
	VerifiedPurchase bool      `json:"verified_purchase"`
}
type Distribution struct {
	Rating int   `json:"rating"`
	Count  int64 `json:"count"`
}
type Page struct {
	Reviews      []PublicReview `json:"reviews"`
	Count        int64          `json:"count"`
	Offset       int            `json:"offset"`
	Limit        int            `json:"limit"`
	Average      float64        `json:"average"`
	Distribution []Distribution `json:"distribution"`
}
type ListQuery struct {
	ProductID     string
	Offset, Limit int
}

func statusOK(s string) bool { return s == "pending" || s == "approved" || s == "rejected" }
func idOK(s string) bool {
	return utf8.ValidString(s) && s != "" && utf8.RuneCountInString(s) <= 255 && !strings.ContainsAny(s, "\x00\r\n")
}
func operationOK(s string) bool {
	u, e := uuid.Parse(s)
	return e == nil && u.Version() == 4 && u.String() == s
}
func revisionOK(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Source Zod string limits and first-name slicing use JavaScript UTF-16 units.
// Never split an astral character while truncating the public author snapshot.
func units(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
func jsTrim(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return r >= '\t' && r <= '\r' || r == ' ' || r == 0xa0 || r == 0x1680 || r >= 0x2000 && r <= 0x200a || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000 || r == 0xfeff
	})
}
func PublicAuthor(firstName string) string {
	s := jsTrim(firstName)
	n := 0
	for i, r := range s {
		width := 1
		if r > 0xffff {
			width = 2
		}
		if n+width > 60 {
			return s[:i]
		}
		n += width
	}
	return s
}
func normalize(in Submission) (Submission, error) {
	in.ProductID = jsTrim(in.ProductID)
	in.Title = jsTrim(in.Title)
	in.Content = jsTrim(in.Content)
	if in.OrderID != nil {
		s := jsTrim(*in.OrderID)
		in.OrderID = &s
	}
	if !operationOK(in.OperationID) || !idOK(in.ProductID) || in.OrderID != nil && !idOK(*in.OrderID) || in.Rating < 1 || in.Rating > 5 || !utf8.ValidString(in.Title) || !utf8.ValidString(in.Content) || units(in.Title) < 2 || units(in.Title) > 120 || units(in.Content) < 10 || units(in.Content) > 3000 {
		return Submission{}, ErrInput
	}
	return in, nil
}

// Decode flat objects explicitly so duplicate keys, nulls and caller-selected
// author/customer/status/verified flags cannot change a privileged field.
func decodeObject(r io.Reader, allowed ...string) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, 32769))
	if e != nil || len(b) > 32768 || !utf8.Valid(b) {
		return nil, ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(b))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return nil, ErrInput
	}
	seen := map[string]bool{}
	permitted := map[string]bool{}
	for _, key := range allowed {
		permitted[key] = true
	}
	for d.More() {
		t, e := d.Token()
		if e != nil {
			return nil, ErrInput
		}
		key, ok := t.(string)
		if !ok || !permitted[key] || seen[key] {
			return nil, ErrInput
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrInput
		}
	}
	if _, e = d.Token(); e != nil {
		return nil, ErrInput
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, ErrInput
	}
	return b, nil
}
func DecodeSubmission(r io.Reader) (Submission, error) {
	b, e := decodeObject(r, "operation_id", "product_id", "order_id", "rating", "title", "content")
	if e != nil {
		return Submission{}, e
	}
	var in Submission
	if json.Unmarshal(b, &in) != nil {
		return in, ErrInput
	}
	return normalize(in)
}
func ParseListQuery(raw string, purchases bool) (ListQuery, error) {
	out := ListQuery{Limit: 5}
	if purchases {
		out.Limit = 20
	}
	q, e := url.ParseQuery(raw)
	if e != nil {
		return out, ErrInput
	}
	for key, values := range q {
		if len(values) != 1 || values[0] == "" {
			return out, ErrInput
		}
		s := values[0]
		switch key {
		case "product_id":
			out.ProductID = jsTrim(s)
		case "offset", "limit":
			if purchases && key == "limit" {
				return out, ErrInput
			}
			n, e := strconv.Atoi(s)
			if e != nil || strconv.Itoa(n) != s || n < 0 {
				return out, ErrInput
			}
			if key == "offset" {
				if n > 100000 {
					return out, ErrInput
				}
				out.Offset = n
			} else {
				if n < 1 || n > 20 {
					return out, ErrInput
				}
				out.Limit = n
			}
		default:
			return out, ErrInput
		}
	}
	if !idOK(out.ProductID) {
		return out, ErrInput
	}
	return out, nil
}

// Shipping and fulfillment are intentionally absent: the existing workflow
// requires a non-canceled/non-draft order with every collection completed.
func OrderEligible(status string, collections []*string) bool {
	if status == "canceled" || status == "draft" || len(collections) == 0 {
		return false
	}
	for _, s := range collections {
		if s == nil || *s != "completed" {
			return false
		}
	}
	return true
}
