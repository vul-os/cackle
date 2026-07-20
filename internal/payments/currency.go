package payments

import (
	"fmt"
	"strconv"
	"strings"
)

// This file is shared infrastructure for every adapter in this package
// that has to convert between Cackle's own integer-minor-unit
// representation (AmountMinor, e.g. 1050 == R10.50 for ZAR) and whatever
// convention a given provider's API uses on the wire. Several providers in
// this set do NOT use "integer minor units" the way Paystack/Stripe/
// Razorpay do:
//
//   - Flutterwave, Xendit, Midtrans, Mercado Pago, dLocal, PayU and iyzico
//     all take (or can take) a decimal string/number in MAJOR units on
//     their public APIs (e.g. "100.50" meaning R100.50, not 10050).
//
// Getting this conversion wrong by one exponent is a 100x-money bug (see
// PAYMENTS-CONTRACT.md's warning about "cents" not being universal). Every
// adapter that needs to go minor-units <-> major-units string MUST route
// through minorUnitExponent/minorToMajorString/majorStringToMinor below
// rather than hardcoding "divide by 100" — so the zero- and three-decimal
// currencies below are handled correctly instead of silently mangled.
//
// internal/money (owned by a sibling agent building the v2 Provider seam)
// is intended to be Cackle's single canonical source for this table,
// used by order totals and UI formatting elsewhere in the codebase. This
// file is a self-contained, independently-testable local copy scoped to
// internal/payments' own provider-subunit conversions, so this package
// does not take on a hard build-order dependency on a sibling package
// that may not exist yet. The numbers below are the standard ISO 4217
// published minor-unit exponents and MUST match internal/money's table;
// if the two ever disagree, internal/money is the one to trust and this
// file should be updated to match it.

// zeroDecimalCurrencies are ISO 4217 currencies with NO minor unit at all
// (1 unit == 1 subunit, e.g. JPY 1000 is just ¥1000, never divided).
// Source: ISO 4217 currency & funds code list, ISO 4217 exponent column.
var zeroDecimalCurrencies = map[string]bool{
	"BIF": true, "CLP": true, "DJF": true, "GNF": true, "ISK": true,
	"JPY": true, "KMF": true, "KRW": true, "PYG": true, "RWF": true,
	"UGX": true, "VND": true, "VUV": true, "XAF": true, "XOF": true,
	"XPF": true,
}

// threeDecimalCurrencies are ISO 4217 currencies whose minor unit is
// 1/1000 of the major unit (1 KWD == 1000 fils), rather than the usual
// 1/100. Source: ISO 4217.
var threeDecimalCurrencies = map[string]bool{
	"BHD": true, "IQD": true, "JOD": true, "KWD": true,
	"LYD": true, "OMR": true, "TND": true,
}

// minorUnitExponent returns the number of decimal digits between an ISO
// 4217 currency's major unit and its minor unit: 0, 2 (the common case),
// or 3. Unknown/malformed codes default to 2, the overwhelmingly common
// case, but callers dealing with a provider that has an explicit
// supported-currency allowlist should prefer rejecting unknown codes
// outright over silently trusting this default.
func minorUnitExponent(currency string) int {
	c := strings.ToUpper(strings.TrimSpace(currency))
	switch {
	case zeroDecimalCurrencies[c]:
		return 0
	case threeDecimalCurrencies[c]:
		return 3
	default:
		return 2
	}
}

// minorToMajorString converts an integer minor-unit amount (Cackle's
// AmountMinor) into the decimal-string-in-major-units form several
// providers in this file expect on the wire (e.g. 10050 minor, ZAR ->
// "100.50"; 1000 minor, JPY -> "1000"; 1000 minor, KWD -> "1.000").
// amountMinor must be non-negative; negative amounts are refused by
// returning an error rather than emitting a nonsensical string.
func minorToMajorString(amountMinor int64, currency string) (string, error) {
	if amountMinor < 0 {
		return "", fmt.Errorf("payments: negative amount %d cannot be converted to a major-unit string", amountMinor)
	}
	exp := minorUnitExponent(currency)
	if exp == 0 {
		return strconv.FormatInt(amountMinor, 10), nil
	}
	div := int64(1)
	for i := 0; i < exp; i++ {
		div *= 10
	}
	whole := amountMinor / div
	frac := amountMinor % div
	return fmt.Sprintf("%d.%0*d", whole, exp, frac), nil
}

// majorStringToMinor is the inverse of minorToMajorString: it parses a
// decimal-string-in-major-units amount (as returned by a provider's API or
// webhook, e.g. Flutterwave/Xendit/Mercado Pago/PayU/iyzico's "amount"
// fields) back into Cackle's integer minor units, using the same
// per-currency exponent table so this is exact (never float) arithmetic.
// It fails closed on anything that doesn't parse as a plain non-negative
// decimal number with no more fractional digits than the currency's
// exponent allows (extra fractional precision is refused rather than
// silently truncated, since that could hide a rounding-based
// under-reconciliation).
func majorStringToMinor(s string, currency string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("payments: empty amount string")
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("payments: negative amount %q is not valid", s)
	}
	exp := minorUnitExponent(currency)

	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	for _, r := range whole {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("payments: malformed amount string %q", s)
		}
	}
	if hasFrac {
		for _, r := range frac {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("payments: malformed amount string %q", s)
			}
		}
		if len(frac) > exp {
			return 0, fmt.Errorf("payments: amount string %q has more fractional digits than %s's %d-decimal exponent", s, strings.ToUpper(currency), exp)
		}
		frac = frac + strings.Repeat("0", exp-len(frac))
	} else {
		frac = strings.Repeat("0", exp)
	}

	wholeN, err := strconv.ParseInt(whole, 10, 63)
	if err != nil {
		return 0, fmt.Errorf("payments: malformed amount string %q: %w", s, err)
	}
	var fracN int64
	if exp > 0 {
		fracN, err = strconv.ParseInt(frac, 10, 63)
		if err != nil {
			return 0, fmt.Errorf("payments: malformed amount string %q: %w", s, err)
		}
	}
	mul := int64(1)
	for i := 0; i < exp; i++ {
		mul *= 10
	}
	return wholeN*mul + fracN, nil
}
