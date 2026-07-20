// Package payments: PayU adapter — specifically PayU India ("PayU Biz"),
// NOT PayU LatAm or PayU Global, which are different products with
// different APIs this file does not attempt to cover.
//
// Reference: https://docs.payu.in/docs/hash-generation (request/response
// hash sequences) and https://docs.payu.in/reference/verify-payment-api
// (server-to-server Verify Payment API).
//
// Confidence: MEDIUM. The hash sequence below —
//
//	request:  sha512(key|txnid|amount|productinfo|firstname|email|udf1|udf2|udf3|udf4|udf5||||||SALT)
//	response: sha512(SALT|status|udf5|udf4|udf3|udf2|udf1|email|firstname|productinfo|amount|txnid|key)
//
// — is consistent across PayU India's own documentation and every
// third-party integration guide referencing it, so this file implements
// it with real confidence. The Verify Payment API's exact request/response
// field names are implemented from documentation but are less certain.
// This has NOT been run against a real PayU India test account.
//
// IMPORTANT caveat on Begin: PayU India's hosted checkout is an HTML form
// POST (action https://secure.payu.in/_payment), not a bare clickable
// redirect link — same caveat as this package's PayFast adapter.
// Charge.RedirectURL is the form action URL; Charge.Instructions carries
// the full field set (including the request hash) as a URL-encoded query
// string for the caller to render as a hidden auto-submitting form.
//
// Amounts are decimal strings in MAJOR units (e.g. "100.00" for ₹100),
// via currency.go's minorToMajorString/majorStringToMinor.
package payments

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProviderNamePayU is the stable Name() this provider registers under.
const ProviderNamePayU = "payu"

const (
	EnvPayUMerchantKey  = "CACKLE_PAYU_MERCHANT_KEY"
	EnvPayUSalt         = "CACKLE_PAYU_SALT"
	payUCheckoutURL     = "https://secure.payu.in/_payment"
	payUVerifyURL       = "https://info.payu.in/merchant/postservice?form=2"
	payUHTTPTimeout     = 15 * time.Second
	payUMaxResponseSize = 1 << 20
)

var (
	ErrPayUCredentialsNotConfigured = errors.New("payments: payu: " + EnvPayUMerchantKey + " and " + EnvPayUSalt + " must both be set")
	ErrPayUMissingHash              = errors.New("payments: payu: missing hash field")
	ErrPayUInvalidHash              = errors.New("payments: payu: invalid response hash")
	ErrPayUMalformedResponse        = errors.New("payments: payu: malformed API response")
	ErrPayUUnexpectedStatus         = errors.New("payments: payu: unexpected API response status")
	ErrPayUResponseTooLarge         = errors.New("payments: payu: response body exceeds size limit")
	ErrPayUTransactionNotFound      = errors.New("payments: payu: transaction not found for txnid")
)

// PayUProvider implements Provider against PayU India's hosted checkout
// (hash-based form POST) and Verify Payment API.
type PayUProvider struct {
	merchantKey string
	salt        string
	httpClient  *http.Client
	checkoutURL string
	verifyURL   string
}

// NewPayU constructs a PayUProvider from EnvPayUMerchantKey and EnvPayUSalt.
func NewPayU() (*PayUProvider, error) {
	key := strings.TrimSpace(os.Getenv(EnvPayUMerchantKey))
	salt := strings.TrimSpace(os.Getenv(EnvPayUSalt))
	if key == "" || salt == "" {
		return nil, ErrPayUCredentialsNotConfigured
	}
	return &PayUProvider{
		merchantKey: key,
		salt:        salt,
		httpClient:  &http.Client{Timeout: payUHTTPTimeout},
		checkoutURL: payUCheckoutURL,
		verifyURL:   payUVerifyURL,
	}, nil
}

// Name implements Provider.
func (p *PayUProvider) Name() string { return ProviderNamePayU }

