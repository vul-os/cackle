package payments

import (
	"errors"
	"testing"
)

func mustManual(t *testing.T) *ManualProvider {
	t.Helper()
	return NewManual(nil)
}

// --- enablement from config -------------------------------------------

func TestNewRegistryFromEnv_EmptyEnablesEverythingRegistered(t *testing.T) {
	r := NewRegistryFromEnv("")
	fake := &fakeProvider{name: "acme"}
	if err := r.Register(fake); err != nil {
		t.Fatalf("Register() = %v", err)
	}
	if !r.IsEnabled("acme") {
		t.Fatal("IsEnabled(acme) = false, want true when CACKLE_PAYMENT_PROVIDERS is unset")
	}
}

func TestNewRegistryFromEnv_RestrictsToListedNames(t *testing.T) {
	r := NewRegistryFromEnv("stripe, paystack")
	if !r.IsEnabled("stripe") {
		t.Fatal("IsEnabled(stripe) = false, want true (listed)")
	}
	if !r.IsEnabled("paystack") {
		t.Fatal("IsEnabled(paystack) = false, want true (listed)")
	}
	if r.IsEnabled("acme") {
		t.Fatal("IsEnabled(acme) = true, want false (not listed)")
	}
}

func TestNewRegistryFromEnv_CaseInsensitiveAndTrimmed(t *testing.T) {
	r := NewRegistryFromEnv(" Stripe ,PAYSTACK")
	if !r.IsEnabled("stripe") || !r.IsEnabled("paystack") {
		t.Fatal("IsEnabled should be case-insensitive and trim whitespace around entries")
	}
}

func TestNewRegistryFromEnv_ManualAlwaysEnabled(t *testing.T) {
	// Even when manual is deliberately left OFF the allowlist, and even
	// when the allowlist is otherwise restrictive, manual must remain
	// enabled: it cannot be disabled.
	r := NewRegistryFromEnv("stripe")
	if !r.IsEnabled(ProviderNameManual) {
		t.Fatal("IsEnabled(manual) = false, want true (manual can never be disabled)")
	}
}

func TestRegistry_ManualAlwaysEnabledEvenUnregistered(t *testing.T) {
	r := NewRegistryFromEnv("stripe")
	// IsEnabled(manual) must be true even before manual is Registered —
	// enablement and registration are independent checks.
	if !r.IsEnabled(ProviderNameManual) {
		t.Fatal("IsEnabled(manual) = false, want true")
	}
}

// --- List / capability filtering ---------------------------------------

func TestRegistry_ListFiltersDisabledProviders(t *testing.T) {
	r := NewRegistryFromEnv("stripe") // only stripe (+manual) enabled
	acme := &fakeProvider{name: "acme", caps: Capabilities{Flow: FlowRedirect}}
	stripe := &fakeProvider{name: "stripe", caps: Capabilities{Flow: FlowRedirect}}
	if err := r.Register(acme); err != nil {
		t.Fatalf("Register(acme) = %v", err)
	}
	if err := r.Register(stripe); err != nil {
		t.Fatalf("Register(stripe) = %v", err)
	}

	list := r.List(CapabilityFilter{})
	names := map[string]bool{}
	for _, p := range list {
		names[p.Name()] = true
	}
	if names["acme"] {
		t.Fatal("List() included a disabled provider")
	}
	if !names["stripe"] {
		t.Fatal("List() excluded an enabled provider")
	}
}

func TestRegistry_ListFiltersByCurrency(t *testing.T) {
	r := NewRegistry()
	usdOnly := &fakeProvider{name: "usd-only", caps: Capabilities{Currencies: []string{"USD"}}}
	broad := &fakeProvider{name: "broad", caps: Capabilities{}}
	r.Register(usdOnly)
	r.Register(broad)

	list := r.List(CapabilityFilter{Currency: "ZAR"})
	for _, p := range list {
		if p.Name() == "usd-only" {
			t.Fatal("List(currency=ZAR) included a USD-only provider")
		}
	}
	names := map[string]bool{}
	for _, p := range list {
		names[p.Name()] = true
	}
	if !names["broad"] {
		t.Fatal("List(currency=ZAR) excluded a provider with unrestricted currencies")
	}
}

