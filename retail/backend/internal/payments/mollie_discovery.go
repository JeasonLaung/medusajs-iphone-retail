package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// SessionDiscoverer is private server recovery, never a browser-supplied ID.
// Failure (including no match, ambiguity or an incomplete scan) proves nothing
// about whether a payment exists and must NEVER authorize another POST.
type SessionDiscoverer interface {
	DiscoverSession(context.Context, SessionRequest) (SessionObservation, error)
}

func InvokeDiscovery(ctx context.Context, adapter SessionDiscoverer, request SessionRequest) (out SessionObservation, err error) {
	defer func() {
		if recover() != nil {
			out, err = SessionObservation{}, ErrCreateExecution
		}
	}()
	if adapter == nil {
		return SessionObservation{}, ErrCreateExecution
	}
	out, err = adapter.DiscoverSession(ctx, request)
	if err != nil || ValidateMollieObservation(request, out.PaymentID, out) != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	return out, nil
}

// DiscoverSession scans at most 100 payments in the configured merchant profile.
// The list only supplies candidate identity. A unique candidate is independently
// GET-verified with the original immutable request. Large/incomplete histories
// remain uncertain; bounded discovery never claims a payment did not happen.
// https://docs.mollie.com/reference/list-payments
func (a *MollieCreateAdapter) DiscoverSession(ctx context.Context, request SessionRequest) (SessionObservation, error) {
	body, _, cartID, err := a.createBody(request)
	if err != nil {
		return SessionObservation{}, ErrCreateExecution
	}
	clear(body)
	const endpoint = "https://api.mollie.com/v2/payments"
	cursor, candidate := "", ""
	seen := map[string]bool{}
	for page := 0; page < 4; page++ {
		query := url.Values{"limit": {"25"}, "sort": {"desc"}}
		if cursor != "" {
			query.Set("from", cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return SessionObservation{}, ErrCreateExecution
		}
		req.Header.Set("Authorization", "Bearer "+a.transport.apiKey)
		req.Header.Set("Accept", "application/hal+json")
		response, err := a.transport.client.Do(req)
		if err != nil {
			return SessionObservation{}, ErrCreateExecution
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || len(raw) > 1<<20 {
			clear(raw)
			return SessionObservation{}, ErrCreateExecution
		}
		var list struct {
			Count    *int `json:"count"`
			Embedded struct {
				Payments []json.RawMessage `json:"payments"`
			} `json:"_embedded"`
			Links struct {
				Next *struct {
					Href string `json:"href"`
				} `json:"next"`
			} `json:"_links"`
		}
		validated, validationErr := requestObject(raw, false)
		err = json.Unmarshal(validated, &list)
		var envelope map[string]json.RawMessage
		var links map[string]json.RawMessage
		_ = json.Unmarshal(validated, &envelope)
		_ = json.Unmarshal(envelope["_links"], &links)
		clear(raw)
		clear(validated)
		if validationErr != nil || err != nil || links["next"] == nil || list.Count == nil || *list.Count != len(list.Embedded.Payments) || *list.Count > 25 || list.Embedded.Payments == nil {
			return SessionObservation{}, ErrCreateExecution
		}
		for _, item := range list.Embedded.Payments {
			var payment struct {
				ID       string          `json:"id"`
				Metadata json.RawMessage `json:"metadata"`
			}
			if json.Unmarshal(item, &payment) != nil || !molliePaymentID.MatchString(payment.ID) || seen[payment.ID] {
				return SessionObservation{}, ErrCreateExecution
			}
			seen[payment.ID] = true
			var metadata struct {
				SessionID string `json:"session_id"`
				CartID    string `json:"cart_id"`
			}
			if json.Unmarshal(payment.Metadata, &metadata) == nil && metadata.SessionID == request.SessionID && metadata.CartID == cartID {
				if candidate != "" {
					return SessionObservation{}, ErrCreateExecution
				}
				candidate = payment.ID
			}
		}
		if list.Links.Next == nil {
			if candidate == "" {
				return SessionObservation{}, ErrCreateExecution
			}
			return a.ObserveSession(ctx, request, candidate)
		}
		// Never follow a returned URL with merchant credentials. Accept only the
		// documented fixed-origin cursor and reconstruct our own bounded request.
		next, err := url.Parse(list.Links.Next.Href)
		if err != nil || next.Scheme != "https" || next.Host != "api.mollie.com" || next.Path != "/v2/payments" || next.User != nil || next.Fragment != "" || next.Opaque != "" {
			return SessionObservation{}, ErrCreateExecution
		}
		values, err := url.ParseQuery(next.RawQuery)
		if err != nil || len(values["from"]) != 1 || !molliePaymentID.MatchString(values.Get("from")) || values.Get("from") == cursor || seen[values.Get("from")] || *list.Count == 0 {
			return SessionObservation{}, ErrCreateExecution
		}
		for key, value := range values {
			if len(value) != 1 || key != "from" && key != "limit" && key != "sort" || key == "limit" && value[0] != "25" || key == "sort" && value[0] != "desc" {
				return SessionObservation{}, ErrCreateExecution
			}
		}
		cursor = values.Get("from")
	}
	return SessionObservation{}, ErrCreateExecution
}
