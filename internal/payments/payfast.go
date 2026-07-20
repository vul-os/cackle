// Package payments: PayFast adapter (South Africa).
//
// Reference: https://developers.payfast.co.za/docs (Onsite/Redirect
// payment flow + signature generation) and
// https://developers.payfast.co.za/docs#step_5_confirm_payment (ITN —
// Instant Transaction Notification: signature check + server-to-server
// "validate" confirmation round trip).
//
// Confidence: HIGH on the ITN verification model (signature + mandatory
// validate-callback), which is PayFast's most consistently documented
// flow and the security-critical part of this adapter. MEDIUM on Begin's
// exact request shape: PayFast's canonical integration method is an HTML
// form auto-POSTed to their process endpoint, not a bare clickable
// redirect link — see the caveat on Charge.RedirectURL below. This has
// not been run against a real PayFast sandbox account.
//
// PayFast is ZAR-only. Amounts are decimal strings in major units (e.g.
// "100.00" for R100), a straightforward 2-decimal conversion since ZAR
// has no zero/three-decimal ambiguity.
//
// PayFast's own documented anti-fraud checklist for a webhook (ITN) has
// THREE steps: (1) verify the signature, (2) verify the source IP is one
// of PayFast's published ranges, (3) POST the notification data back to
// PayFast's own "validate" endpoint and require the literal response body
// "VALID". This file implements (1) and (3) — (3) is arguably the
// strongest of the three since it round-trips through PayFast itself —
// but does NOT implement (2) (source IP allowlisting), since that
// requires deployment-specific network configuration this package has no
// way to see; callers deploying this in production should add IP
// filtering at their ingress/load balancer as PayFast recommends.
package payments

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProviderNamePayFast is the stable Name() this provider registers under.
const ProviderNamePayFast = "payfast"

const (
	EnvPayFastMerchantID   = "CACKLE_PAYFAST_MERCHANT_ID"
	EnvPayFastMerchantKey  = "CACKLE_PAYFAST_MERCHANT_KEY"
	EnvPayFastPassphrase   = "CACKLE_PAYFAST_PASSPHRASE" // optional but strongly recommended by PayFast
	payFastProcessURL      = "https://www.payfast.co.za/eng/process"
	payFastValidateURL     = "https://www.payfast.co.za/eng/query/validate"
	payFastHTTPTimeout     = 15 * time.Second
	payFastMaxResponseSize = 1 << 20
)

var (
	ErrPayFastCredentialsNotConfigured = errors.New("payments: payfast: " + EnvPayFastMerchantID + " and " + EnvPayFastMerchantKey + " must both be set")
	ErrPayFastUnsupportedCurrency      = errors.New("payments: payfast: only ZAR is supported")
	ErrPayFastMissingSignature         = errors.New("payments: payfast: missing signature field")
	ErrPayFastInvalidSignature         = errors.New("payments: payfast: invalid signature")
	ErrPayFastNotValidated             = errors.New("payments: payfast: server-side validate confirmation did not return VALID")
	ErrPayFastMalformedNotification    = errors.New("payments: payfast: malformed ITN payload")
	ErrPayFastResponseTooLarge         = errors.New("payments: payfast: request body exceeds size limit")
	ErrPayFastUnexpectedStatus         = errors.New("payments: payfast: unexpected API response status")
)

// PayFastProvider implements Provider against PayFast's Onsite/Redirect
// payment flow and ITN webhook.
type PayFastProvider struct {
	merchantID  string
	merchantKey string
	passphrase  string // optional
	httpClient  *http.Client
	processURL  string
	validateURL string
}

// NewPayFast constructs a PayFastProvider from EnvPayFastMerchantID,
// EnvPayFastMerchantKey, and the optional EnvPayFastPassphrase.
func NewPayFast() (*PayFastProvider, error) {
	merchantID := strings.TrimSpace(os.Getenv(EnvPayFastMerchantID))
	merchantKey := strings.TrimSpace(os.Getenv(EnvPayFastMerchantKey))
	if merchantID == "" || merchantKey == "" {
		return nil, ErrPayFastCredentialsNotConfigured
	}
	return &PayFastProvider{
		merchantID:  merchantID,
		merchantKey: merchantKey,
		passphrase:  strings.TrimSpace(os.Getenv(EnvPayFastPassphrase)),
		httpClient:  &http.Client{Timeout: payFastHTTPTimeout},
		processURL:  payFastProcessURL,
		validateURL: payFastValidateURL,
	}, nil
}

// Name implements Provider.
func (p *PayFastProvider) Name() string { return ProviderNamePayFast }

