package payments

import "context"

// SessionObserver performs only an authoritative provider read. Candidate ID is
// a locator, not proof; verify it against the entire saved merchant/session/cart/
// amount/currency request. Bound HTTP timeouts; never create/delete or write SQL.
type SessionObserver interface {
	ObserveSession(context.Context, SessionRequest, string) (SessionObservation, error)
}

func (a *CreateAdapters) Observer(providerID string) (SessionObserver, error) {
	adapter, err := a.Lookup(providerID)
	if err != nil {
		return nil, err
	}
	observer, ok := adapter.(SessionObserver)
	if !ok || observer == nil {
		return nil, ErrEffectInput
	}
	return observer, nil
}

type observationInvocation struct {
	observer    SessionObserver
	candidateID string
	observed    *SessionObservation
}

func (a observationInvocation) CreateSession(ctx context.Context, request SessionRequest) (CreateOutcome, CreatedSession, error) {
	observed, err := a.observer.ObserveSession(ctx, request, a.candidateID)
	if err == nil && a.observed != nil {
		*a.observed = observed
	}
	return CreateSucceeded, observed.Session, err
}

// InvokeObservation applies the SAME opaque error/panic/exact response boundary
// as creation without issuing a creation call. It returns only verified session
// recovery data; webhook action processing uses the full observation separately.
func InvokeObservation(ctx context.Context, observer SessionObserver, request SessionRequest, candidateID string) (CreatedSession, error) {
	observed, err := InvokeSessionObservation(ctx, observer, request, candidateID)
	return observed.Session, err
}

func InvokeSessionObservation(ctx context.Context, observer SessionObserver, request SessionRequest, candidateID string) (SessionObservation, error) {
	var observed SessionObservation
	_, result, err := InvokeCreate(ctx, observationInvocation{observer, candidateID, &observed}, request)
	if err != nil {
		return SessionObservation{}, err
	}
	observed.Session = result
	return observed, nil
}