func TestRegistry_ListFiltersByCountryAndFlow(t *testing.T) {
	r := NewRegistry()
	za := &fakeProvider{name: "za-redirect", caps: Capabilities{Countries: []string{"ZA"}, Flow: FlowRedirect}}
	ng := &fakeProvider{name: "ng-inline", caps: Capabilities{Countries: []string{"NG"}, Flow: FlowInline}}
	r.Register(za)
	r.Register(ng)

	list := r.List(CapabilityFilter{Country: "ZA"})
	if len(list) != 1 || list[0].Name() != "za-redirect" {
		t.Fatalf("List(country=ZA) = %v, want only za-redirect", namesOf(list))
	}

	list = r.List(CapabilityFilter{Flow: FlowInline})
	if len(list) != 1 || list[0].Name() != "ng-inline" {
		t.Fatalf("List(flow=inline) = %v, want only ng-inline", namesOf(list))
	}
}

func TestRegistry_ListIsSortedByName(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeProvider{name: "zeta"})
	r.Register(&fakeProvider{name: "alpha"})
	r.Register(&fakeProvider{name: "mid"})

	list := r.List(CapabilityFilter{})
	got := namesOf(list)
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List() = %v, want %v", got, want)
		}
	}
}

func namesOf(providers []Provider) []string {
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p.Name()
	}
	return out
}

// --- Select --------------------------------------------------------------

func TestRegistry_SelectNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Select("nonexistent", "USD")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("Select() = %v, want ErrProviderNotFound", err)
	}
}

func TestRegistry_SelectDisabled(t *testing.T) {
	r := NewRegistryFromEnv("stripe")
	acme := &fakeProvider{name: "acme", caps: Capabilities{}}
	r.Register(acme)
	_, err := r.Select("acme", "USD")
	if !errors.Is(err, ErrProviderDisabled) {
		t.Fatalf("Select() = %v, want ErrProviderDisabled", err)
	}
}

func TestRegistry_SelectCurrencyNotSupported(t *testing.T) {
	r := NewRegistry()
	usdOnly := &fakeProvider{name: "usd-only", caps: Capabilities{Currencies: []string{"USD"}}}
	r.Register(usdOnly)
	_, err := r.Select("usd-only", "ZAR")
	if !errors.Is(err, ErrCurrencyNotSupported) {
		t.Fatalf("Select() = %v, want ErrCurrencyNotSupported", err)
	}
}

func TestRegistry_SelectSuccess(t *testing.T) {
	r := NewRegistry()
	usdOnly := &fakeProvider{name: "usd-only", caps: Capabilities{Currencies: []string{"USD"}}}
	r.Register(usdOnly)
	p, err := r.Select("usd-only", "USD")
	if err != nil {
		t.Fatalf("Select() = %v, want nil", err)
	}
	if p.Name() != "usd-only" {
		t.Fatalf("Select() = %v, want usd-only", p.Name())
	}
}

func TestRegistry_SelectManualAlwaysWorksForAnyCurrency(t *testing.T) {
	r := NewRegistryFromEnv("stripe") // manual not explicitly listed
	m := mustManual(t)
	if err := r.Register(m); err != nil {
		t.Fatalf("Register(manual) = %v", err)
	}
	for _, cur := range []string{"USD", "JPY", "KWD", "ZAR"} {
		if _, err := r.Select(ProviderNameManual, cur); err != nil {
			t.Fatalf("Select(manual, %s) = %v, want nil", cur, err)
		}
	}
}

func TestRegistry_SelectEmptyCurrencySkipsCurrencyCheck(t *testing.T) {
	r := NewRegistry()
	usdOnly := &fakeProvider{name: "usd-only", caps: Capabilities{Currencies: []string{"USD"}}}
	r.Register(usdOnly)
	if _, err := r.Select("usd-only", ""); err != nil {
		t.Fatalf("Select() with empty currency = %v, want nil (no currency check requested)", err)
	}
}

// --- Capabilities.SupportsCurrency / SupportsCountry --------------------

func TestCapabilities_SupportsCurrency_EmptyMeansUnrestricted(t *testing.T) {
	c := Capabilities{}
	if !c.SupportsCurrency("ZAR") {
		t.Fatal("empty Currencies should mean unrestricted")
	}
}

func TestCapabilities_SupportsCurrency_CaseInsensitive(t *testing.T) {
	c := Capabilities{Currencies: []string{"USD", "EUR"}}
	if !c.SupportsCurrency("usd") {
		t.Fatal("SupportsCurrency should be case-insensitive")
	}
	if c.SupportsCurrency("ZAR") {
		t.Fatal("SupportsCurrency(ZAR) = true, want false")
	}
}

func TestCapabilities_SupportsCountry_EmptyMeansUnrestricted(t *testing.T) {
	c := Capabilities{}
	if !c.SupportsCountry("ZA") {
		t.Fatal("empty Countries should mean unrestricted")
	}
}
