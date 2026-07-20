// Package money is Cackle's currency and amount model. It exists because
// "integer cents" — which the rest of the codebase historically assumed —
// is not a universal truth: ISO 4217 currencies do not all have two decimal
// places. JPY, KRW, VND and CLP have zero; KWD, BHD, JOD, OMR and TND have
// three. Dividing a JPY amount by 100 to display it is a bug, not a
// rounding nicety, and multiplying a KWD amount assuming "cents" silently
// throws away (or fabricates) a decimal digit.
//
// Every money value in Cackle should be an Amount: an integer minor-unit
// count plus the ISO-4217 currency it is denominated in. Floats are never
// used for money anywhere in this package or (per the payments contract)
// anywhere else in Cackle.
package money

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Callers should match with errors.Is.
var (
	// ErrInvalidCurrency means the input isn't shaped like an ISO-4217
	// alpha-3 code at all (wrong length, non-letters) — a syntax error,
	// distinct from ErrUnknownCurrency below.
	ErrInvalidCurrency = errors.New("money: invalid currency code")
	// ErrUnknownCurrency means the input is syntactically a 3-letter code
	// but isn't in this package's currency table.
	ErrUnknownCurrency = errors.New("money: unknown currency code")
	// ErrCurrencyMismatch is returned by Amount arithmetic when the two
	// operands are denominated in different currencies — Cackle never
	// silently converts between currencies (that requires an FX rate and
	// a decision about which rate/timestamp to trust; see the crypto
	// adapters' pricing rules in the payments contract).
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	// ErrOverflow is returned by Amount arithmetic that would overflow
	// int64. Money arithmetic never wraps silently.
	ErrOverflow = errors.New("money: arithmetic overflow")
	// ErrInvalidAmount is returned by parsing helpers given a
	// non-numeric, empty, or over-precise decimal string.
	ErrInvalidAmount = errors.New("money: invalid amount")
)

// CurrencyInfo describes one ISO-4217 currency: how many digits follow the
// decimal point in its major-unit display (Exponent), and its English
// name.
type CurrencyInfo struct {
	Code     string
	Exponent int
	Name     string
}

