package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/eurorefurb/retail/internal/protected"
	"gorm.io/gorm"
)

// CreatedSession is private verified adapter output, not customer input. Data
// contains the provider's returned session data; it must never be logged. The
// adapter must validate remote identity/amount/currency before reporting success.
type CreatedSession struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}
type creationResult struct {
	Version   int            `json:"version"`
	AttemptID string         `json:"attempt_id"`
	Result    CreatedSession `json:"result"`
}

func creationBytes(attempt string, result CreatedSession) ([]byte, error) {
	switch result.Status {
	case "pending", "requires_more", "error", "canceled", "pending_authorization", "authorized", "captured":
	default:
		return nil, ErrEffectInput
	}
	var err error
	result.Data, err = requestObject(result.Data, false)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(creationResult{1, attempt, result})
	if err != nil || len(raw) > protected.MaxPlaintext {
		return nil, ErrEffectInput
	}
	return raw, nil
}
func creationContext(key EffectKey) protected.Context {
	c := requestContext(key)
	c.Purpose = "payment-effect-result-v1"
	c.Column = "result_envelope"
	return c
}
func openCreation(key EffectKey, ring *protected.Ring, envelope []byte) (creationResult, error) {
	raw, err := ring.Open(creationContext(key), envelope)
	if err != nil {
		return creationResult{}, ErrEffectProtection
	}
	defer clear(raw)
	var saved creationResult
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&saved) != nil || d.Decode(new(any)) != io.EOF || saved.Version != 1 {
		return creationResult{}, ErrEffectProtection
	}
	encoded, err := creationBytes(saved.AttemptID, saved.Result)
	if err != nil {
		return creationResult{}, ErrEffectProtection
	}
	defer clear(encoded)
	if !bytes.Equal(raw, encoded) {
		return creationResult{}, ErrEffectProtection
	}
	return saved, nil
}

// RecordSessionCreation saves a protected provider result and succeeded receipt
// in ONE caller transaction. Retry authenticates the existing envelope and exact
// attempt/result; it never overwrites a different response or repairs corruption.
// Paid statuses may be recorded as external facts; compensation activation must
// reject them. Roll back on any error. This function performs no external call.
func RecordSessionCreation(ctx context.Context, tx *gorm.DB, key EffectKey, attempt string, ring *protected.Ring, result CreatedSession) (EffectReceipt, error) {
	if key.Kind != "create_session" {
		return EffectReceipt{}, ErrEffectInput
	}
	raw, err := creationBytes(attempt, result)
	if err != nil {
		return EffectReceipt{}, err
	}
	defer clear(raw)
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	defer cancel()
	row, err := lockEffect(q, key)
	if err != nil {
		return EffectReceipt{}, err
	}
	if row.AttemptID == nil || *row.AttemptID != attempt || (row.State != "inflight" && row.State != "uncertain" && row.State != "succeeded") {
		return EffectReceipt{}, ErrEffectConflict
	}
	if _, err := ReadSessionRequest(ctx, tx, key, ring); err != nil {
		return EffectReceipt{}, err
	}
	var stored struct{ ResultEnvelope []byte }
	if effectWhere(q, key).Select("result_envelope").Take(&stored).Error != nil {
		return EffectReceipt{}, ErrEffectStorage
	}
	if stored.ResultEnvelope != nil {
		saved, err := openCreation(key, ring, stored.ResultEnvelope)
		if err != nil {
			return EffectReceipt{}, err
		}
		encoded, err := creationBytes(saved.AttemptID, saved.Result)
		if err != nil {
			return EffectReceipt{}, ErrEffectProtection
		}
		defer clear(encoded)
		if !bytes.Equal(raw, encoded) {
			return EffectReceipt{}, ErrEffectConflict
		}
	} else {
		if row.State == "succeeded" {
			return EffectReceipt{}, ErrEffectConflict
		}
		envelope, err := ring.Seal(creationContext(key), raw)
		if err != nil {
			return EffectReceipt{}, ErrEffectProtection
		}
		defer clear(envelope)
		r := effectWhere(q, key).Where("result_envelope IS NULL").Update("result_envelope", envelope)
		if r.Error != nil || r.RowsAffected != 1 {
			return EffectReceipt{}, ErrEffectStorage
		}
	}
	if err := registerMollieReference(ctx, tx, key, ring, result); err != nil {
		return EffectReceipt{}, err
	}
	return RecordEffect(ctx, tx, key, attempt, "succeeded")
}

// ReadSessionCreation returns only an authenticated result whose attempt is the
// same succeeded journal attempt. A bare success or absent result is not usable.
func ReadSessionCreation(ctx context.Context, tx *gorm.DB, key EffectKey, ring *protected.Ring) (CreatedSession, error) {
	if key.Kind != "create_session" {
		return CreatedSession{}, ErrEffectInput
	}
	q, cancel, err := effectTx(ctx, tx, key)
	if err != nil {
		return CreatedSession{}, err
	}
	defer cancel()
	row, err := lockEffect(q, key)
	if err != nil {
		return CreatedSession{}, err
	}
	if row.State != "succeeded" || row.AttemptID == nil {
		return CreatedSession{}, ErrEffectConflict
	}
	var stored struct{ ResultEnvelope []byte }
	if effectWhere(q, key).Select("result_envelope").Take(&stored).Error != nil {
		return CreatedSession{}, ErrEffectStorage
	}
	saved, err := openCreation(key, ring, stored.ResultEnvelope)
	if err != nil {
		return CreatedSession{}, err
	}
	if saved.AttemptID != *row.AttemptID {
		return CreatedSession{}, ErrEffectConflict
	}
	return saved.Result, nil
}
