package payments

// Stablecoin adapter — watch-only, no custody, ever.
//
// This adapter never generates, derives, or holds a private key. Cackle's
// own design constraint ("Cackle never holds funds") is at its strictest
// here: the organiser runs their OWN wallet (hardware wallet, any software
// wallet, a multisig — whatever they already control) and pre-generates a
// small POOL of receive addresses from it. This adapter's only job is to
// (1) hand out one of those pre-existing addresses per order, via a
// caller-supplied StablecoinAddressAllocator (the same storage-agnostic
// seam shape as SeenStore/OrderLookup in provider.go — Cackle never
// persists this itself), and (2) watch the chain for confirmed incoming
// token transfers to that address using a configurable, generic block
// explorer / indexer HTTP API. No wallet library, no HD derivation, no key
// material of any kind is imported or held anywhere in this file.
//
// Built against the Etherscan-family "account tokentx" API — the same
// documented shape is shared, unmodified, by Etherscan, BscScan,
// Polygonscan, Arbiscan and most other EVM block explorers, which is
// exactly the "configurable RPC/indexer endpoint" the payments contract
// asks for if a chain-specific dependency isn't warranted:
//   - Docs: https://docs.etherscan.io/api-endpoints/tokens#get-a-list-of-erc20-token-transfer-events-by-address
//
// Confidence: HIGH for the tokentx response shape used below (to, value,
// tokenDecimal, confirmations, timeStamp, hash — long-stable fields on one
// of the most widely integrated block explorer APIs), INCLUDING the
// well-known quirk that an empty result set comes back as
// {"status":"0","message":"No transactions found","result":[]} rather than
// an actual error — this adapter special-cases exactly that message rather
// than treating every status=="0" as a hard failure. Everything else with
// status=="0" IS treated as an error (rate limiting, a bad API key,
// etc) — fail closed. Unit-tested only (no real indexer account was used
// for these tests, only httptest fakes matching the documented shape) —
// see stablecoin_test.go.
//
// This adapter deliberately scopes itself to a single quote currency (see
// EnvStablecoinQuoteCurrency): it assumes the configured token (USDC/USDT)
// is pegged 1:1 to that currency's major unit and does NOT perform any
// other FX conversion — no rate oracle is wired up here, and this author is
// not confident enough in any specific one to fake it. An order priced in
// any other currency is refused at Begin with a clear configuration error,
// never silently mis-converted.
//
// # Address reuse — a real, stated hazard
//
// This adapter's reconciliation assumes each pool address is used for
// EXACTLY ONE order — it sums ALL qualifying incoming transfers to that
// address (after the order's allocation time) as belonging to that one
// order. If your StablecoinAddressAllocator implementation ever reissues
// an address to a second order (e.g. the pool runs out and wraps around)
// while the first order is still open, transfers will conflate across
// orders. This is a real correctness requirement on the allocator, not
// something this file can detect or defend against purely from chain data
// — state it loudly to whoever implements StablecoinAddressAllocator.
//
// # Underpayment / overpayment
//
// Underpayment: the running total of qualifying transfers is compared
// EXACTLY against the order's stored fiat total; anything less reports
// StatusPending (or StatusFailed once the order's TTL has passed) — never
// StatusPaid. Overpayment: if the total exceeds the ask, this adapter still
// reports StatusPaid with the ACTUAL (larger) total — the generic
// Reconcile() step in provider.go then rejects it as ErrAmountMismatch
// rather than silently accepting it, the same flagging mechanism documented
// on btcpay.go.
//
// # Confirmations
//
// EnvStablecoinMinConfirmations is REQUIRED with no default: a "safe"
// confirmation count varies enormously by chain (mainnet vs. an L2 vs. a
// testnet), and guessing one would be worse than making the operator state
// it explicitly. A transfer only counts toward the running total once ITS
// OWN confirmations meet or exceed this threshold — zero-conf transfers are
// never counted.
//
// # No webhook — Verify (polling) only, honestly
//
// Free-tier block explorer APIs generally do not push webhooks. Rather than
// invent a fictional push mechanism, Webhook always returns an error here;
// Capabilities().Webhooks is false, and callers must poll Verify
// periodically instead.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// ProviderNameStablecoin is the stable Name() this provider registers
// under.
const ProviderNameStablecoin = "stablecoin"

