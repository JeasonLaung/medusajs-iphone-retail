package payments

import (
	"context"
	"errors"
	"testing"
)

type artificialDelete func(context.Context, SessionRequest) (DeleteOutcome, error)

func (f artificialDelete) DeleteSession(ctx context.Context, in SessionRequest) (DeleteOutcome, error) {
	return f(ctx, in)
}

func TestDeleteAdapterBoundary(t *testing.T) {
	for _, test := range []struct {
		outcome DeleteOutcome
		err     error
		want    DeleteOutcome
		wantErr bool
	}{
		{DeleteSucceeded, nil, DeleteSucceeded, false}, {DeleteFailed, nil, DeleteFailed, false},
		{DeleteUncertain, nil, DeleteUncertain, true}, {"unrecognized", nil, DeleteUncertain, true},
		{DeleteFailed, context.DeadlineExceeded, DeleteUncertain, true}, {DeleteSucceeded, errors.New("private-provider-token"), DeleteUncertain, true},
	} {
		adapter := artificialDelete(func(context.Context, SessionRequest) (DeleteOutcome, error) { return test.outcome, test.err })
		outcome, err := InvokeDelete(t.Context(), adapter, SessionRequest{})
		if outcome != test.want || (err != nil) != test.wantErr || err != nil && err != ErrDeleteExecution {
			t.Fatal("provider error classification leaked or lost uncertainty")
		}
	}
	panicAdapter := artificialDelete(func(context.Context, SessionRequest) (DeleteOutcome, error) { panic("private-provider-token") })
	if outcome, err := InvokeDelete(t.Context(), panicAdapter, SessionRequest{}); outcome != DeleteUncertain || err != ErrDeleteExecution {
		t.Fatal("adapter panic lost uncertainty")
	}
	if _, err := NewDeleteAdapters(nil); err != ErrEffectInput {
		t.Fatal("empty adapter configuration accepted")
	}
	if _, err := NewDeleteAdapters(map[string]DeleteSessionAdapter{"provider": nil}); err != ErrEffectInput {
		t.Fatal("nil adapter accepted")
	}
	entries := map[string]DeleteSessionAdapter{"provider": artificialDelete(func(context.Context, SessionRequest) (DeleteOutcome, error) { return DeleteSucceeded, nil })}
	registry, err := NewDeleteAdapters(entries)
	if err != nil {
		t.Fatal("artificial registry rejected")
	}
	entries["provider"] = nil
	adapter, err := registry.Lookup("provider")
	if err != nil || adapter == nil {
		t.Fatal("caller mutation changed registry")
	}
	if _, err := registry.Lookup("browser-selected-provider"); err != ErrEffectInput {
		t.Fatal("unknown provider accepted")
	}
}
