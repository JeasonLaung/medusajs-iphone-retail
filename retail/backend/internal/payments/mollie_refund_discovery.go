package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// DiscoverRefund locates a lost POST result by the original immutable UUID in
// the configured merchant's parent payment. It scans at most 4x25 refunds, then
// independently GET-verifies the unique candidate. No candidate, ambiguous or
// incomplete history is uncertain; none of these outcomes authorizes a POST.
func (a *MollieCreateAdapter) DiscoverRefund(ctx context.Context, in RefundRequest) (out RefundObservation, err error) {
	defer refundBoundary(&out, &err)
	if _, e := a.refundInput(in); e != nil {
		return out, ErrRefundExecution
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	parent, e := a.ObserveSession(ctx, in.Session, in.RemotePaymentID)
	if e != nil || parent.ProviderStatus != "paid" {
		return out, ErrRefundExecution
	}
	clear(parent.Session.Data)
	path := in.RemotePaymentID + "/refunds"
	cursor, candidate := "", ""
	seen := map[string]bool{}
	for page := 0; page < 4; page++ {
		query := url.Values{"limit": {"25"}}
		if cursor != "" {
			query.Set("from", cursor)
		}
		raw, e := a.refundJSON(ctx, http.MethodGet, path+"?"+query.Encode(), nil, "", http.StatusOK)
		if e != nil {
			return out, ErrRefundExecution
		}
		var list struct {
			Count    *int `json:"count"`
			Embedded struct {
				Refunds []json.RawMessage `json:"refunds"`
			} `json:"_embedded"`
			Links struct {
				Next *struct {
					Href string `json:"href"`
				} `json:"next"`
			} `json:"_links"`
		}
		var envelope map[string]json.RawMessage
		var links map[string]json.RawMessage
		decode := json.Unmarshal(raw, &list)
		_ = json.Unmarshal(raw, &envelope)
		_ = json.Unmarshal(envelope["_links"], &links)
		clear(raw)
		if decode != nil || list.Count == nil || *list.Count != len(list.Embedded.Refunds) || *list.Count > 25 || list.Embedded.Refunds == nil || links["next"] == nil {
			return out, ErrRefundExecution
		}
		for _, item := range list.Embedded.Refunds {
			var v struct {
				ID       string          `json:"id"`
				Metadata json.RawMessage `json:"metadata"`
			}
			if json.Unmarshal(item, &v) != nil || !mollieRefundID.MatchString(v.ID) || seen[v.ID] {
				return out, ErrRefundExecution
			}
			seen[v.ID] = true
			var metadata struct {
				OperationID string `json:"refund_operation_id"`
			}
			if json.Unmarshal(v.Metadata, &metadata) == nil && metadata.OperationID == in.OperationID {
				if candidate != "" {
					return out, ErrRefundExecution
				}
				candidate = v.ID
			}
		}
		if list.Links.Next == nil {
			if candidate == "" {
				return out, ErrRefundExecution
			}
			return a.ObserveRefund(ctx, in, candidate)
		}
		// A remote link supplies only a checked cursor. Credentials are never sent
		// to returned URLs, even another path on the same host.
		next, e := url.Parse(list.Links.Next.Href)
		if e != nil || next.Scheme != "https" || next.Host != "api.mollie.com" || next.Path != "/v2/payments/"+path || next.User != nil || next.Fragment != "" || next.Opaque != "" {
			return out, ErrRefundExecution
		}
		values, e := url.ParseQuery(next.RawQuery)
		if e != nil || len(values["from"]) != 1 || !mollieRefundID.MatchString(values.Get("from")) || values.Get("from") == cursor || seen[values.Get("from")] || *list.Count == 0 {
			return out, ErrRefundExecution
		}
		for key, value := range values {
			if len(value) != 1 || key != "from" && key != "limit" || key == "limit" && value[0] != "25" {
				return out, ErrRefundExecution
			}
		}
		cursor = values.Get("from")
	}
	return out, ErrRefundExecution
}