// Environment variables StablecoinProvider reads from.
const (
	EnvStablecoinIndexerBaseURL    = "CACKLE_STABLECOIN_INDEXER_BASE_URL"  // e.g. https://api.etherscan.io/api (or any Etherscan-family explorer)
	EnvStablecoinIndexerAPIKey     = "CACKLE_STABLECOIN_INDEXER_API_KEY"   // required
	EnvStablecoinTokenContract     = "CACKLE_STABLECOIN_TOKEN_CONTRACT"    // the ERC20 contract address for the token being watched
	EnvStablecoinTokenDecimals     = "CACKLE_STABLECOIN_TOKEN_DECIMALS"    // required, e.g. 6 for USDC/USDT
	EnvStablecoinQuoteCurrency     = "CACKLE_STABLECOIN_QUOTE_CURRENCY"    // optional, default "USD" — the ONLY currency this adapter accepts orders in
	EnvStablecoinMinConfirmations  = "CACKLE_STABLECOIN_MIN_CONFIRMATIONS" // required, no default (see file doc comment)
	EnvStablecoinOrderTTLSeconds   = "CACKLE_STABLECOIN_ORDER_TTL_SECONDS" // optional, default 3600 (1 hour)
	stablecoinDefaultQuoteCurrency = "USD"
	stablecoinDefaultOrderTTLSecs  = 3600
)

// Sentinel errors specific to the stablecoin adapter.
var (
	ErrStablecoinNotConfigured        = errors.New("payments: stablecoin: " + EnvStablecoinIndexerBaseURL + ", " + EnvStablecoinIndexerAPIKey + ", " + EnvStablecoinTokenContract + ", " + EnvStablecoinTokenDecimals + " and " + EnvStablecoinMinConfirmations + " must all be set")
	ErrStablecoinUnsupportedCurrency  = errors.New("payments: stablecoin: this adapter only accepts orders priced in its configured quote currency (no FX conversion is wired up)")
	ErrStablecoinUnexpectedStatus     = errors.New("payments: stablecoin: unexpected indexer response status")
	ErrStablecoinMalformedResponse    = errors.New("payments: stablecoin: malformed indexer response")
	ErrStablecoinResponseTooLarge     = errors.New("payments: stablecoin: response body exceeds size limit")
	ErrStablecoinTokenDecimalMismatch = errors.New("payments: stablecoin: indexer reported a tokenDecimal that does not match the configured token contract's decimals — check CACKLE_STABLECOIN_TOKEN_CONTRACT/CACKLE_STABLECOIN_TOKEN_DECIMALS")
	// ErrStablecoinNoWebhook is always returned by Webhook — see file doc
	// comment's "No webhook" section.
	ErrStablecoinNoWebhook = errors.New("payments: stablecoin: this adapter has no webhook mechanism; poll Verify instead")
)

// StablecoinAllocation is what Begin records for an order via
// StablecoinAddressAllocator.Allocate, and what Verify recovers via
// StablecoinAddressAllocator.Lookup.
type StablecoinAllocation struct {
	Address     string
	AmountMinor int64
	Currency    string
	AllocatedAt time.Time
}

// StablecoinAddressAllocator is how Begin obtains a receive address for an
// order, and how Verify later recovers what that address was for — the
// same storage-agnostic seam shape as SeenStore/OrderLookup in provider.go.
// Implementations wrap the CALLER's own pool of pre-existing addresses,
// each already controlled by a wallet the organiser holds the keys to.
// Cackle never derives or stores a private key anywhere in this package.
//
// See the file doc comment's "address reuse" section: implementations MUST
// NOT hand out the same address to two orders that are concurrently open.
type StablecoinAddressAllocator interface {
	// Allocate assigns an unused address to orderReference, records
	// amountMinor/currency/the current time against it, and returns that
	// allocation.
	Allocate(ctx context.Context, orderReference string, amountMinor int64, currency string) (StablecoinAllocation, error)
	// Lookup returns what Allocate previously recorded for address (the
	// only thing Verify receives as its "reference" parameter, per the
	// Provider interface).
	Lookup(ctx context.Context, address string) (StablecoinAllocation, error)
}

