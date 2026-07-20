package money

import (
	"errors"
	"testing"
)

// --- Exponent / Validate / Normalize / Lookup ---------------------------

func TestExponent_ZeroDecimalCurrencies(t *testing.T) {
	for _, code := range []string{"JPY", "KRW", "VND", "CLP", "ISK"} {
		exp, err := Exponent(code)
		if err != nil {
			t.Fatalf("Exponent(%q) = %v", code, err)
		}
		if exp != 0 {
			t.Fatalf("Exponent(%q) = %d, want 0", code, exp)
		}
	}
}

func TestExponent_ThreeDecimalCurrencies(t *testing.T) {
	for _, code := range []string{"KWD", "BHD", "JOD", "OMR", "TND"} {
		exp, err := Exponent(code)
		if err != nil {
			t.Fatalf("Exponent(%q) = %v", code, err)
		}
		if exp != 3 {
			t.Fatalf("Exponent(%q) = %d, want 3", code, exp)
		}
	}
}

func TestExponent_TwoDecimalCurrencies(t *testing.T) {
	for _, code := range []string{"USD", "EUR", "GBP", "ZAR", "NGN", "INR"} {
		exp, err := Exponent(code)
		if err != nil {
			t.Fatalf("Exponent(%q) = %v", code, err)
		}
		if exp != 2 {
			t.Fatalf("Exponent(%q) = %d, want 2", code, exp)
		}
	}
}

func TestExponent_UnknownCode(t *testing.T) {
	_, err := Exponent("XXX")
	if !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("Exponent(\"XXX\") = %v, want ErrUnknownCurrency", err)
	}
}

func TestExponent_InvalidShape(t *testing.T) {
	for _, code := range []string{"", "US", "USDD", "12A", "US$"} {
		_, err := Exponent(code)
		if !errors.Is(err, ErrInvalidCurrency) {
			t.Fatalf("Exponent(%q) = %v, want ErrInvalidCurrency", code, err)
		}
	}
}

func TestValidate_LowercaseAccepted(t *testing.T) {
	if err := Validate("usd"); err != nil {
		t.Fatalf("Validate(\"usd\") = %v, want nil", err)
	}
	if err := Validate("jpy"); err != nil {
		t.Fatalf("Validate(\"jpy\") = %v, want nil", err)
	}
}

func TestValidate_WhitespaceTrimmed(t *testing.T) {
	if err := Validate(" USD "); err != nil {
		t.Fatalf("Validate(\" USD \") = %v, want nil", err)
	}
}

func TestNormalize_Uppercases(t *testing.T) {
	norm, err := Normalize("usd")
	if err != nil {
		t.Fatalf("Normalize() = %v", err)
	}
	if norm != "USD" {
		t.Fatalf("Normalize(\"usd\") = %q, want USD", norm)
	}
}

func TestName_KnownCurrency(t *testing.T) {
	name, err := Name("JPY")
	if err != nil {
		t.Fatalf("Name(JPY) = %v", err)
	}
	if name == "" {
		t.Fatal("Name(JPY) = empty string")
	}
}

func TestSupportedCurrencies_ContainsCoreSet(t *testing.T) {
	set := make(map[string]bool)
	for _, c := range SupportedCurrencies() {
		set[c] = true
	}
	for _, c := range []string{"USD", "EUR", "JPY", "KWD", "ZAR", "NGN", "INR", "IDR", "BRL"} {
		if !set[c] {
			t.Fatalf("SupportedCurrencies() missing %q", c)
		}
	}
}

// --- Amount construction --------------------------------------------------

func TestNew_RejectsUnknownCurrency(t *testing.T) {
	_, err := New(100, "ZZZ")
	if !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("New() = %v, want ErrUnknownCurrency", err)
	}
}

func TestNew_NormalizesCurrency(t *testing.T) {
	a, err := New(100, "usd")
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	if a.Currency != "USD" {
		t.Fatalf("a.Currency = %q, want USD", a.Currency)
	}
}

// --- Add / Sub / Mul -------------------------------------------------------

func TestAdd_Success(t *testing.T) {
	a, _ := New(100, "USD")
	b, _ := New(250, "USD")
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add() = %v", err)
	}
	if sum.Minor != 350 || sum.Currency != "USD" {
		t.Fatalf("Add() = %+v, want {350 USD}", sum)
	}
}

