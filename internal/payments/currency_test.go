package payments

import "testing"

func TestMinorUnitExponent(t *testing.T) {
	cases := map[string]int{
		"ZAR": 2, "USD": 2, "usd": 2, "": 2,
		"JPY": 0, "KRW": 0, "VND": 0, "CLP": 0, "ISK": 0,
		"KWD": 3, "BHD": 3, "JOD": 3, "OMR": 3, "TND": 3, "IQD": 3, "LYD": 3,
	}
	for currency, want := range cases {
		if got := minorUnitExponent(currency); got != want {
			t.Errorf("minorUnitExponent(%q) = %d, want %d", currency, got, want)
		}
	}
}

func TestMinorToMajorString(t *testing.T) {
	cases := []struct {
		minor    int64
		currency string
		want     string
	}{
		{10050, "ZAR", "100.50"},
		{100, "USD", "1.00"},
		{5, "USD", "0.05"},
		{1000, "JPY", "1000"},
		{1, "JPY", "1"},
		{1000, "KWD", "1.000"},
		{1500, "KWD", "1.500"},
		{0, "ZAR", "0.00"},
	}
	for _, c := range cases {
		got, err := minorToMajorString(c.minor, c.currency)
		if err != nil {
			t.Errorf("minorToMajorString(%d, %q) error: %v", c.minor, c.currency, err)
			continue
		}
		if got != c.want {
			t.Errorf("minorToMajorString(%d, %q) = %q, want %q", c.minor, c.currency, got, c.want)
		}
	}
}

func TestMinorToMajorString_RejectsNegative(t *testing.T) {
	if _, err := minorToMajorString(-100, "USD"); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestMajorStringToMinor(t *testing.T) {
	cases := []struct {
		s        string
		currency string
		want     int64
	}{
		{"100.50", "ZAR", 10050},
		{"1", "USD", 100},
		{"1.00", "USD", 100},
		{"0.05", "USD", 5},
		{"1000", "JPY", 1000},
		{"1000.00", "JPY", 1000}, // extra .00 zero-fraction still valid (all zero, within exponent 0? see below)
		{"1.000", "KWD", 1000},
		{"1.5", "KWD", 1500},
		{"0", "ZAR", 0},
		{".5", "USD", 50},
	}
	for _, c := range cases {
		got, err := majorStringToMinor(c.s, c.currency)
		if c.s == "1000.00" && c.currency == "JPY" {
			// JPY has 0 decimals: a non-empty fractional part with more
			// digits than allowed (2 > 0) must be rejected, not silently
			// accepted with the ".00" thrown away.
			if err == nil {
				t.Errorf("majorStringToMinor(%q, %q) = %d, want error (fractional digits exceed JPY's 0-decimal exponent)", c.s, c.currency, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("majorStringToMinor(%q, %q) error: %v", c.s, c.currency, err)
			continue
		}
		if got != c.want {
			t.Errorf("majorStringToMinor(%q, %q) = %d, want %d", c.s, c.currency, got, c.want)
		}
	}
}

func TestMajorStringToMinor_FailsClosed(t *testing.T) {
	bad := []struct{ s, currency string }{
		{"", "USD"},
		{"-1.00", "USD"},
		{"abc", "USD"},
		{"1.2.3", "USD"},
		{"1.005", "USD"},  // 3 fractional digits, USD only allows 2
		{"1.0000", "KWD"}, // 4 fractional digits, KWD only allows 3
		{"1.x", "USD"},
	}
	for _, c := range bad {
		if _, err := majorStringToMinor(c.s, c.currency); err == nil {
			t.Errorf("majorStringToMinor(%q, %q) = nil error, want error", c.s, c.currency)
		}
	}
}

func TestMinorMajorRoundTrip(t *testing.T) {
	currencies := []string{"ZAR", "USD", "JPY", "KWD"}
	amounts := []int64{0, 1, 5, 99, 100, 1000, 123456}
	for _, cur := range currencies {
		for _, amt := range amounts {
			s, err := minorToMajorString(amt, cur)
			if err != nil {
				t.Fatalf("minorToMajorString(%d, %s): %v", amt, cur, err)
			}
			back, err := majorStringToMinor(s, cur)
			if err != nil {
				t.Fatalf("majorStringToMinor(%q, %s): %v", s, cur, err)
			}
			if back != amt {
				t.Errorf("round trip %d %s -> %q -> %d, want %d", amt, cur, s, back, amt)
			}
		}
	}
}
