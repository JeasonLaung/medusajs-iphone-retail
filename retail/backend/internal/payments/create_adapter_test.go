package payments

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type artificialCreate func(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error)

func (f artificialCreate) CreateSession(ctx context.Context, in SessionRequest) (CreateOutcome, CreatedSession, error) {
	return f(ctx, in)
}

func TestCreateAdapterBoundary(t *testing.T) {
	good := CreatedSession{Status: "pending", Data: json.RawMessage(`{"id":"private-token","large":9007199254740993.000000000000000000001}`)}
	for _, test := range []struct {
		outcome CreateOutcome
		result  CreatedSession
		err     error
		want    CreateOutcome
	}{
		{CreateSucceeded, good, nil, CreateSucceeded}, {CreateFailed, CreatedSession{}, nil, CreateFailed},
		{CreateFailed, good, nil, CreateUncertain}, {CreateSucceeded, CreatedSession{}, nil, CreateUncertain},
		{CreateSucceeded, CreatedSession{Status: "unknown", Data: json.RawMessage(`{}`)}, nil, CreateUncertain},
		{CreateSucceeded, CreatedSession{Status: "pending", Data: json.RawMessage(`{"a":1,"a":2}`)}, nil, CreateUncertain},
		{CreateSucceeded, good, errors.New("private-provider-token"), CreateUncertain},
		{CreateFailed, CreatedSession{}, context.DeadlineExceeded, CreateUncertain},
		{CreateUncertain, good, nil, CreateUncertain}, {"unrecognized", good, nil, CreateUncertain},
	} {
		adapter := artificialCreate(func(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error) {
			return test.outcome, test.result, test.err
		})
		outcome, result, err := InvokeCreate(t.Context(), adapter, SessionRequest{})
		if outcome != test.want || (err != nil) != (test.want == CreateUncertain) || err != nil && err != ErrCreateExecution {
			t.Fatal("creation outcome lost uncertainty or exposed raw errors")
		}
		if outcome == CreateSucceeded && !strings.Contains(string(result.Data), "9007199254740993.000000000000000000001") {
			t.Fatal("creation result lost numeric precision")
		}
		if outcome == CreateUncertain && (result.Status != "" || len(result.Data) != 0) {
			t.Fatal("unverified creation result escaped")
		}
	}
	panicAdapter := artificialCreate(func(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error) {
		panic("private-provider-token")
	})
	if outcome, result, err := InvokeCreate(t.Context(), panicAdapter, SessionRequest{}); outcome != CreateUncertain || result.Status != "" || err != ErrCreateExecution {
		t.Fatal("creation panic not isolated")
	}
	if _, err := NewCreateAdapters(nil); err != ErrEffectInput {
		t.Fatal("empty creation registry accepted")
	}
	entries := map[string]CreateSessionAdapter{"provider": artificialCreate(func(context.Context, SessionRequest) (CreateOutcome, CreatedSession, error) {
		return CreateSucceeded, good, nil
	})}
	registry, err := NewCreateAdapters(entries)
	if err != nil {
		t.Fatal("artificial creation registry failed")
	}
	entries["provider"] = nil
	if adapter, err := registry.Lookup("provider"); err != nil || adapter == nil {
		t.Fatal("caller mutated creation registry")
	}
	if _, err := registry.Lookup("unknown"); err != ErrEffectInput {
		t.Fatal("unknown provider creation allowed")
	}
}
