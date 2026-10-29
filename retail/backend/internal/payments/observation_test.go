package payments

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type artificialObserver func(context.Context, SessionRequest, string) (SessionObservation, error)

func (f artificialObserver) ObserveSession(c context.Context, r SessionRequest, id string) (SessionObservation, error) {
	return f(c, r, id)
}

func TestObservationBoundary(t *testing.T) {
	good := CreatedSession{Status: "pending", Data: json.RawMessage(`{"large":9007199254740993.000000000000000000001}`)}
	for _, mode := range []string{"success", "error", "panic", "empty", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			observer := artificialObserver(func(c context.Context, r SessionRequest, id string) (SessionObservation, error) {
				if id != "private-candidate" {
					t.Fatal("candidate was changed")
				}
				switch mode {
				case "error":
					return SessionObservation{Session: good}, errors.New("private-provider-error")
				case "panic":
					panic("private-provider-panic")
				case "empty":
					return SessionObservation{}, nil
				case "invalid":
					return SessionObservation{Session: CreatedSession{Status: "pending", Data: json.RawMessage(`{"a":1,"a":2}`)}}, nil
				}
				return SessionObservation{Session: good}, nil
			})
			result, err := InvokeObservation(context.Background(), observer, SessionRequest{}, "private-candidate")
			if mode == "success" {
				if err != nil || string(result.Data) != string(good.Data) {
					t.Fatal("verified observation lost exact data")
				}
			} else if err != ErrCreateExecution || len(result.Data) != 0 || result.Status != "" {
				t.Fatal("unverified observation escaped boundary")
			}
		})
	}
	if _, err := InvokeObservation(context.Background(), nil, SessionRequest{}, "private-candidate"); err != ErrCreateExecution {
		t.Fatal("nil observer not protected")
	}
}