func TestAdd_CrossCurrencyRejected(t *testing.T) {
	a, _ := New(100, "USD")
	b, _ := New(100, "EUR")
	_, err := a.Add(b)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Add() = %v, want ErrCurrencyMismatch", err)
	}
}

func TestSub_Success(t *testing.T) {
	a, _ := New(500, "USD")
	b, _ := New(200, "USD")
	diff, err := a.Sub(b)
	if err != nil {
		t.Fatalf("Sub() = %v", err)
	}
	if diff.Minor != 300 {
		t.Fatalf("Sub() = %+v, want Minor=300", diff)
	}
}

func TestSub_CrossCurrencyRejected(t *testing.T) {
	a, _ := New(500, "USD")
	b, _ := New(200, "ZAR")
	_, err := a.Sub(b)
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("Sub() = %v, want ErrCurrencyMismatch", err)
	}
}

func TestMul_Success(t *testing.T) {
	a, _ := New(1500, "USD") // $15.00 per ticket
	total, err := a.Mul(3)
	if err != nil {
		t.Fatalf("Mul() = %v", err)
	}
	if total.Minor != 4500 {
		t.Fatalf("Mul() = %+v, want Minor=4500", total)
	}
}

func TestAdd_OverflowRejected(t *testing.T) {
	a := Amount{Minor: maxInt64, Currency: "USD"}
	b := Amount{Minor: 1, Currency: "USD"}
	_, err := a.Add(b)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Add() = %v, want ErrOverflow", err)
	}
}

func TestSub_OverflowRejected(t *testing.T) {
	a := Amount{Minor: minInt64, Currency: "USD"}
	b := Amount{Minor: 1, Currency: "USD"}
	_, err := a.Sub(b)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Sub() = %v, want ErrOverflow", err)
	}
}

func TestMul_OverflowRejected(t *testing.T) {
	a := Amount{Minor: maxInt64, Currency: "USD"}
	_, err := a.Mul(2)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Mul() = %v, want ErrOverflow", err)
	}
}

func TestMul_MinInt64TimesNegativeOneRejected(t *testing.T) {
	a := Amount{Minor: minInt64, Currency: "USD"}
	_, err := a.Mul(-1)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("Mul(-1) on MinInt64 = %v, want ErrOverflow", err)
	}
}

func TestMul_ByZero(t *testing.T) {
	a := Amount{Minor: 12345, Currency: "USD"}
	zero, err := a.Mul(0)
	if err != nil {
		t.Fatalf("Mul(0) = %v", err)
	}
	if zero.Minor != 0 {
		t.Fatalf("Mul(0).Minor = %d, want 0", zero.Minor)
	}
}

func TestSub_MinInt64FromNegativeOne(t *testing.T) {
	// -1 - MinInt64 == MaxInt64, must not overflow or panic.
	a := Amount{Minor: -1, Currency: "USD"}
	b := Amount{Minor: minInt64, Currency: "USD"}
	got, err := a.Sub(b)
	if err != nil {
		t.Fatalf("Sub() = %v, want nil", err)
	}
	if got.Minor != maxInt64 {
		t.Fatalf("Sub() = %d, want MaxInt64 (%d)", got.Minor, int64(maxInt64))
	}
}

// --- Formatting -------------------------------------------------------------

func TestMajor_TwoDecimal(t *testing.T) {
	a, _ := New(123456, "USD")
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got != "1234.56" {
		t.Fatalf("Major() = %q, want 1234.56", got)
	}
}

func TestMajor_ZeroDecimal(t *testing.T) {
	a, _ := New(1000, "JPY")
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got != "1000" {
		t.Fatalf("Major() = %q, want 1000 (no decimal point for JPY)", got)
	}
}

func TestMajor_ThreeDecimal(t *testing.T) {
	a, _ := New(12345, "KWD")
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got != "12.345" {
		t.Fatalf("Major() = %q, want 12.345", got)
	}
}

func TestMajor_Negative(t *testing.T) {
	a, _ := New(-500, "USD")
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got != "-5.00" {
		t.Fatalf("Major() = %q, want -5.00", got)
	}
}

func TestMajor_SmallFractional(t *testing.T) {
	a, _ := New(5, "USD")
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got != "0.05" {
		t.Fatalf("Major() = %q, want 0.05", got)
	}
}

