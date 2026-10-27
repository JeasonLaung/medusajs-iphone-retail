package orders

import (
	"database/sql"
	"errors"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/totals"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strings"
	"time"
	"unicode/utf8"
)

// PaymentEvidence contains explicitly projected historical facts. Provider
// payloads, metadata, addresses, saved methods and decryption keys never enter it.
type PaymentEvidence struct {
	OrderID           string              `json:"order_id"`
	Version           int                 `json:"version"`
	OperationsEnabled bool                `json:"operations_enabled"`
	Collections       []PaymentCollection `json:"collections"`
}
type PaymentCollection struct {
	ID               string           `json:"id"`
	CurrencyCode     string           `json:"currency_code"`
	Status           string           `json:"status"`
	Amount           string           `json:"amount"`
	AuthorizedAmount *string          `json:"authorized_amount"`
	CapturedAmount   *string          `json:"captured_amount"`
	RefundedAmount   *string          `json:"refunded_amount"`
	Payments         []PaymentRecord  `json:"payments"`
	Sessions         []PaymentSession `json:"sessions"`
}
type PaymentSession struct {
	ID            string `json:"id"`
	ProviderID    string `json:"provider_id"`
	CurrencyCode  string `json:"currency_code"`
	Amount        string `json:"amount"`
	Status        string `json:"status"`
	OperationHold string `json:"operation_hold"`
}
type PaymentRecord struct {
	ID            string            `json:"id"`
	ProviderID    string            `json:"provider_id"`
	SessionID     string            `json:"session_id"`
	CurrencyCode  string            `json:"currency_code"`
	Amount        string            `json:"amount"`
	CapturedAt    *time.Time        `json:"captured_at"`
	CanceledAt    *time.Time        `json:"canceled_at"`
	OperationHold string            `json:"operation_hold"`
	Captures      []PaymentMovement `json:"captures"`
	Refunds       []PaymentMovement `json:"refunds"`
}
type PaymentMovement struct {
	ID        string    `json:"id"`
	Amount    string    `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

func paymentAmount(decimal, raw *string) (*string, error) {
	if decimal == nil && raw == nil {
		return nil, nil
	}
	if decimal == nil {
		return nil, ErrSchema
	}
	n, err := amountNumber(*decimal, raw)
	if err != nil {
		return nil, ErrSchema
	}
	value, err := totals.Resolve(n)
	if err != nil {
		return nil, ErrSchema
	}
	return &value, nil
}
func requiredPaymentAmount(decimal, raw string) (string, error) {
	value, err := paymentAmount(&decimal, &raw)
	if err != nil || value == nil {
		return "", ErrSchema
	}
	return *value, nil
}

// Payments preserves collection/payment/session states as independent evidence.
// A completed collection or captured timestamp does not authorize any command.
func (h Handler) Payments(c *gin.Context) {
	id := c.Param("id")
	if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 191 || strings.ContainsRune(id, 0) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "orders.invalid_query", "Invalid order request")
		return
	}
	if h.DB == nil {
		platform.Fail(c, 503, "orders.payments_unavailable", "Order payments unavailable")
		return
	}
	var result PaymentEvidence
	err := h.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if Ready(c.Request.Context(), tx) != nil || payments.Ready(c.Request.Context(), tx) != nil {
			return ErrSchema
		}
		var order struct {
			ID      string
			Version int
		}
		if err := tx.Table("orders").Select("id,version").Where("id=? AND deleted_at IS NULL", id).Take(&order).Error; err != nil {
			return err
		}
		result = PaymentEvidence{OrderID: id, Version: order.Version, Collections: []PaymentCollection{}}
		var links []struct{ PaymentCollectionID string }
		if tx.Table("order_payment_collections").Select("payment_collection_id").Where("order_id=? AND deleted_at IS NULL", id).Order("payment_collection_id ASC").Limit(101).Scan(&links).Error != nil || len(links) > 100 {
			return ErrSchema
		}
		if len(links) == 0 {
			return nil
		}
		collectionIDs := make([]string, 0, len(links))
		for _, link := range links {
			collectionIDs = append(collectionIDs, link.PaymentCollectionID)
		}
		var collections []struct {
			ID, CurrencyCode, Status, Amount, RawAmount                                                                 string
			AuthorizedAmount, RawAuthorizedAmount, CapturedAmount, RawCapturedAmount, RefundedAmount, RawRefundedAmount *string
		}
		if tx.Table("payment_collections").Select("id,currency_code,status,CAST(amount AS CHAR) AS amount,raw_amount,CAST(authorized_amount AS CHAR) AS authorized_amount,raw_authorized_amount,CAST(captured_amount AS CHAR) AS captured_amount,raw_captured_amount,CAST(refunded_amount AS CHAR) AS refunded_amount,raw_refunded_amount").Where("id IN ? AND deleted_at IS NULL", collectionIDs).Order("id ASC").Scan(&collections).Error != nil || len(collections) != len(links) {
			return ErrSchema
		}
		positions := map[string]int{}
		for _, row := range collections {
			amount, err := requiredPaymentAmount(row.Amount, row.RawAmount)
			if err != nil {
				return err
			}
			authorized, err := paymentAmount(row.AuthorizedAmount, row.RawAuthorizedAmount)
			if err != nil {
				return err
			}
			captured, err := paymentAmount(row.CapturedAmount, row.RawCapturedAmount)
			if err != nil {
				return err
			}
			refunded, err := paymentAmount(row.RefundedAmount, row.RawRefundedAmount)
			if err != nil {
				return err
			}
			positions[row.ID] = len(result.Collections)
			result.Collections = append(result.Collections, PaymentCollection{ID: row.ID, CurrencyCode: row.CurrencyCode, Status: row.Status, Amount: amount, AuthorizedAmount: authorized, CapturedAmount: captured, RefundedAmount: refunded, Payments: []PaymentRecord{}, Sessions: []PaymentSession{}})
		}
		var sessionRows []struct{ ID, PaymentCollectionID, ProviderID, CurrencyCode, Amount, RawAmount, Status string }
		if tx.Table("payment_sessions").Select("id,payment_collection_id,provider_id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount,status").Where("payment_collection_id IN ? AND deleted_at IS NULL", collectionIDs).Order("id ASC").Limit(1001).Scan(&sessionRows).Error != nil || len(sessionRows) > 1000 {
			return ErrSchema
		}
		var paymentRows []struct {
			ID, PaymentCollectionID, ProviderID, PaymentSessionID, CurrencyCode, Amount, RawAmount string
			CapturedAt, CanceledAt                                                                 *time.Time
		}
		if tx.Table("payments").Select("id,payment_collection_id,provider_id,payment_session_id,currency_code,CAST(amount AS CHAR) AS amount,raw_amount,captured_at,canceled_at").Where("payment_collection_id IN ? AND deleted_at IS NULL", collectionIDs).Order("id ASC").Limit(1001).Scan(&paymentRows).Error != nil || len(paymentRows) > 1000 {
			return ErrSchema
		}
		sessionIDs, paymentIDs := []string{}, []string{}
		for _, row := range sessionRows {
			sessionIDs = append(sessionIDs, row.ID)
		}
		for _, row := range paymentRows {
			paymentIDs = append(paymentIDs, row.ID)
		}
		holds := map[string]string{}
		for source, ids := range map[string][]string{"payment": paymentIDs, "payment_session": sessionIDs} {
			if len(ids) == 0 {
				continue
			}
			var rows []struct {
				SourceID, Disposition, Reason string
				PolicyVersion                 int
			}
			if tx.Table("migration_payment_dispositions").Select("source_id,disposition,reason,policy_version").Where("source_table=? AND source_id IN ?", source, ids).Limit(1001).Scan(&rows).Error != nil || len(rows) > len(ids) {
				return ErrSchema
			}
			for _, row := range rows {
				if row.Disposition != "hold" || row.Reason != "legacy-payment-reconciliation" || row.PolicyVersion != 1 {
					return ErrSchema
				}
				holds[source+":"+row.SourceID] = "held"
			}
		}
		hold := func(source, id string) string {
			if v := holds[source+":"+id]; v != "" {
				return v
			}
			return "unverified"
		}
		for _, row := range sessionRows {
			amount, err := requiredPaymentAmount(row.Amount, row.RawAmount)
			if err != nil {
				return err
			}
			n, ok := positions[row.PaymentCollectionID]
			if !ok {
				return ErrSchema
			}
			result.Collections[n].Sessions = append(result.Collections[n].Sessions, PaymentSession{ID: row.ID, ProviderID: row.ProviderID, CurrencyCode: row.CurrencyCode, Amount: amount, Status: row.Status, OperationHold: hold("payment_session", row.ID)})
		}
		type location struct{ collection, payment int }
		paymentPositions := map[string]location{}
		for _, row := range paymentRows {
			amount, err := requiredPaymentAmount(row.Amount, row.RawAmount)
			if err != nil {
				return err
			}
			n, ok := positions[row.PaymentCollectionID]
			if !ok {
				return ErrSchema
			}
			paymentPositions[row.ID] = location{n, len(result.Collections[n].Payments)}
			result.Collections[n].Payments = append(result.Collections[n].Payments, PaymentRecord{ID: row.ID, ProviderID: row.ProviderID, SessionID: row.PaymentSessionID, CurrencyCode: row.CurrencyCode, Amount: amount, CapturedAt: row.CapturedAt, CanceledAt: row.CanceledAt, OperationHold: hold("payment", row.ID), Captures: []PaymentMovement{}, Refunds: []PaymentMovement{}})
		}
		if len(paymentIDs) > 0 {
			for _, table := range []string{"payment_captures", "payment_refunds"} {
				var rows []struct {
					ID, PaymentID, Amount, RawAmount string
					CreatedAt                        time.Time
				}
				if tx.Table(table).Select("id,payment_id,CAST(amount AS CHAR) AS amount,raw_amount,created_at").Where("payment_id IN ? AND deleted_at IS NULL", paymentIDs).Order("id ASC").Limit(1001).Scan(&rows).Error != nil || len(rows) > 1000 {
					return ErrSchema
				}
				for _, row := range rows {
					amount, err := requiredPaymentAmount(row.Amount, row.RawAmount)
					if err != nil {
						return err
					}
					pos, ok := paymentPositions[row.PaymentID]
					if !ok {
						return ErrSchema
					}
					p := &result.Collections[pos.collection].Payments[pos.payment]
					movement := PaymentMovement{ID: row.ID, Amount: amount, CreatedAt: row.CreatedAt}
					if table == "payment_captures" {
						p.Captures = append(p.Captures, movement)
					} else {
						p.Refunds = append(p.Refunds, movement)
					}
				}
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		platform.Fail(c, 404, "orders.not_found", "Order not found")
		return
	}
	if err != nil {
		platform.Fail(c, 503, "orders.payments_unavailable", "Order payments unavailable")
		return
	}
	c.JSON(200, result)
}