// currencyDefs is the ISO-4217 currency table. Coverage: every currency
// named anywhere in the payments contract's adapter list (global cards,
// African/LATAM/South Asian/Southeast Asian regional processors, Middle
// Eastern gateways) plus the standard set of 0- and 3-decimal exceptions,
// plus enough of the rest of ISO-4217 that an org picking an uncommon
// default currency doesn't immediately hit ErrUnknownCurrency.
//
// Zero-decimal currencies (exponent 0): BIF, CLP, DJF, GNF, ISK, JPY, KMF,
// KRW, PYG, RWF, UGX, VND, VUV, XAF, XOF, XPF.
// Three-decimal currencies (exponent 3): BHD, IQD, JOD, KWD, LYD, OMR, TND.
// Everything else defaults to 2.
var currencyDefs = []CurrencyInfo{
	{"AED", 2, "United Arab Emirates Dirham"},
	{"AFN", 2, "Afghan Afghani"},
	{"ALL", 2, "Albanian Lek"},
	{"AMD", 2, "Armenian Dram"},
	{"ANG", 2, "Netherlands Antillean Guilder"},
	{"AOA", 2, "Angolan Kwanza"},
	{"ARS", 2, "Argentine Peso"},
	{"AUD", 2, "Australian Dollar"},
	{"AWG", 2, "Aruban Florin"},
	{"AZN", 2, "Azerbaijani Manat"},
	{"BAM", 2, "Bosnia-Herzegovina Convertible Mark"},
	{"BBD", 2, "Barbadian Dollar"},
	{"BDT", 2, "Bangladeshi Taka"},
	{"BGN", 2, "Bulgarian Lev"},
	{"BHD", 3, "Bahraini Dinar"},
	{"BIF", 0, "Burundian Franc"},
	{"BMD", 2, "Bermudian Dollar"},
	{"BND", 2, "Brunei Dollar"},
	{"BOB", 2, "Bolivian Boliviano"},
	{"BRL", 2, "Brazilian Real"},
	{"BSD", 2, "Bahamian Dollar"},
	{"BTN", 2, "Bhutanese Ngultrum"},
	{"BWP", 2, "Botswana Pula"},
	{"BYN", 2, "Belarusian Ruble"},
	{"BZD", 2, "Belize Dollar"},
	{"CAD", 2, "Canadian Dollar"},
	{"CDF", 2, "Congolese Franc"},
	{"CHF", 2, "Swiss Franc"},
	{"CLP", 0, "Chilean Peso"},
	{"CNY", 2, "Chinese Yuan"},
	{"COP", 2, "Colombian Peso"},
	{"CRC", 2, "Costa Rican Colon"},
	{"CUP", 2, "Cuban Peso"},
	{"CVE", 2, "Cape Verdean Escudo"},
	{"CZK", 2, "Czech Koruna"},
	{"DJF", 0, "Djiboutian Franc"},
	{"DKK", 2, "Danish Krone"},
	{"DOP", 2, "Dominican Peso"},
	{"DZD", 2, "Algerian Dinar"},
	{"EGP", 2, "Egyptian Pound"},
	{"ETB", 2, "Ethiopian Birr"},
	{"EUR", 2, "Euro"},
	{"FJD", 2, "Fijian Dollar"},
	{"GBP", 2, "British Pound Sterling"},
	{"GEL", 2, "Georgian Lari"},
	{"GHS", 2, "Ghanaian Cedi"},
	{"GMD", 2, "Gambian Dalasi"},
	{"GNF", 0, "Guinean Franc"},
	{"GTQ", 2, "Guatemalan Quetzal"},
	{"GYD", 2, "Guyanese Dollar"},
	{"HKD", 2, "Hong Kong Dollar"},
	{"HNL", 2, "Honduran Lempira"},
	{"HTG", 2, "Haitian Gourde"},
	{"HUF", 2, "Hungarian Forint"},
	{"IDR", 2, "Indonesian Rupiah"},
	{"ILS", 2, "Israeli New Shekel"},
	{"INR", 2, "Indian Rupee"},
	{"IQD", 3, "Iraqi Dinar"},
	{"IRR", 2, "Iranian Rial"},
	{"ISK", 0, "Icelandic Krona"},
	{"JMD", 2, "Jamaican Dollar"},
	{"JOD", 3, "Jordanian Dinar"},
	{"JPY", 0, "Japanese Yen"},
	{"KES", 2, "Kenyan Shilling"},
	{"KGS", 2, "Kyrgyzstani Som"},
	{"KHR", 2, "Cambodian Riel"},
	{"KMF", 0, "Comorian Franc"},
	{"KRW", 0, "South Korean Won"},
	{"KWD", 3, "Kuwaiti Dinar"},
	{"KYD", 2, "Cayman Islands Dollar"},
	{"KZT", 2, "Kazakhstani Tenge"},
	{"LAK", 2, "Lao Kip"},
	{"LBP", 2, "Lebanese Pound"},
	{"LKR", 2, "Sri Lankan Rupee"},
	{"LRD", 2, "Liberian Dollar"},
	{"LSL", 2, "Lesotho Loti"},
	{"LYD", 3, "Libyan Dinar"},
	{"MAD", 2, "Moroccan Dirham"},
	{"MDL", 2, "Moldovan Leu"},
	{"MGA", 2, "Malagasy Ariary"},
	{"MKD", 2, "Macedonian Denar"},
	{"MMK", 2, "Myanmar Kyat"},
	{"MNT", 2, "Mongolian Tugrik"},
	{"MOP", 2, "Macanese Pataca"},
	{"MRU", 2, "Mauritanian Ouguiya"},
	{"MUR", 2, "Mauritian Rupee"},
	{"MVR", 2, "Maldivian Rufiyaa"},
	{"MWK", 2, "Malawian Kwacha"},
	{"MXN", 2, "Mexican Peso"},
	{"MYR", 2, "Malaysian Ringgit"},
	{"MZN", 2, "Mozambican Metical"},
	{"NAD", 2, "Namibian Dollar"},
	{"NGN", 2, "Nigerian Naira"},
	{"NIO", 2, "Nicaraguan Cordoba"},
	{"NOK", 2, "Norwegian Krone"},
	{"NPR", 2, "Nepalese Rupee"},
	{"NZD", 2, "New Zealand Dollar"},
	{"OMR", 3, "Omani Rial"},
	{"PAB", 2, "Panamanian Balboa"},
	{"PEN", 2, "Peruvian Sol"},
	{"PGK", 2, "Papua New Guinean Kina"},
	{"PHP", 2, "Philippine Peso"},
	{"PKR", 2, "Pakistani Rupee"},
	{"PLN", 2, "Polish Zloty"},
	{"PYG", 0, "Paraguayan Guarani"},
	{"QAR", 2, "Qatari Riyal"},
	{"RON", 2, "Romanian Leu"},
	{"RSD", 2, "Serbian Dinar"},
	{"RUB", 2, "Russian Ruble"},
	{"RWF", 0, "Rwandan Franc"},
	{"SAR", 2, "Saudi Riyal"},
	{"SBD", 2, "Solomon Islands Dollar"},
	{"SCR", 2, "Seychellois Rupee"},
	{"SDG", 2, "Sudanese Pound"},
	{"SEK", 2, "Swedish Krona"},
	{"SGD", 2, "Singapore Dollar"},
	{"SLE", 2, "Sierra Leonean Leone"},
	{"SOS", 2, "Somali Shilling"},
	{"SRD", 2, "Surinamese Dollar"},
	{"SSP", 2, "South Sudanese Pound"},
	{"STN", 2, "Sao Tome and Principe Dobra"},
	{"SZL", 2, "Eswatini Lilangeni"},
	{"THB", 2, "Thai Baht"},
	{"TJS", 2, "Tajikistani Somoni"},
	{"TMT", 2, "Turkmenistani Manat"},
	{"TND", 3, "Tunisian Dinar"},
	{"TOP", 2, "Tongan Pa'anga"},
	{"TRY", 2, "Turkish Lira"},
	{"TTD", 2, "Trinidad and Tobago Dollar"},
	{"TWD", 2, "New Taiwan Dollar"},
	{"TZS", 2, "Tanzanian Shilling"},
	{"UAH", 2, "Ukrainian Hryvnia"},
	{"UGX", 0, "Ugandan Shilling"},
	{"USD", 2, "United States Dollar"},
	{"UYU", 2, "Uruguayan Peso"},
	{"UZS", 2, "Uzbekistani Som"},
	{"VES", 2, "Venezuelan Bolivar Soberano"},
	{"VND", 0, "Vietnamese Dong"},
	{"VUV", 0, "Vanuatu Vatu"},
	{"WST", 2, "Samoan Tala"},
	{"XAF", 0, "Central African CFA Franc"},
	{"XCD", 2, "East Caribbean Dollar"},
	{"XOF", 0, "West African CFA Franc"},
	{"XPF", 0, "CFP Franc"},
	{"YER", 2, "Yemeni Rial"},
	{"ZAR", 2, "South African Rand"},
	{"ZMW", 2, "Zambian Kwacha"},
}

