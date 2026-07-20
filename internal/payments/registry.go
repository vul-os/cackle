package payments

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// EnvPaymentProviders configures which OPTIONAL providers are enabled in
// this deployment, as a comma-separated list of provider Name()s (e.g.
// "manual,stripe,paystack"). ProviderNameManual is always enabled
// regardless of this variable's value or absence — it is Cackle's default
// and cannot be disabled. See NewRegistryFromEnv.
const EnvPaymentProviders = "CACKLE_PAYMENT_PROVIDERS"

// Sentinel errors for provider selection. Callers should match with
// errors.Is.
var (
	// ErrProviderNotFound means no provider is registered under the
	// requested name.
	ErrProviderNotFound = errors.New("payments: provider not registered")
	// ErrProviderDisabled means a provider IS registered under the
	// requested name, but this deployment has not enabled it (see
	// EnvPaymentProviders) — a distinct, more actionable error than "not
	// found" for an operator who forgot to add a name to their config.
	ErrProviderDisabled = errors.New("payments: provider is registered but not enabled (see " + EnvPaymentProviders + ")")
	// ErrCurrencyNotSupported means the provider is found and enabled,
	// but its Capabilities().Currencies does not include the requested
	// currency. Select fails with this EARLY (before Begin ever makes a
	// network call), by design: "selecting a provider that cannot
	// handle an event's currency must fail clearly and early."
	ErrCurrencyNotSupported = errors.New("payments: provider does not support this currency")
)

// Registry looks up providers by name so callers never hardcode one, and
// filters them by capability and by which ones this deployment has chosen
// to enable.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	// enabled is nil when this Registry was constructed with NewRegistry
	// (no allowlist configured: every REGISTERED provider is considered
	// enabled — the permissive default for tests and single-provider
	// setups). NewRegistryFromEnv sets this to a concrete allowlist
	// (always including ProviderNameManual) once CACKLE_PAYMENT_PROVIDERS
	// is non-empty.
	enabled map[string]bool
}

// NewRegistry returns an empty Registry with no enablement restriction:
// every provider that gets Registered is considered enabled. Suitable for
// tests, and for callers that want to apply their own gating some other
// way. Production wiring that wants CACKLE_PAYMENT_PROVIDERS enforced
// should use NewRegistryFromEnv instead.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// NewRegistryFromEnv returns an empty Registry whose enablement is derived
// from env, a CACKLE_PAYMENT_PROVIDERS-shaped comma-separated list of
// provider names (case-insensitive, whitespace around entries is
// trimmed). ProviderNameManual is always enabled no matter what — it
// cannot be disabled, by design (see manual.go): Cackle must always have a
// zero-network-zero-API-key way to run an event.
//
// An empty/unset env enables every provider that ends up Registered
// (matching NewRegistry's permissive default) — a self-hoster who has only
// ever configured one provider doesn't need to also enumerate it. Once env
// is non-empty, ONLY the listed names (plus manual) are enabled: anything
// else Registered is still reachable via Get (e.g. admin/debug tooling)
// but List and Select will refuse to use it.
func NewRegistryFromEnv(env string) *Registry {
	r := NewRegistry()
	set := make(map[string]bool)
	any := false
	for _, part := range strings.Split(env, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		set[name] = true
		any = true
	}
	if any {
		set[ProviderNameManual] = true
		r.enabled = set
	}
	return r
}

// Register adds p under p.Name(). It refuses a nil provider, an empty
// name, a duplicate name, and — as defense in depth alongside the checks
// already inside NewStub — refuses to register anything named
// ProviderNameStub if a real Paystack secret is configured in this
// environment, so the demo/test provider can't end up live even if
// something upstream constructed it incorrectly.
//
// Register does not consult the enablement allowlist: registering a
// provider and enabling it for selection are separate concerns (an
// operator can register everything this binary was built with and let
// EnvPaymentProviders decide what's actually reachable). Use IsEnabled,
// List, or Select to respect enablement.
func (r *Registry) Register(p Provider) error {
	if p == nil {
		return errors.New("payments: cannot register a nil provider")
	}
	name := p.Name()
	if strings.TrimSpace(name) == "" {
		return errors.New("payments: provider Name() must not be empty")
	}
	if name == ProviderNameStub && realPaystackSecretConfigured() {
		return ErrStubRefusesRealSecret
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("payments: provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get looks up a provider by name, ignoring enablement — see IsEnabled if
// you need to know whether this deployment has chosen to allow it.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Names returns every REGISTERED provider name, sorted, regardless of
// enablement.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// IsEnabled reports whether name is allowed to be selected in this
// deployment. ProviderNameManual is always enabled. It does NOT require
// name to be registered — it only answers "would this name be allowed if
// it were registered", matching Select's own check order (enablement is
// checked independently of registration).
func (r *Registry) IsEnabled(name string) bool {
	if name == ProviderNameManual {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.enabled == nil {
		return true
	}
	return r.enabled[strings.ToLower(name)]
}

// CapabilityFilter narrows List to providers matching every non-empty
// field. An empty CapabilityFilter matches every enabled, registered
// provider.
type CapabilityFilter struct {
	// Currency, if non-empty, restricts to providers whose Capabilities
	// support this ISO-4217 code (via Capabilities.SupportsCurrency).
	Currency string
	// Country, if non-empty, restricts to providers whose Capabilities
	// support this ISO-3166-1 alpha-2 merchant country (via
	// Capabilities.SupportsCountry).
	Country string
	// Flow, if non-empty, restricts to providers whose Capabilities.Flow
	// equals this exactly.
	Flow Flow
}

// List returns every ENABLED, registered provider matching filter, sorted
// by Name(). Disabled providers (see IsEnabled) are never returned, even
// if they'd otherwise match the filter — this is the seam
// checkout/settings UIs should use to offer buyers/organisers a choice of
// provider without ever hardcoding provider names.
func (r *Registry) List(filter CapabilityFilter) []Provider {
	r.mu.RLock()
	all := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		all = append(all, p)
	}
	r.mu.RUnlock()

	sort.Slice(all, func(i, j int) bool { return all[i].Name() < all[j].Name() })

	out := make([]Provider, 0, len(all))
	for _, p := range all {
		if !r.IsEnabled(p.Name()) {
			continue
		}
		caps := p.Capabilities()
		if filter.Currency != "" && !caps.SupportsCurrency(filter.Currency) {
			continue
		}
		if filter.Country != "" && !caps.SupportsCountry(filter.Country) {
			continue
		}
		if filter.Flow != "" && caps.Flow != filter.Flow {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Select looks up name and returns it ONLY if it is both registered and
// enabled, and its Capabilities support currency. This is the entry point
// order-creation code should use instead of bare Get: it turns "picked a
// provider that can't actually handle this event's currency" into a
// clear, early, typed error (ErrProviderNotFound /
// ErrProviderDisabled / ErrCurrencyNotSupported) instead of a confusing
// failure deep inside Begin or — worse — a silently wrong charge.
func (r *Registry) Select(name, currency string) (Provider, error) {
	p, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
	}
	if !r.IsEnabled(name) {
		return nil, fmt.Errorf("%w: %q", ErrProviderDisabled, name)
	}
	if currency != "" && !p.Capabilities().SupportsCurrency(currency) {
		return nil, fmt.Errorf("%w: %q does not support %q", ErrCurrencyNotSupported, name, currency)
	}
	return p, nil
}