// StablecoinProvider implements Provider as a watch-only, non-custodial
// on-chain payment checker. See the file-level doc comment for design
// rationale, confidence notes, and stated limitations.
type StablecoinProvider struct {
	indexerBaseURL string
	indexerAPIKey  string
	tokenContract  string
	tokenDecimals  int
	quoteCurrency  string
	minConfirms    int
	orderTTL       time.Duration
	httpClient     *http.Client
	allocator      StablecoinAddressAllocator
}

// NewStablecoin constructs a StablecoinProvider from
// EnvStablecoinIndexerBaseURL, EnvStablecoinIndexerAPIKey,
// EnvStablecoinTokenContract, EnvStablecoinTokenDecimals and
// EnvStablecoinMinConfirmations (all required, no defaults), plus the
// optional EnvStablecoinQuoteCurrency and EnvStablecoinOrderTTLSeconds.
// allocator must be non-nil — see StablecoinAddressAllocator's doc comment.
func NewStablecoin(allocator StablecoinAddressAllocator) (*StablecoinProvider, error) {
	if allocator == nil {
		return nil, errors.New("payments: stablecoin: a non-nil StablecoinAddressAllocator is required")
	}
	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv(EnvStablecoinIndexerBaseURL)), "/")
	key := strings.TrimSpace(os.Getenv(EnvStablecoinIndexerAPIKey))
	contract := strings.TrimSpace(os.Getenv(EnvStablecoinTokenContract))
	decimalsStr := strings.TrimSpace(os.Getenv(EnvStablecoinTokenDecimals))
	minConfirmsStr := strings.TrimSpace(os.Getenv(EnvStablecoinMinConfirmations))
	if base == "" || key == "" || contract == "" || decimalsStr == "" || minConfirmsStr == "" {
		return nil, ErrStablecoinNotConfigured
	}
	decimals, err := strconv.Atoi(decimalsStr)
	if err != nil || decimals < 0 {
		return nil, fmt.Errorf("payments: stablecoin: %s must be a non-negative integer", EnvStablecoinTokenDecimals)
	}
	minConfirms, err := strconv.Atoi(minConfirmsStr)
	if err != nil || minConfirms <= 0 {
		return nil, fmt.Errorf("payments: stablecoin: %s must be a positive integer", EnvStablecoinMinConfirmations)
	}
	quoteCurrency := strings.ToUpper(strings.TrimSpace(os.Getenv(EnvStablecoinQuoteCurrency)))
	if quoteCurrency == "" {
		quoteCurrency = stablecoinDefaultQuoteCurrency
	}
	ttlSecs := stablecoinDefaultOrderTTLSecs
	if v := strings.TrimSpace(os.Getenv(EnvStablecoinOrderTTLSeconds)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("payments: stablecoin: %s must be a positive integer number of seconds", EnvStablecoinOrderTTLSeconds)
		}
		ttlSecs = n
	}
	return &StablecoinProvider{
		indexerBaseURL: base,
		indexerAPIKey:  key,
		tokenContract:  contract,
		tokenDecimals:  decimals,
		quoteCurrency:  quoteCurrency,
		minConfirms:    minConfirms,
		orderTTL:       time.Duration(ttlSecs) * time.Second,
		httpClient:     &http.Client{Timeout: cryptoDefaultHTTPTimeout},
		allocator:      allocator,
	}, nil
}

// Name implements Provider.
func (p *StablecoinProvider) Name() string { return ProviderNameStablecoin }

// Capabilities implements Provider.
func (p *StablecoinProvider) Capabilities() Capabilities {
	return Capabilities{
		Currencies:    []string{p.quoteCurrency},
		Countries:     nil,
		Flow:          FlowInvoice,
		Refunds:       false,
		Payouts:       false, // funds land directly in the organiser's own wallet; nothing to pay out
		Webhooks:      false, // see file doc comment: Verify (polling) only
		ZeroDecimalOK: true,
	}
}