// currencyTable is currencyDefs indexed by code, built once at init.
var currencyTable = func() map[string]CurrencyInfo {
	m := make(map[string]CurrencyInfo, len(currencyDefs))
	for _, c := range currencyDefs {
		m[c.Code] = c
	}
	return m
}()

// normalize upper-cases and trims code, and rejects anything that isn't
// exactly 3 ASCII letters, BEFORE the table lookup — this is the syntactic
// check (ErrInvalidCurrency), distinct from "not in our table"
// (ErrUnknownCurrency).
func normalizeCode(code string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if len(c) != 3 {
		return "", fmt.Errorf("%w: %q (must be a 3-letter ISO-4217 code)", ErrInvalidCurrency, code)
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return "", fmt.Errorf("%w: %q (must be alphabetic)", ErrInvalidCurrency, code)
		}
	}
	return c, nil
}

// Lookup normalizes code (uppercasing, trimming whitespace) and returns its
// CurrencyInfo, or an error: ErrInvalidCurrency if it isn't shaped like a
// currency code at all, ErrUnknownCurrency if it is well-formed but not in
// this package's table.
func Lookup(code string) (CurrencyInfo, error) {
	norm, err := normalizeCode(code)
	if err != nil {
		return CurrencyInfo{}, err
	}
	info, ok := currencyTable[norm]
	if !ok {
		return CurrencyInfo{}, fmt.Errorf("%w: %q", ErrUnknownCurrency, norm)
	}
	return info, nil
}

// Exponent returns how many digits follow the decimal point when code is
// displayed in its major unit (0 for JPY, 3 for KWD, 2 for most others).
// Never assume 2 — always call this.
func Exponent(code string) (int, error) {
	info, err := Lookup(code)
	if err != nil {
		return 0, err
	}
	return info.Exponent, nil
}

// Validate reports whether code is a normalizable, known ISO-4217 currency.
// It accepts lowercase input (money.Validate("usd") == nil) since the
// normalization itself is not an error — only an unknown/malformed code is.
func Validate(code string) error {
	_, err := Lookup(code)
	return err
}

// Normalize returns code upper-cased and validated, for callers that want
// the canonical form to store/compare (e.g. before persisting
// events.currency).
func Normalize(code string) (string, error) {
	info, err := Lookup(code)
	if err != nil {
		return "", err
	}
	return info.Code, nil
}

// Name returns the English display name of code, e.g. "Japanese Yen".
func Name(code string) (string, error) {
	info, err := Lookup(code)
	if err != nil {
		return "", err
	}
	return info.Name, nil
}

// SupportedCurrencies returns every ISO-4217 code this package knows about,
// in table order (not sorted — callers that want a sorted list should sort
// it themselves).
func SupportedCurrencies() []string {
	codes := make([]string, len(currencyDefs))
	for i, c := range currencyDefs {
		codes[i] = c.Code
	}
	return codes
}
