package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vul-os/cackle/internal/money"
)

// ProviderNameManual is the stable Name() the manual provider registers
// under. It is Cackle's DEFAULT provider — always registered, always
// enabled (see Registry.IsEnabled), and cannot be turned off via
// EnvPaymentProviders. This is deliberate: Cackle must always have a way
// to run a real event with zero API keys, zero compliance surface, and
// zero network calls, in any country.
const ProviderNameManual = "manual"

// ErrManualNoWebhook is returned unconditionally by ManualProvider.Webhook:
// there is no signature to validate and no push channel to receive from —
// settlement only ever happens via MarkPaid.
var ErrManualNoWebhook = errors.New("payments: manual: does not support webhooks; call MarkPaid to record settlement")

// ManualInstructions builds the buyer-facing text Begin returns for an
// order: bank details, "pay at the door", an invoice reference, a mobile
// money number — whatever an organiser has configured. Cackle never
// generates or validates the CONTENT of these instructions; it is opaque
// operator-supplied text. Implementations should be pure and side-effect
// free (no network calls, no mutation) — Begin will surface any error as
// a failure to start the order.
type ManualInstructions func(o Order) (string, error)

// DefaultManualInstructions is used when NewManual is given a nil
// ManualInstructions: a generic reference the organiser and buyer can use
// to match a payment to an order, formatted via internal/money so it never
// mis-renders a zero- or three-decimal currency.
func DefaultManualInstructions(o Order) (string, error) {
	amount, err := money.New(o.AmountMinor, o.Currency)
	if err != nil {
		return "", fmt.Errorf("payments: manual: %w", err)
	}
	major, err := amount.Major()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"Pay %s %s, quoting order reference %s. Ask the organiser to confirm and mark this order paid once they've received it.",
		major, amount.Currency, o.Reference,
	), nil
}

// ManualRecord is the full recorded state of one manual order, including
// the audit fields (MarkedBy/MarkedAt) that Result does not carry — see
// ManualProvider.Record.
type ManualRecord struct {
	Reference    string
	AmountMinor  int64
	Currency     string
	Status       Status
	Instructions string
	CreatedAt    time.Time
	// MarkedBy identifies who last changed Status away from pending (an
	// organiser id/email — whatever the caller of MarkPaid/MarkFailed
	// passed in). Empty while Status is still StatusPending. THIS is the
	// auditable "who marked it paid" the payments contract requires.
	MarkedBy string
	// MarkedAt is when that happened. Zero while Status is still
	// StatusPending.
	MarkedAt time.Time
}

// ManualProvider is Cackle's default, always-available payment provider.
// It makes NO network calls whatsoever, anywhere:
//
//   - Begin validates the order and returns buyer-facing instructions
//     (bank details / pay-at-the-door / invoice text) — nothing is
//     contacted, nothing settles yet.
//   - Settlement happens ONLY when an organiser explicitly calls MarkPaid.
//     This is the auditable action the payments contract requires: the
//     record captures WHO marked it paid and WHEN. This package has no
//     notion of authorization itself — callers MUST authenticate/authorize
//     the organiser before calling MarkPaid; markedBy is trusted verbatim
//     as the audit identity.
//   - Verify reports whatever is currently recorded. It never contacts
//     anything — there is nothing to contact.
//   - Webhook is unconditionally unsupported (ErrManualNoWebhook):
//     accepting a "manual provider webhook" would mean trusting an
//     unauthenticated HTTP request to settle money with no signature
//     scheme to check it against, which is exactly the failure mode this
//     package exists to prevent.
//
// State here (the records map) is in-memory only, guarded by mu. This is
// the reference behaviour for tests and small deployments; a durable,
// database-backed audit trail (organiser id, timestamp, possibly IP/UA,
// all persisted in internal/store) is the separate, later migration pass
// described in the payments contract. Callers that need durability across
// process restarts should treat MarkPaid/MarkFailed as the integration
// point at which to ALSO write their own persisted audit row — this type
// does not replace that, it is the provider-level source of truth used by
// Verify/Reconcile in the meantime.
type ManualProvider struct {
	mu           sync.Mutex
	records      map[string]*ManualRecord
	instructions ManualInstructions
	// store, if non-nil, makes every state transition below durable
	// across a process restart — see NewManualWithStore. nil (the
	// default from NewManual) preserves the original in-memory-only
	// behaviour every existing test in this package relies on.
	store RecordStore
}