func TestMajor_MinInt64DoesNotPanic(t *testing.T) {
	a := Amount{Minor: minInt64, Currency: "USD"}
	got, err := a.Major()
	if err != nil {
		t.Fatalf("Major() = %v", err)
	}
	if got == "" {
		t.Fatal("Major() = empty string")
	}
}

func TestString_InvalidCurrencyDoesNotPanic(t *testing.T) {
	a := Amount{Minor: 100, Currency: "ZZZ"}
	got := a.String()
	if got == "" {
		t.Fatal("String() = empty")
	}
}

// --- ParseMajor -------------------------------------------------------------

func TestParseMajor_TwoDecimal(t *testing.T) {
	a, err := ParseMajor("12.34", "USD")
	if err != nil {
		t.Fatalf("ParseMajor() = %v", err)
	}
	if a.Minor != 1234 {
		t.Fatalf("ParseMajor().Minor = %d, want 1234", a.Minor)
	}
}

func TestParseMajor_ZeroDecimal(t *testing.T) {
	a, err := ParseMajor("1000", "JPY")
	if err != nil {
		t.Fatalf("ParseMajor() = %v", err)
	}
	if a.Minor != 1000 {
		t.Fatalf("ParseMajor().Minor = %d, want 1000", a.Minor)
	}
}

func TestParseMajor_ZeroDecimalRejectsFraction(t *testing.T) {
	_, err := ParseMajor("10.5", "JPY")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("ParseMajor(\"10.5\", JPY) = %v, want ErrInvalidAmount", err)
	}
}

func TestParseMajor_ThreeDecimal(t *testing.T) {
	a, err := ParseMajor("12.345", "KWD")
	if err != nil {
		t.Fatalf("ParseMajor() = %v", err)
	}
	if a.Minor != 12345 {
		t.Fatalf("ParseMajor().Minor = %d, want 12345", a.Minor)
	}
}

func TestParseMajor_RejectsTooMuchPrecision(t *testing.T) {
	_, err := ParseMajor("12.3456", "KWD") // KWD only allows 3 decimals
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("ParseMajor() = %v, want ErrInvalidAmount", err)
	}
}

func TestParseMajor_PadsShortFraction(t *testing.T) {
	a, err := ParseMajor("12.3", "USD")
	if err != nil {
		t.Fatalf("ParseMajor() = %v", err)
	}
	if a.Minor != 1230 {
		t.Fatalf("ParseMajor(\"12.3\").Minor = %d, want 1230", a.Minor)
	}
}

func TestParseMajor_Negative(t *testing.T) {
	a, err := ParseMajor("-5.00", "USD")
	if err != nil {
		t.Fatalf("ParseMajor() = %v", err)
	}
	if a.Minor != -500 {
		t.Fatalf("ParseMajor().Minor = %d, want -500", a.Minor)
	}
}

func TestParseMajor_RejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "abc", "12.3.4", "$5", "5,000", " "} {
		_, err := ParseMajor(in, "USD")
		if !errors.Is(err, ErrInvalidAmount) {
			t.Fatalf("ParseMajor(%q) = %v, want ErrInvalidAmount", in, err)
		}
	}
}

func TestParseMajor_UnknownCurrencyRejected(t *testing.T) {
	_, err := ParseMajor("12.34", "ZZZ")
	if !errors.Is(err, ErrUnknownCurrency) {
		t.Fatalf("ParseMajor() = %v, want ErrUnknownCurrency", err)
	}
}

// --- Round trip -------------------------------------------------------------

func TestRoundTrip_MajorThenParseMajor(t *testing.T) {
	for _, tc := range []struct {
		minor    int64
		currency string
	}{
		{123456, "USD"},
		{1000, "JPY"},
		{12345, "KWD"},
		{5, "USD"},
		{-500, "EUR"},
	} {
		a, err := New(tc.minor, tc.currency)
		if err != nil {
			t.Fatalf("New(%d, %s) = %v", tc.minor, tc.currency, err)
		}
		major, err := a.Major()
		if err != nil {
			t.Fatalf("Major() = %v", err)
		}
		back, err := ParseMajor(major, tc.currency)
		if err != nil {
			t.Fatalf("ParseMajor(%q, %s) = %v", major, tc.currency, err)
		}
		if back.Minor != tc.minor {
			t.Fatalf("round trip %d %s -> %q -> %d, want %d", tc.minor, tc.currency, major, back.Minor, tc.minor)
		}
	}
}
