package money

import (
	"fmt"
	"strconv"
	"strings"
)

// Amount is an exact monetary value: an integer count of the currency's
// minor units (e.g. cents for USD, whole yen for JPY, fils for KWD) plus
// the ISO-4217 currency it is denominated in. There is no float anywhere
// in this type or its methods — money is never approximated.
//
// The zero Amount{} is not generally useful (Currency == "" fails
// Validate) — construct with New, or build one from a trusted, already
// validated source and set Currency directly if you must, e.g. when
// decoding a stored row whose currency was validated at write time.
type Amount struct {
	Minor    int64
	Currency string // ISO-4217 alpha-3, uppercase, once constructed via New.
}

// New builds an Amount, normalizing and validating currency. This is the
// only constructor that guarantees the result's Currency is a known,
// canonical ISO-4217 code.
func New(minor int64, currency string) (Amount, error) {
	norm, err := Normalize(currency)
	if err != nil {
		return Amount{}, err
	}
	return Amount{Minor: minor, Currency: norm}, nil
}

// Zero returns a zero-value Amount in currency.
func Zero(currency string) (Amount, error) {
	return New(0, currency)
}

// IsZero reports whether the amount is exactly zero.
func (a Amount) IsZero() bool { return a.Minor == 0 }

// IsPositive reports whether the amount is strictly greater than zero.
func (a Amount) IsPositive() bool { return a.Minor > 0 }

// IsNegative reports whether the amount is strictly less than zero.
func (a Amount) IsNegative() bool { return a.Minor < 0 }

// sameCurrency reports whether a and b are denominated in the same
// currency, case-insensitively (so an Amount built by hand with a
// lowercase Currency still compares correctly against one built via New).
func sameCurrency(a, b Amount) bool {
	return strings.EqualFold(a.Currency, b.Currency)
}

// Add returns a+b. It refuses (ErrCurrencyMismatch) if a and b are
// denominated in different currencies, and refuses (ErrOverflow) if the
// sum would overflow int64. Cackle never silently converts currencies or
// wraps on overflow.
func (a Amount) Add(b Amount) (Amount, error) {
	if !sameCurrency(a, b) {
		return Amount{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, a.Currency, b.Currency)
	}
	sum, ok := addInt64(a.Minor, b.Minor)
	if !ok {
		return Amount{}, fmt.Errorf("%w: %d + %d", ErrOverflow, a.Minor, b.Minor)
	}
	return Amount{Minor: sum, Currency: a.Currency}, nil
}

// Sub returns a-b, with the same currency-match and overflow rules as Add.
func (a Amount) Sub(b Amount) (Amount, error) {
	if !sameCurrency(a, b) {
		return Amount{}, fmt.Errorf("%w: %s - %s", ErrCurrencyMismatch, a.Currency, b.Currency)
	}
	diff, ok := subInt64(a.Minor, b.Minor)
	if !ok {
		return Amount{}, fmt.Errorf("%w: %d - %d", ErrOverflow, a.Minor, b.Minor)
	}
	return Amount{Minor: diff, Currency: a.Currency}, nil
}

// Mul returns a scaled by an integer factor (e.g. quantity × unit price).
// There is deliberately no Mul-by-fraction/float: splitting a fee or
// applying a percentage must be done in integer minor units by the caller
// (round explicitly, don't let this type hide a rounding decision).
func (a Amount) Mul(factor int64) (Amount, error) {
	product, ok := mulInt64(a.Minor, factor)
	if !ok {
		return Amount{}, fmt.Errorf("%w: %d * %d", ErrOverflow, a.Minor, factor)
	}
	return Amount{Minor: product, Currency: a.Currency}, nil
}