// Capabilities implements Provider.
func (p *PayUProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"INR"},
		Countries:     []string{"IN"},
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // INR only; 2-decimal, never exercised against 0/3-decimal currencies
	}
}

// requestHash computes PayU's request hash sequence:
// key|txnid|amount|productinfo|firstname|email|udf1|udf2|udf3|udf4|udf5||||||SALT
func (p *PayUProvider) requestHash(txnid, amount, productinfo, firstname, email string, udf [5]string) string {
	parts := []string{p.merchantKey, txnid, amount, productinfo, firstname, email,
		udf[0], udf[1], udf[2], udf[3], udf[4], "", "", "", "", "", p.salt}
	sum := sha512.Sum512([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// responseHash computes PayU's reverse (response verification) hash
// sequence: SALT|status|udf5|udf4|udf3|udf2|udf1|email|firstname|productinfo|amount|txnid|key
func (p *PayUProvider) responseHash(status, txnid, amount, productinfo, firstname, email string, udf [5]string) string {
	parts := []string{p.salt, status, udf[4], udf[3], udf[2], udf[1], udf[0],
		email, firstname, productinfo, amount, txnid, p.merchantKey}
	sum := sha512.Sum512([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// Begin builds the parameter set and request hash for PayU's hosted
// checkout form. See the file header for why this returns a form action
// URL plus a field set rather than a bare clickable link.
func (p *PayUProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: payu: order reference is required as txnid")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: payu: amount_minor must be positive")
	}
	if !strings.EqualFold(o.Currency, "INR") {
		return Charge{}, fmt.Errorf("payments: payu: only INR is supported, got %q", o.Currency)
	}
	if strings.TrimSpace(o.BuyerEmail) == "" {
		return Charge{}, errors.New("payments: payu: buyer email is required")
	}
	amountStr, err := minorToMajorString(o.AmountMinor, "INR")
	if err != nil {
		return Charge{}, fmt.Errorf("payments: payu: %w", err)
	}
	productinfo := "Order " + o.Reference
	firstname := o.BuyerName
	if firstname == "" {
		firstname = "Customer"
	}
	var udf [5]string
	hash := p.requestHash(o.Reference, amountStr, productinfo, firstname, o.BuyerEmail, udf)

	fields := url.Values{}
	fields.Set("key", p.merchantKey)
	fields.Set("txnid", o.Reference)
	fields.Set("amount", amountStr)
	fields.Set("productinfo", productinfo)
	fields.Set("firstname", firstname)
	fields.Set("email", o.BuyerEmail)
	fields.Set("hash", hash)
	if o.CallbackURL != "" {
		fields.Set("surl", o.CallbackURL)
		fields.Set("furl", o.CallbackURL)
	}

	return Charge{
		Provider:     ProviderNamePayU,
		Reference:    o.Reference,
		RedirectURL:  p.checkoutURL,
		Instructions: fields.Encode(),
	}, nil
}

// Webhook validates PayU's response hash on the surl/furl callback POST
// (application/x-www-form-urlencoded) and returns the settled result.
func (p *PayUProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrPayUMalformedResponse)
	}
	body, err := boundedRead(r.Body, payUMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrPayUResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: payu: read webhook body: %w", err)
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrPayUMalformedResponse, err)
	}
	given := values.Get("hash")
	if given == "" {
		return Result{}, ErrPayUMissingHash
	}
	status := values.Get("status")
	txnid := values.Get("txnid")
	amount := values.Get("amount")
	productinfo := values.Get("productinfo")
	firstname := values.Get("firstname")
	email := values.Get("email")
	mihpayid := values.Get("mihpayid")
	var udf [5]string
	for i := range udf {
		udf[i] = values.Get(fmt.Sprintf("udf%d", i+1))
	}
	if txnid == "" || amount == "" {
		return Result{}, fmt.Errorf("%w: missing txnid or amount", ErrPayUMalformedResponse)
	}

	expected := p.responseHash(status, txnid, amount, productinfo, firstname, email, udf)
	if !constantTimeEqualString(expected, given) {
		return Result{}, ErrPayUInvalidHash
	}

	amountMinor, err := majorStringToMinor(amount, "INR")
	if err != nil {
		return Result{}, fmt.Errorf("%w: amount %q: %v", ErrPayUMalformedResponse, amount, err)
	}
	result := Result{
		Provider:    ProviderNamePayU,
		Reference:   txnid,
		EventID:     mihpayid,
		AmountMinor: amountMinor,
		Currency:    "INR",
		Raw:         rawJSONFromForm(body),
	}
	switch status {
	case "success":
		if amountMinor <= 0 || mihpayid == "" {
			return Result{}, fmt.Errorf("%w: success status with non-positive amount or no mihpayid", ErrPayUMalformedResponse)
		}
		result.Status = StatusPaid
	case "failure", "failed":
		result.Status = StatusFailed
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

// constantTimeEqualString compares two strings in constant time.
func constantTimeEqualString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

type payUVerifyTransactionDetail struct {
	MihpayID string `json:"mihpayid"`
	Status   string `json:"status"`
	TxnID    string `json:"txnid"`
	Amount   string `json:"amt"`
	AddedOn  string `json:"addedon"`
}

// Verify calls PayU's server-to-server Verify Payment API:
// command=verify_payment, var1=txnid, hash=sha512(key|verify_payment|txnid|SALT).
func (p *PayUProvider) Verify(ctx context.Context, reference string) (Result, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return Result{}, errors.New("payments: payu: reference is required")
	}
	hashParts := []string{p.merchantKey, "verify_payment", reference, p.salt}
	sum := sha512.Sum512([]byte(strings.Join(hashParts, "|")))
	hash := hex.EncodeToString(sum[:])

	form := url.Values{}
	form.Set("key", p.merchantKey)
	form.Set("command", "verify_payment")
	form.Set("var1", reference)
	form.Set("hash", hash)

	respBody, status, err := p.do(ctx, form)
	if err != nil {
		return Result{}, err
	}
	if status < 200 || status >= 300 {
		return Result{}, fmt.Errorf("%w: http %d", ErrPayUUnexpectedStatus, status)
	}
	var parsed struct {
		Status             int                                    `json:"status"`
		TransactionDetails map[string]payUVerifyTransactionDetail `json:"transaction_details"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrPayUMalformedResponse, err)
	}
	detail, ok := parsed.TransactionDetails[reference]
	if !ok {
		return Result{}, ErrPayUTransactionNotFound
	}
	if detail.TxnID != "" && detail.TxnID != reference {
		return Result{}, fmt.Errorf("%w: returned txnid %q for requested %q", ErrPayUMalformedResponse, detail.TxnID, reference)
	}
	amountMinor, err := majorStringToMinor(detail.Amount, "INR")
	if err != nil {
		return Result{}, fmt.Errorf("%w: amt %q: %v", ErrPayUMalformedResponse, detail.Amount, err)
	}
	result := Result{
		Provider:    ProviderNamePayU,
		Reference:   reference,
		EventID:     detail.MihpayID,
		AmountMinor: amountMinor,
		Currency:    "INR",
		Raw:         json.RawMessage(respBody),
	}
	switch strings.ToLower(detail.Status) {
	case "success":
		if amountMinor <= 0 || detail.MihpayID == "" {
			return Result{}, fmt.Errorf("%w: success with non-positive amount or no mihpayid", ErrPayUMalformedResponse)
		}
		result.Status = StatusPaid
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

func (p *PayUProvider) do(ctx context.Context, form url.Values) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, payUHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.verifyURL, bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		return nil, 0, fmt.Errorf("payments: payu: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: payu: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, payUMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, resp.StatusCode, ErrPayUResponseTooLarge
		}
		return nil, resp.StatusCode, fmt.Errorf("payments: payu: read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}