// Capabilities implements Provider.
func (p *PayFastProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{"ZAR"},
		Countries:     []string{"ZA"},
		Flow:          FlowRedirect,
		Refunds:       false,
		Payouts:       false,
		Webhooks:      true,
		ZeroDecimalOK: false, // ZAR only
	}
}

// payFastSignature computes PayFast's MD5 signature over fields, in the
// exact order given (PayFast signs the parameter string in the order
// fields were assembled, NOT alphabetically), url-encoding each value the
// same way PayFast's own examples do (spaces as '+'), and appending the
// passphrase if one is configured. The "signature" key itself must never
// be included in fields.
func (p *PayFastProvider) payFastSignature(fields []payFastKV) string {
	var b strings.Builder
	for i, kv := range fields {
		if kv.value == "" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString(kv.key)
		b.WriteByte('=')
		b.WriteString(strings.ReplaceAll(url.QueryEscape(kv.value), "%20", "+"))
	}
	if p.passphrase != "" {
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		b.WriteString("passphrase=")
		b.WriteString(strings.ReplaceAll(url.QueryEscape(p.passphrase), "%20", "+"))
	}
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

type payFastKV struct{ key, value string }

// Begin builds the parameter set and signature for PayFast's Onsite
// payment flow.
//
// IMPORTANT: PayFast's canonical integration is an HTML form auto-POSTed
// to RedirectURL (all the fields below, plus "signature"), not a bare
// clickable GET link. Charge.RedirectURL here is the process endpoint;
// Charge.Instructions carries the full field set as a URL-encoded query
// string for the caller to render as a hidden auto-submitting form. A
// caller that only redirects the browser to RedirectURL with no form data
// will NOT work — this is flagged rather than guessed at further because
// this file has not been verified against a live PayFast checkout.
func (p *PayFastProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: payfast: order reference is required as m_payment_id")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: payfast: amount_minor must be positive")
	}
	if !strings.EqualFold(o.Currency, "ZAR") {
		return Charge{}, fmt.Errorf("%w: got %q", ErrPayFastUnsupportedCurrency, o.Currency)
	}
	amountStr, err := minorToMajorString(o.AmountMinor, "ZAR")
	if err != nil {
		return Charge{}, fmt.Errorf("payments: payfast: %w", err)
	}

	fields := []payFastKV{
		{"merchant_id", p.merchantID},
		{"merchant_key", p.merchantKey},
		{"return_url", o.CallbackURL},
		{"cancel_url", o.CallbackURL},
		{"notify_url", ""}, // caller's webhook route; filled in by the caller's own wiring, not this package
		{"m_payment_id", o.Reference},
		{"amount", amountStr},
		{"item_name", "Order " + o.Reference},
	}
	sig := p.payFastSignature(fields)

	var qs strings.Builder
	for i, kv := range fields {
		if kv.value == "" {
			continue
		}
		if i > 0 && qs.Len() > 0 {
			qs.WriteByte('&')
		}
		qs.WriteString(kv.key)
		qs.WriteByte('=')
		qs.WriteString(url.QueryEscape(kv.value))
	}
	qs.WriteString("&signature=")
	qs.WriteString(sig)

	return Charge{
		Provider:     ProviderNamePayFast,
		Reference:    o.Reference,
		RedirectURL:  p.processURL,
		Instructions: qs.String(),
	}, nil
}

// payFastNotification is the ITN payload PayFast POSTs as
// application/x-www-form-urlencoded.
type payFastNotification struct {
	values       url.Values
	rawSignature string
}

func parsePayFastNotification(body []byte) (payFastNotification, error) {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return payFastNotification{}, fmt.Errorf("%w: %v", ErrPayFastMalformedNotification, err)
	}
	sig := values.Get("signature")
	return payFastNotification{values: values, rawSignature: sig}, nil
}

// verifySignature recomputes PayFast's signature over every field EXCEPT
// "signature" itself, in the order they appear in the raw body (POST
// field order is preserved by url.ParseQuery only insofar as Go retains
// insertion order in the underlying query string parse — to be safe this
// reconstructs the order from the raw body directly instead of trusting
// map iteration order).
func (p *PayFastProvider) verifySignature(rawBody []byte, given string) bool {
	if given == "" {
		return false
	}
	var fields []payFastKV
	for _, pair := range strings.Split(string(rawBody), "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		if k == "signature" {
			continue
		}
		decodedKey, err1 := url.QueryUnescape(k)
		decodedVal, err2 := url.QueryUnescape(v)
		if err1 != nil || err2 != nil {
			continue
		}
		fields = append(fields, payFastKV{decodedKey, decodedVal})
	}
	expected := p.payFastSignature(fields)
	return hmacEqualHex(expected, given)
}