// addInt64 adds two int64s, reporting ok=false on overflow instead of
// wrapping.
func addInt64(a, b int64) (sum int64, ok bool) {
	sum = a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

// subInt64 subtracts two int64s, reporting ok=false on overflow. Guards
// the a - math.MinInt64 case explicitly since -math.MinInt64 itself
// overflows int64: mathematically a - MinInt64 == a + 2^63, which only
// fits in int64 when a is negative, and even then must be computed as
// (a+1)+MaxInt64 to avoid any intermediate overflowing.
func subInt64(a, b int64) (diff int64, ok bool) {
	if b == minInt64 {
		if a >= 0 {
			return 0, false
		}
		return (a + 1) + maxInt64, true
	}
	return addInt64(a, -b)
}

// mulInt64 multiplies two int64s, reporting ok=false on overflow.
func mulInt64(a, b int64) (product int64, ok bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product = a * b
	if product/b != a {
		return 0, false
	}
	// The one case product/b != a doesn't catch: MinInt64 * -1, which
	// mathematically overflows to MaxInt64+1 but division masks it
	// because MinInt64/-1 is itself the overflow case Go computes as
	// MinInt64 (two's complement wraparound) — guard explicitly.
	if a == minInt64 && b == -1 {
		return 0, false
	}
	if b == minInt64 && a == -1 {
		return 0, false
	}
	return product, true
}

const (
	maxInt64 = 1<<63 - 1
	minInt64 = -1 << 63
)

// Major formats the amount in major units as a decimal string using the
// currency's own exponent (e.g. 12345 minor JPY -> "12345", 12345 minor
// USD -> "123.45", 12345 minor KWD -> "12.345"). It never uses a hardcoded
// symbol or hardcoded decimal count — that's the entire point of this
// package. Use this at provider edges and for any non-Intl.NumberFormat
// display path (e.g. server-rendered text, logs, receipts).
func (a Amount) Major() (string, error) {
	exp, err := Exponent(a.Currency)
	if err != nil {
		return "", err
	}
	return formatMinor(a.Minor, exp), nil
}

// String is a best-effort Stringer for logging/debugging: "<major>
// <CUR>", e.g. "123.45 USD". If Currency is invalid it falls back to a
// raw representation rather than panicking — callers that need a
// guaranteed-correct, error-checked format should use Major instead.
func (a Amount) String() string {
	major, err := a.Major()
	if err != nil {
		return fmt.Sprintf("%d minor %s (invalid currency)", a.Minor, a.Currency)
	}
	return major + " " + a.Currency
}

// formatMinor renders minor units as a decimal string with exp digits
// after the point (no point at all if exp is 0). Implemented on unsigned
// magnitude throughout to avoid the int64 overflow that naive negation of
// math.MinInt64 would hit.
func formatMinor(minor int64, exp int) string {
	neg := minor < 0
	mag := absInt64AsUint64(minor)

	if exp <= 0 {
		s := strconv.FormatUint(mag, 10)
		if neg {
			s = "-" + s
		}
		return s
	}

	div := uint64(1)
	for i := 0; i < exp; i++ {
		div *= 10
	}
	intPart := mag / div
	fracPart := mag % div
	fracStr := strconv.FormatUint(fracPart, 10)
	for len(fracStr) < exp {
		fracStr = "0" + fracStr
	}
	s := strconv.FormatUint(intPart, 10) + "." + fracStr
	if neg {
		s = "-" + s
	}
	return s
}

// absInt64AsUint64 returns |x| as a uint64 without overflowing for
// x == math.MinInt64 (where -x is not representable as an int64).
func absInt64AsUint64(x int64) uint64 {
	if x >= 0 {
		return uint64(x)
	}
	// -(x+1) is safe (x+1 > MinInt64), then add 1 in uint64 space.
	return uint64(-(x + 1)) + 1
}

// ParseMajor parses a decimal major-unit string (e.g. "123.45", "-0.5",
// "1000" for a zero-decimal currency) into an Amount denominated in
// currency, using the currency's own exponent. It is strict: it rejects
// more fractional digits than the currency's exponent allows rather than
// rounding or truncating (silently losing precision on a money value is
// exactly the kind of bug this package exists to prevent) — callers that
// genuinely need to round should round explicitly before calling this.
func ParseMajor(major string, currency string) (Amount, error) {
	norm, err := Normalize(currency)
	if err != nil {
		return Amount{}, err
	}
	exp, err := Exponent(norm)
	if err != nil {
		return Amount{}, err
	}
	minor, err := parseDecimalToMinor(major, exp)
	if err != nil {
		return Amount{}, err
	}
	return Amount{Minor: minor, Currency: norm}, nil
}

func parseDecimalToMinor(s string, exp int) (int64, error) {
	orig := s
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: empty amount", ErrInvalidAmount)
	}

	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart = s[:i]
		fracPart = s[i+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || (fracPart != "" && !isDigits(fracPart)) {
		return 0, fmt.Errorf("%w: %q is not a decimal number", ErrInvalidAmount, orig)
	}
	if len(fracPart) > exp {
		return 0, fmt.Errorf("%w: %q has more precision than this currency allows (%d decimal place(s))", ErrInvalidAmount, orig, exp)
	}
	for len(fracPart) < exp {
		fracPart += "0"
	}

	combined := intPart + fracPart
	// Trim leading zeros so ParseInt doesn't choke on e.g. "007"; keep at
	// least one digit.
	combined = strings.TrimLeft(combined, "0")
	if combined == "" {
		combined = "0"
	}

	val, err := strconv.ParseInt(combined, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is out of range: %v", ErrInvalidAmount, orig, err)
	}
	if neg {
		val = -val
	}
	return val, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
