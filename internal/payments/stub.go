package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProviderNameStub is the stable Name() the stub provider registers under.
const ProviderNameStub = "stub"

// Sentinel errors guarding the stub provider. See NewStub's doc comment:
// these exist to make it structurally hard to run real money through the
// auto-settling demo/test provider by accident.
var (
	ErrStubNotOptedIn        = errors.New("payments: stub: refuses to run without an explicit opt-in (wire this to --demo / CACKLE_DEMO, never default it to true)")
	ErrStubRefusesRealSecret = errors.New("payments: stub: refuses to run because a real Paystack secret is configured in this environment")
)

// StubProvider auto-settles every order the instant Begin is called. It
// exists only for `cackle --demo` and tests.
//
// It is deliberately hard to enable by accident:
//   - NewStub requires an explicit optIn bool. Callers must wire this to
//     something the operator chose on purpose (a --demo flag, CACKLE_DEMO)
//     — never a zero-value default that happens to be true.
//   - NewStub (and, redundantly, Registry.Register) refuse outright if
//     EnvPaystackSecretKey is set in the environment: a real provider
//     secret being configured means this is not a demo/test environment,
//     so the auto-settling stub must not be reachable at all.
type StubProvider struct {
	mu sync.Mutex
	// settled records charges the stub has "settled", keyed by
	// reference, so Verify reflects exactly what Begin produced instead
	// of independently re-deriving it.
	settled map[string]Result
}

// NewStub constructs the stub provider. See the type doc comment for why
// this refuses unless optIn is true and no real Paystack secret is
// configured.
func NewStub(optIn bool) (*StubProvider, error) {
	if !optIn {
		return nil, ErrStubNotOptedIn
	}
	if realPaystackSecretConfigured() {
		return nil, ErrStubRefusesRealSecret
	}
	return &StubProvider{settled: make(map[string]Result)}, nil
}

// Name implements Provider.
func (s *StubProvider) Name() string { return ProviderNameStub }

// Begin immediately marks the order as paid in the stub's in-memory
// ledger and returns instructions rather than a redirect URL — there is no
// real payment page.
func (s *StubProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.ID) == "" {
		return Charge{}, errors.New("payments: stub: order id is required")
	}
	if o.TotalCents <= 0 {
		return Charge{}, errors.New("payments: stub: total_cents must be positive")
	}
	currency := o.Currency
	if currency == "" {
		currency = "ZAR"
	}

	result := Result{
		Provider:    ProviderNameStub,
		Reference:   o.ID,
		EventID:     "stub-" + o.ID,
		Status:      StatusPaid,
		AmountCents: o.TotalCents,
		Currency:    currency,
		PaidAt:      time.Now().UTC(),
	}

	s.mu.Lock()
	s.settled[o.ID] = result
	s.mu.Unlock()

	return Charge{
		Provider:     ProviderNameStub,
		Reference:    o.ID,
		Instructions: "Demo mode: this order auto-settles instantly. No real payment was taken.",
	}, nil
}

// Verify returns whatever Begin recorded for reference. Unknown references
// fail closed with an error rather than fabricating a Result.
func (s *StubProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: stub: reference is required")
	}
	s.mu.Lock()
	result, ok := s.settled[reference]
	s.mu.Unlock()
	if !ok {
		return Result{}, fmt.Errorf("payments: stub: unknown reference %q", reference)
	}
	return result, nil
}

// Webhook always fails: the stub settles synchronously inside Begin and
// has no webhook mechanism to validate, so there is nothing it could
// return here without fabricating a signature check that doesn't exist.
func (s *StubProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	return Result{}, errors.New("payments: stub: does not accept webhooks (it auto-settles inside Begin)")
}