// hmacEqualHex compares two hex-encoded digests in constant time.
func hmacEqualHex(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// Webhook validates a PayFast ITN: (1) recomputes and compares the
// signature, then (2) round-trips the same raw payload to PayFast's own
// "validate" endpoint and requires the literal response "VALID" before
// trusting anything in the notification. Both steps fail closed.
func (p *PayFastProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	if r.Body == nil {
		return Result{}, fmt.Errorf("%w: empty request body", ErrPayFastMalformedNotification)
	}
	body, err := boundedRead(r.Body, payFastMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return Result{}, ErrPayFastResponseTooLarge
		}
		return Result{}, fmt.Errorf("payments: payfast: read webhook body: %w", err)
	}
	notification, err := parsePayFastNotification(body)
	if err != nil {
		return Result{}, err
	}
	if notification.rawSignature == "" {
		return Result{}, ErrPayFastMissingSignature
	}
	if !p.verifySignature(body, notification.rawSignature) {
		return Result{}, ErrPayFastInvalidSignature
	}

	if err := p.confirmWithPayFast(ctx, body); err != nil {
		return Result{}, err
	}

	reference := notification.values.Get("m_payment_id")
	paymentStatus := notification.values.Get("payment_status")
	amountStr := notification.values.Get("amount_gross")
	pfPaymentID := notification.values.Get("pf_payment_id")
	if reference == "" || amountStr == "" {
		return Result{}, fmt.Errorf("%w: missing m_payment_id or amount_gross", ErrPayFastMalformedNotification)
	}
	amountMinor, err := majorStringToMinor(amountStr, "ZAR")
	if err != nil {
		return Result{}, fmt.Errorf("%w: amount_gross %q: %v", ErrPayFastMalformedNotification, amountStr, err)
	}

	result := Result{
		Provider:    ProviderNamePayFast,
		Reference:   reference,
		EventID:     pfPaymentID,
		AmountMinor: amountMinor,
		Currency:    "ZAR",
		Raw:         rawJSONFromForm(body),
	}
	switch paymentStatus {
	case "COMPLETE":
		if amountMinor <= 0 || pfPaymentID == "" {
			return Result{}, fmt.Errorf("%w: COMPLETE status with non-positive amount or no pf_payment_id", ErrPayFastMalformedNotification)
		}
		result.Status = StatusPaid
	case "FAILED", "CANCELLED":
		result.Status = StatusFailed
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

// confirmWithPayFast performs step 3 of PayFast's own documented ITN
// checklist: POST the exact same raw payload back to PayFast's validate
// endpoint and require the literal response body "VALID". This is done
// over the raw form body (not re-encoded) so it is byte-identical to what
// PayFast sent.
func (p *PayFastProvider) confirmWithPayFast(ctx context.Context, rawBody []byte) error {
	ctx, cancel := context.WithTimeout(ctx, payFastHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.validateURL, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("payments: payfast: build validate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("payments: payfast: validate request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, payFastMaxResponseSize)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return ErrPayFastResponseTooLarge
		}
		return fmt.Errorf("payments: payfast: read validate response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: validate endpoint returned http %d", ErrPayFastUnexpectedStatus, resp.StatusCode)
	}
	if strings.TrimSpace(string(respBody)) != "VALID" {
		return ErrPayFastNotValidated
	}
	return nil
}

// rawJSONFromForm stores the raw ITN form body as-is for audit purposes
// (Result.Raw is typed json.RawMessage, but nothing requires it to be
// semantically JSON — callers treat it as an opaque audit blob).
func rawJSONFromForm(body []byte) []byte {
	// Wrap as a JSON string literal so callers that do try to
	// json.Unmarshal Result.Raw elsewhere get a valid (if unhelpful)
	// JSON value instead of a parse error.
	escaped := strings.ReplaceAll(string(body), `"`, `\"`)
	return []byte(`"` + escaped + `"`)
}

// Verify is not implemented: PayFast has no documented "fetch a
// transaction by reference" polling endpoint the way Paystack/Xendit do
// — its model is push-only ITN plus the mandatory validate round trip
// already performed inside Webhook. Rather than guess at an undocumented
// endpoint, Verify fails closed and callers should rely on the ITN
// webhook (Webhook above already re-confirms with PayFast server-side).
func (p *PayFastProvider) Verify(ctx context.Context, reference string) (Result, error) {
	return Result{}, fmt.Errorf("payments: payfast: Verify is not supported by this adapter (no documented polling endpoint); rely on the ITN webhook, which already re-confirms server-side via PayFast's validate endpoint")
}