// NewManual constructs the manual provider with in-memory-only state
// (does not survive a process restart). instructionsFn generates the
// buyer-facing text for a given order; pass nil to use
// DefaultManualInstructions. instructionsFn may be per-org configuration
// (e.g. a closure capturing an org's bank details) supplied by the
// caller — this package has no concept of "an org's bank details" itself.
// Production wiring that wants restart durability should use
// NewManualWithStore instead.
func NewManual(instructionsFn ManualInstructions) *ManualProvider {
	if instructionsFn == nil {
		instructionsFn = DefaultManualInstructions
	}
	return &ManualProvider{
		records:      make(map[string]*ManualRecord),
		instructions: instructionsFn,
	}
}

// NewManualWithStore constructs the manual provider backed by rs: every
// Begin/MarkPaid/MarkFailed call is durably persisted, and any existing
// records for this provider are warm-loaded into memory up front — so a
// process restart (or a fresh process picking up where a previous one
// left off) never loses an order's manual-payment state or its audit
// trail (MarkedBy/MarkedAt). Pass nil rs to get exactly NewManual's
// in-memory-only behaviour.
func NewManualWithStore(ctx context.Context, instructionsFn ManualInstructions, rs RecordStore) (*ManualProvider, error) {
	m := NewManual(instructionsFn)
	m.store = rs
	if rs == nil {
		return m, nil
	}
	existing, err := rs.ListPaymentRecords(ctx, ProviderNameManual)
	if err != nil {
		return nil, fmt.Errorf("payments: manual: load persisted records: %w", err)
	}
	for _, rec := range existing {
		m.records[rec.Reference] = &ManualRecord{
			Reference:    rec.Reference,
			AmountMinor:  rec.AmountMinor,
			Currency:     rec.Currency,
			Status:       rec.Status,
			Instructions: rec.Instructions,
			MarkedBy:     rec.MarkedBy,
			MarkedAt:     rec.MarkedAt,
			CreatedAt:    rec.CreatedAt,
		}
	}
	return m, nil
}

// persistLocked writes rec's current state to m.store, if configured.
// Callers must hold m.mu. A no-op (nil error) when m.store is nil.
func (m *ManualProvider) persistLocked(ctx context.Context, rec *ManualRecord) error {
	if m.store == nil {
		return nil
	}
	updatedAt := rec.CreatedAt
	if !rec.MarkedAt.IsZero() {
		updatedAt = rec.MarkedAt
	}
	err := m.store.PutPaymentRecord(ctx, PaymentRecord{
		Provider:     ProviderNameManual,
		Reference:    rec.Reference,
		AmountMinor:  rec.AmountMinor,
		Currency:     rec.Currency,
		Status:       rec.Status,
		Instructions: rec.Instructions,
		MarkedBy:     rec.MarkedBy,
		MarkedAt:     rec.MarkedAt,
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    updatedAt,
	})
	if err != nil {
		return fmt.Errorf("payments: manual: persist record: %w", err)
	}
	return nil
}

// Name implements Provider.
func (m *ManualProvider) Name() string { return ProviderNameManual }

// Capabilities implements Provider. The manual provider handles every
// ISO-4217 currency correctly (Currencies is empty/unrestricted, and
// ZeroDecimalOK is true because DefaultManualInstructions/Verify always go
// through internal/money rather than assuming two decimal places), makes
// no webhook calls, and never redirects the buyer anywhere.
func (m *ManualProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    nil, // every currency internal/money knows about
		Countries:     nil, // every country — there is no merchant account
		Flow:          FlowManual,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      false,
		ZeroDecimalOK: true,
	}
}

// Begin validates the order and returns buyer-facing instructions. It is
// idempotent: calling Begin again for an order id that already began
// returns the SAME recorded instructions rather than erroring or
// resetting state (safe for a checkout page retry/refresh).
func (m *ManualProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	id := strings.TrimSpace(o.Reference)
	if id == "" {
		return Charge{}, errors.New("payments: manual: order id is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: manual: amount_minor must be positive")
	}
	currency, err := money.Normalize(o.Currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: manual: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.records[id]; ok {
		return Charge{
			Provider:     ProviderNameManual,
			Reference:    existing.Reference,
			Instructions: existing.Instructions,
		}, nil
	}

	oNorm := o
	oNorm.Currency = currency
	text, err := m.instructions(oNorm)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: manual: generating instructions: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return Charge{}, errors.New("payments: manual: instructions must not be empty")
	}

	rec := &ManualRecord{
		Reference:    id,
		AmountMinor:  o.AmountMinor,
		Currency:     currency,
		Status:       StatusPending,
		Instructions: text,
		CreatedAt:    time.Now().UTC(),
	}
	if err := m.persistLocked(ctx, rec); err != nil {
		return Charge{}, err
	}
	m.records[id] = rec
	return Charge{
		Provider:     ProviderNameManual,
		Reference:    id,
		Instructions: text,
	}, nil
}