// Begin allocates a receive address for the order (via the configured
// StablecoinAddressAllocator) and returns it as payment instructions. It
// refuses any currency other than the configured quote currency.
func (p *StablecoinProvider) Begin(ctx context.Context, o Order) (Charge, error) {
	if strings.TrimSpace(o.Reference) == "" {
		return Charge{}, errors.New("payments: stablecoin: order reference is required")
	}
	if o.AmountMinor <= 0 {
		return Charge{}, errors.New("payments: stablecoin: amount_minor must be positive")
	}
	currency := strings.ToUpper(strings.TrimSpace(o.Currency))
	if currency != p.quoteCurrency {
		return Charge{}, fmt.Errorf("%w: order currency %q, configured quote currency %q", ErrStablecoinUnsupportedCurrency, currency, p.quoteCurrency)
	}

	alloc, err := p.allocator.Allocate(ctx, o.Reference, o.AmountMinor, currency)
	if err != nil {
		return Charge{}, fmt.Errorf("payments: stablecoin: allocate address: %w", err)
	}
	if strings.TrimSpace(alloc.Address) == "" {
		return Charge{}, errors.New("payments: stablecoin: allocator returned an empty address")
	}

	return Charge{
		Provider:  ProviderNameStablecoin,
		Reference: alloc.Address,
		Instructions: fmt.Sprintf(
			"Send exactly %s %s worth of tokens to %s within %s. Underpayment will not settle this order; overpayment will be flagged for manual review, not refunded automatically.",
			minorAmountForDisplay(o.AmountMinor, currency), currency, alloc.Address, p.orderTTL,
		),
	}, nil
}

// minorAmountForDisplay renders an amount for a human-readable instructions
// string. Errors from minorToMajorString are treated as "unknown" rather
// than propagated, since Begin has already validated amount/currency by
// this point and this is cosmetic text, not something reconciliation
// depends on.
func minorAmountForDisplay(amountMinor int64, currency string) string {
	s, err := minorToMajorString(amountMinor, currency)
	if err != nil {
		return fmt.Sprintf("%d (minor units)", amountMinor)
	}
	return s
}

// Verify looks up what address reference (the allocated address) was
// allocated for, then sums qualifying (confirmed, post-allocation) token
// transfers to it via the configured indexer.
func (p *StablecoinProvider) Verify(ctx context.Context, reference string) (Result, error) {
	address := strings.TrimSpace(reference)
	if address == "" {
		return Result{}, errors.New("payments: stablecoin: reference (address) is required")
	}
	alloc, err := p.allocator.Lookup(ctx, address)
	if err != nil {
		return Result{}, fmt.Errorf("payments: stablecoin: lookup allocation: %w", err)
	}

	transfers, err := p.fetchTokenTransfers(ctx, address)
	if err != nil {
		return Result{}, err
	}

	total := new(big.Int)
	for _, tx := range transfers {
		if !strings.EqualFold(tx.To, address) {
			continue
		}
		txDecimals, err := strconv.Atoi(tx.TokenDecimal)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrStablecoinMalformedResponse, err)
		}
		if txDecimals != p.tokenDecimals {
			return Result{}, ErrStablecoinTokenDecimalMismatch
		}
		confirms, err := strconv.Atoi(tx.Confirmations)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrStablecoinMalformedResponse, err)
		}
		if confirms < p.minConfirms {
			continue // not enough confirmations yet — never count a zero/low-conf transfer
		}
		txTimeUnix, err := strconv.ParseInt(tx.TimeStamp, 10, 64)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrStablecoinMalformedResponse, err)
		}
		if time.Unix(txTimeUnix, 0).Before(alloc.AllocatedAt) {
			continue // predates this order's allocation — see "address reuse" hazard in file doc comment
		}
		value, ok := new(big.Int).SetString(tx.Value, 10)
		if !ok {
			return Result{}, fmt.Errorf("%w: unparseable transfer value %q", ErrStablecoinMalformedResponse, tx.Value)
		}
		total.Add(total, value)
	}

	receivedMinor := stablecoinTokenMinorToFiatMinor(total, p.tokenDecimals, currencyExponentForStablecoin(alloc.Currency))

	raw, _ := json.Marshal(transfers)
	result := Result{
		Provider:    ProviderNameStablecoin,
		Reference:   address,
		AmountMinor: receivedMinor,
		Currency:    alloc.Currency,
		Raw:         json.RawMessage(raw),
	}

	expired := time.Since(alloc.AllocatedAt) > p.orderTTL
	switch {
	case receivedMinor >= alloc.AmountMinor && receivedMinor > 0:
		result.Status = StatusPaid
		result.PaidAt = time.Now().UTC()
		// EventID must be non-empty whenever Status is StatusPaid (see
		// provider.go's replay-protection contract). There is no single
		// "settlement event id" for a sum of possibly-multiple on-chain
		// transfers, so this is derived from the address and the total —
		// stable for a given address+total, changes if more arrives later.
		result.EventID = fmt.Sprintf("%s:%s", address, total.String())
	case expired:
		result.Status = StatusFailed
	default:
		result.Status = StatusPending
	}
	return result, nil
}

