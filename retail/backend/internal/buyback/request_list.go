package buyback

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type ReceivedAmount struct {
	CurrencyCode string  `json:"currency_code"`
	Amount       float64 `json:"amount"`
}
type RequestSummary struct {
	Recycled   int              `json:"recycled"`
	Processing int              `json:"processing"`
	Received   []ReceivedAmount `json:"received"`
}
type RequestList struct {
	Summary  RequestSummary  `json:"summary"`
	Requests []RequestDetail `json:"requests"`
	Count    int64           `json:"count"`
	Offset   int             `json:"offset"`
	Limit    int             `json:"limit"`
}

// ListRequests shares the caller transaction with each detail and the global
// summary. Summary covers all owned requests, not only the visible page.
func ListRequests(ctx context.Context, tx *gorm.DB, storeID, accountID string, offset int) (RequestList, error) {
	out := RequestList{Requests: []RequestDetail{}, Offset: offset, Limit: 20, Summary: RequestSummary{Received: []ReceivedAmount{}}}
	if tx == nil || !validID(storeID, 191) || !validID(accountID, 80) || offset < 0 || offset > 100000 {
		return out, ErrInput
	}
	tx = tx.WithContext(ctx)
	query := func() *gorm.DB {
		return tx.Table("buyback_requests r").Joins("JOIN buyback_account_access b ON b.request_id=r.id").Joins("JOIN commerce_stores s ON s.id=b.store_id AND s.deleted_at IS NULL").Where("r.deleted_at IS NULL AND b.customer_id=? AND b.store_id=?", accountID, storeID)
	}
	if query().Count(&out.Count).Error != nil {
		return RequestList{}, ErrUnavailable
	}
	var ids []string
	if query().Select("r.id").Order("r.created_at DESC,r.id DESC").Offset(offset).Limit(20).Scan(&ids).Error != nil {
		return RequestList{}, ErrUnavailable
	}
	for _, id := range ids {
		request, e := ReadRequest(ctx, tx, storeID, accountID, id)
		if e != nil {
			return RequestList{}, ErrUnavailable
		}
		out.Requests = append(out.Requests, request)
	}
	// Stream source rows instead of imposing a page-sized cap on account totals.
	rows, e := query().Select("r.status,r.items,r.currency_code,CAST(r.final_amount AS CHAR) AS final_amount,r.raw_final_amount").Order("r.created_at DESC,r.id DESC").Rows()
	if e != nil {
		return RequestList{}, ErrUnavailable
	}
	defer rows.Close()
	received := map[string]float64{}
	for rows.Next() {
		var status, items, currency string
		var amount, raw *string
		if rows.Scan(&status, &items, &currency, &amount, &raw) != nil || len(items) > 262144 {
			return RequestList{}, ErrUnavailable
		}
		var phones []json.RawMessage
		if json.Unmarshal([]byte(items), &phones) != nil || len(phones) > 10 {
			return RequestList{}, ErrUnavailable
		}
		if status == "paid" {
			out.Summary.Recycled += len(phones)
			n, e := requestAmount(amount, raw)
			if e != nil {
				return RequestList{}, e
			}
			value := 0.0
			if n != nil {
				value, e = strconv.ParseFloat(string(*n), 64)
				if e != nil || math.IsInf(value, 0) || math.IsNaN(value) {
					return RequestList{}, ErrUnavailable
				}
			}
			currency = strings.ToUpper(currency)
			// Original roundPrice after each paid request, including Number.EPSILON.
			total := math.Floor((received[currency]+value+(math.Nextafter(1, 2)-1))*100+0.5) / 100
			if math.IsInf(total, 0) || math.IsNaN(total) {
				return RequestList{}, ErrUnavailable
			}
			received[currency] = total
		} else if status != "returned" && status != "canceled" {
			out.Summary.Processing += len(phones)
		}
	}
	if rows.Err() != nil {
		return RequestList{}, ErrUnavailable
	}
	currencies := make([]string, 0, len(received))
	for currency := range received {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		out.Summary.Received = append(out.Summary.Received, ReceivedAmount{currency, received[currency]})
	}
	return out, nil
}