// MarkPaid is the ONLY way a manual order becomes StatusPaid. Callers MUST
// authenticate and authorize the caller (e.g. "is this user an organiser
// of this event") BEFORE calling MarkPaid — this package has no notion of
// authorization and trusts markedBy verbatim as the audit identity.
// markedBy must not be empty: recording WHO marked an order paid is the
// entire point of this provider's audit trail.
//
// Calling MarkPaid again for an already-paid reference is idempotent and
// does not overwrite the original MarkedBy/MarkedAt — the FIRST mark is
// what the audit trail should reflect. Calling it for a reference that was
// previously marked failed DOES transition it to paid (an organiser
// correcting a mistake), updating MarkedBy/MarkedAt to this action.
func (m *ManualProvider) MarkPaid(ctx context.Context, reference, markedBy string) (Result, error) {
	return m.mark(ctx, reference, markedBy, StatusPaid)
}

// MarkFailed lets an organiser explicitly record that a manual order will
// never be paid (buyer backed out, event cancelled, duplicate order,
// ...). Same authorization and auditing rules as MarkPaid.
func (m *ManualProvider) MarkFailed(ctx context.Context, reference, markedBy string) (Result, error) {
	return m.mark(ctx, reference, markedBy, StatusFailed)
}

func (m *ManualProvider) mark(ctx context.Context, reference, markedBy string, status Status) (Result, error) {
	reference = strings.TrimSpace(reference)
	markedBy = strings.TrimSpace(markedBy)
	if reference == "" {
		return Result{}, errors.New("payments: manual: reference is required")
	}
	if markedBy == "" {
		return Result{}, errors.New("payments: manual: markedBy is required (this is an auditable action and needs an actor)")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[reference]
	if !ok {
		return Result{}, fmt.Errorf("payments: manual: unknown reference %q", reference)
	}

	if status == StatusPaid && rec.Status == StatusPaid {
		// Idempotent: don't clobber the original marker/timestamp.
		return m.resultLocked(rec), nil
	}

	prevStatus, prevMarkedBy, prevMarkedAt := rec.Status, rec.MarkedBy, rec.MarkedAt
	rec.Status = status
	rec.MarkedBy = markedBy
	rec.MarkedAt = time.Now().UTC()
	if err := m.persistLocked(ctx, rec); err != nil {
		// Fail closed on the durability guarantee: roll back the in-memory
		// mutation so this replica's view doesn't disagree with what was
		// actually persisted (or wasn't). The caller sees an error and,
		// for MarkPaid/MarkFailed being idempotent, can safely retry.
		rec.Status, rec.MarkedBy, rec.MarkedAt = prevStatus, prevMarkedBy, prevMarkedAt
		return Result{}, err
	}
	return m.resultLocked(rec), nil
}

// Verify reports the currently recorded state for reference. It NEVER
// contacts anything — there is nothing to contact; this is the entire
// premise of the manual provider. Unknown references fail closed with an
// error rather than fabricating a Result.
func (m *ManualProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: manual: reference is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[reference]
	if !ok {
		return Result{}, fmt.Errorf("payments: manual: unknown reference %q", reference)
	}
	return m.resultLocked(rec), nil
}

// Webhook always fails: the manual provider has no signature scheme and
// no push channel. Settlement only ever happens via MarkPaid. Accepting
// an unauthenticated "webhook" here would defeat the entire point of this
// provider being the safe, no-network default.
func (m *ManualProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	return Result{}, ErrManualNoWebhook
}

// Record returns a copy of the currently recorded state for reference,
// including the audit fields (MarkedBy/MarkedAt) that Result does not
// carry. Callers building an organiser-facing "who marked this paid" view
// should use this rather than Verify.
func (m *ManualProvider) Record(reference string) (ManualRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[strings.TrimSpace(reference)]
	if !ok {
		return ManualRecord{}, false
	}
	return *rec, true
}

// resultLocked builds a Result from rec. Callers must hold m.mu.
func (m *ManualProvider) resultLocked(rec *ManualRecord) Result {
	result := Result{
		Provider:    ProviderNameManual,
		Reference:   rec.Reference,
		Status:      rec.Status,
		AmountMinor: rec.AmountMinor,
		Currency:    rec.Currency,
	}
	if rec.Status == StatusPaid {
		result.PaidAt = rec.MarkedAt
		// EventID must be non-empty whenever Status is StatusPaid (see
		// checkReplay/SeenStore) so a webhook-style dedupe path — if a
		// caller chooses to route MarkPaid results through one — still
		// has something to key on. Derived from the marking timestamp so
		// a later re-mark (idempotent no-op above) still reports the
		// same id, and a genuine paid->failed->paid cycle produces a new
		// one.
		result.EventID = fmt.Sprintf("manual-%s-%d", rec.Reference, rec.MarkedAt.UnixNano())
	}
	return result
}