// currencyExponentForStablecoin is a tiny indirection so this file's
// arithmetic reads clearly at the call site; it is exactly
// minorUnitExponent from currency.go.
func currencyExponentForStablecoin(currency string) int {
	return minorUnitExponent(currency)
}

// stablecoinTokenMinorToFiatMinor converts a token amount (in the token's
// own smallest units, e.g. 6-decimal USDC) into the quote currency's minor
// units (e.g. 2-decimal USD cents), assuming a 1:1 peg between one token
// major unit and one quote-currency major unit. Uses exact big.Int
// arithmetic throughout — never float64 — and floors any remainder (dust
// below the quote currency's smallest unit), which only ever under-counts
// what was received, never over-counts it.
func stablecoinTokenMinorToFiatMinor(tokenMinor *big.Int, tokenDecimals, fiatExponent int) int64 {
	num := new(big.Int).Mul(tokenMinor, bigPow10(fiatExponent))
	den := bigPow10(tokenDecimals)
	result := new(big.Int).Quo(num, den)
	return result.Int64()
}

func bigPow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// Webhook always fails: see the file doc comment's "No webhook" section.
// This adapter is poll-only (Verify), which Capabilities().Webhooks (false)
// also reflects.
func (p *StablecoinProvider) Webhook(ctx context.Context, r *http.Request) (Result, error) {
	return Result{}, ErrStablecoinNoWebhook
}

// stablecoinTokenTx mirrors the fields this adapter reads from an
// Etherscan-family "tokentx" API entry.
type stablecoinTokenTx struct {
	To            string `json:"to"`
	Value         string `json:"value"`
	TokenDecimal  string `json:"tokenDecimal"`
	Confirmations string `json:"confirmations"`
	TimeStamp     string `json:"timeStamp"`
	Hash          string `json:"hash"`
}

// fetchTokenTransfers calls the configured indexer's "tokentx" action for
// address and this adapter's configured token contract, handling the
// well-known Etherscan-family quirk that an empty result set is reported as
// {"status":"0","message":"No transactions found"} rather than an error.
func (p *StablecoinProvider) fetchTokenTransfers(ctx context.Context, address string) ([]stablecoinTokenTx, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoDefaultHTTPTimeout)
	defer cancel()

	q := url.Values{}
	q.Set("module", "account")
	q.Set("action", "tokentx")
	q.Set("address", address)
	q.Set("contractaddress", p.tokenContract)
	q.Set("sort", "desc")
	q.Set("apikey", p.indexerAPIKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.indexerBaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("payments: stablecoin: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("payments: stablecoin: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := boundedRead(resp.Body, cryptoMaxBodyBytes)
	if err != nil {
		if errors.Is(err, errBoundedReadTooLarge) {
			return nil, ErrStablecoinResponseTooLarge
		}
		return nil, fmt.Errorf("payments: stablecoin: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: http %d", ErrStablecoinUnexpectedStatus, resp.StatusCode)
	}

	var envelope struct {
		Status  string          `json:"status"`
		Message string          `json:"message"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStablecoinMalformedResponse, err)
	}

	var txs []stablecoinTokenTx
	if err := json.Unmarshal(envelope.Result, &txs); err != nil {
		// result wasn't an array — either a real API error (bad key, rate
		// limited) or an unexpected shape. Fail closed either way.
		return nil, fmt.Errorf("%w: %s", ErrStablecoinUnexpectedStatus, firstNonEmpty(envelope.Message, "result was not a transfer list"))
	}
	if envelope.Status != "1" && envelope.Status != "0" {
		return nil, fmt.Errorf("%w: unrecognised status %q", ErrStablecoinUnexpectedStatus, envelope.Status)
	}
	if envelope.Status == "0" && len(txs) > 0 {
		// status=="0" is only tolerated for the documented empty-result
		// case; a non-empty transfer list alongside status=="0" is
		// inconsistent enough to fail closed rather than trust it.
		return nil, fmt.Errorf("%w: status=0 with a non-empty result", ErrStablecoinUnexpectedStatus)
	}
	return txs, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
