package payments

import (
	"context"
	"encoding/json"
	"errors"
)

type CreateOutcome string

const (
	CreateSucceeded CreateOutcome = "succeeded"
	CreateFailed    CreateOutcome = "failed"
	CreateUncertain CreateOutcome = "uncertain"
)

var ErrCreateExecution = errors.New("payment session creation requires reconciliation")

// CreateSessionAdapter must verify provider identity/amount/currency, set HTTP
// timeouts, and reuse request.SessionID as the stable provider idempotency key.
// It must not use the changing journal attempt UUID or write local SQL. Failed
// means definitive proof that no remote session was created and has no result.
// Any error, panic, malformed/contradictory result is uncertain, never retryable.
type CreateSessionAdapter interface {
	CreateSession(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error)
}
type CreateAdapters struct {
	entries map[string]CreateSessionAdapter
}

func NewCreateAdapters(entries map[string]CreateSessionAdapter) (*CreateAdapters, error) {
	if len(entries) == 0 || len(entries) > 100 {
		return nil, ErrEffectInput
	}
	out := &CreateAdapters{entries: map[string]CreateSessionAdapter{}}
	for id, adapter := range entries {
		if !effectID.MatchString(id) || adapter == nil {
			return nil, ErrEffectInput
		}
		out.entries[id] = adapter
	}
	return out, nil
}
func (a *CreateAdapters) Lookup(providerID string) (CreateSessionAdapter, error) {
	if a == nil || a.entries[providerID] == nil {
		return nil, ErrEffectInput
	}
	return a.entries[providerID], nil
}

// InvokeCreate never returns/logs raw provider errors or unverified data. Paid
// statuses are retained as facts on success; local compensation must not activate
// them as ordinary unpaid replacements. No real provider is registered here.
func InvokeCreate(ctx context.Context, adapter CreateSessionAdapter, request SessionRequest) (outcome CreateOutcome, result CreatedSession, err error) {
	outcome = CreateUncertain
	defer func() {
		if recover() != nil {
			outcome = CreateUncertain
			result = CreatedSession{}
			err = ErrCreateExecution
		}
	}()
	if adapter == nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	status, response, callErr := adapter.CreateSession(ctx, request)
	if callErr != nil {
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
	switch status {
	case CreateSucceeded:
		// Use a full-length attempt marker for the same protected size bound as the
		// persisted response. Normalize only via the existing exact JSON encoder.
		raw, err := creationBytes("00000000-0000-4000-8000-000000000000", response)
		if err != nil {
			return CreateUncertain, CreatedSession{}, ErrCreateExecution
		}
		defer clear(raw)
		var normalized creationResult
		if json.Unmarshal(raw, &normalized) != nil {
			return CreateUncertain, CreatedSession{}, ErrCreateExecution
		}
		return CreateSucceeded, normalized.Result, nil
	case CreateFailed:
		if response.Status != "" || len(response.Data) != 0 {
			return CreateUncertain, CreatedSession{}, ErrCreateExecution
		}
		return CreateFailed, CreatedSession{}, nil
	default:
		return CreateUncertain, CreatedSession{}, ErrCreateExecution
	}
}
