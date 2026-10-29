package payments

import (
	"context"
	"errors"
)

type DeleteOutcome string

const (
	DeleteSucceeded DeleteOutcome = "succeeded"
	// DeleteFailed is permitted ONLY when the adapter has definitive evidence
	// that no external deletion occurred. A timeout/EOF/unknown response is not it.
	DeleteFailed    DeleteOutcome = "failed"
	DeleteUncertain DeleteOutcome = "uncertain"
)

var ErrDeleteExecution = errors.New("payment session deletion requires reconciliation")

// DeleteSessionAdapter must bound HTTP timeouts, classify provider responses,
// verify remote identity/state and never delete a paid session. It may not mutate
// local SQL or complete the cart. Request is private; never log its fields.
// Any returned error (including timeouts) overrides Outcome to uncertain.
// No concrete remote provider is registered by this package.
type DeleteSessionAdapter interface {
	DeleteSession(context.Context, SessionRequest) (DeleteOutcome, error)
}

// DeleteAdapters is a server-created immutable allowlist, never browser-selected
// code or an arbitrary provider URL. Construct anew after configuration changes.
type DeleteAdapters struct {
	entries map[string]DeleteSessionAdapter
}

func NewDeleteAdapters(entries map[string]DeleteSessionAdapter) (*DeleteAdapters, error) {
	if len(entries) == 0 || len(entries) > 100 {
		return nil, ErrEffectInput
	}
	out := &DeleteAdapters{entries: map[string]DeleteSessionAdapter{}}
	for id, adapter := range entries {
		if !effectID.MatchString(id) || adapter == nil {
			return nil, ErrEffectInput
		}
		out.entries[id] = adapter
	}
	return out, nil
}

func (a *DeleteAdapters) Lookup(providerID string) (DeleteSessionAdapter, error) {
	if a == nil {
		return nil, ErrEffectInput
	}
	adapter := a.entries[providerID]
	if adapter == nil {
		return nil, ErrEffectInput
	}
	return adapter, nil
}

// InvokeDelete converts unexpected panics, adapter errors and invalid outcomes
// into an opaque uncertain result. Raw provider errors can contain secrets and
// are never wrapped, returned or logged here. A provider timeout must be bounded
// by the adapter too; starting a detached goroutine would leak in-flight calls.
func InvokeDelete(ctx context.Context, adapter DeleteSessionAdapter, request SessionRequest) (outcome DeleteOutcome, err error) {
	outcome = DeleteUncertain
	defer func() {
		if recover() != nil {
			outcome = DeleteUncertain
			err = ErrDeleteExecution
		}
	}()
	if adapter == nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	result, callErr := adapter.DeleteSession(ctx, request)
	if callErr != nil {
		return DeleteUncertain, ErrDeleteExecution
	}
	switch result {
	case DeleteSucceeded, DeleteFailed:
		return result, nil
	case DeleteUncertain:
		return result, ErrDeleteExecution
	default:
		return DeleteUncertain, ErrDeleteExecution
	}
}
