package auth

// Google sign-in — a real OAuthProvider, and the only one that ships.
//
// # Why this file is careful about a thing Cackle does not otherwise do
//
// Cackle's defining claim is that the gate works with no internet. OAuth
// requires internet by definition, so this provider is fenced in hard:
//
//   - It is ORGANISER/ADMIN LOGIN ONLY. Nothing on the scanning or admission
//     path may reach it. internal/scan does not import internal/auth at all,
//     and the httpapi scan routes never call anything in this file — see
//     TestGoogle_NoScanPathReachesOAuth, which proves that structurally rather
//     than by promise.
//   - It is OFF unless an operator configured it. FromEnvGoogle returns
//     (nil, nil) when neither environment variable is set: not an error, not a
//     default, simply absent. A provider that is absent is never registered,
//     its routes are never mounted, and the sign-in button never renders.
//   - Password login is unaffected in every case. An operator whose internet
//     is down must still be able to reach their own box.
//
// # What this implementation verifies, and what it deliberately does not
//
// It uses the OpenID Connect authorization-code flow and reads the identity
// out of the ID token returned by the token endpoint. It checks issuer,
// audience, expiry, issued-at, nonce, and email_verified.
//
// It does NOT verify the ID token's RS256 signature against Google's JWKS.
// That is a deliberate, spec-sanctioned choice, not an oversight: OpenID
// Connect Core 1.0 §3.1.3.7 item 6 states that when the ID token is received
// directly from the Token Endpoint over a TLS-protected channel, the TLS
// server validation MAY be used to validate the issuer in place of checking
// the token signature. This code only ever obtains an ID token that way — a
// POST we initiate, to a pinned https:// endpoint, authenticated with the
// client secret. It never accepts an ID token from a redirect, a request
// body, or any other caller-influenced source, which is the case the
// signature check exists to defend. The cost of the alternative is a JWKS
// fetch, a key cache and a rotation story for one more network dependency in
// a product whose whole point is not having network dependencies.
//
// If this file ever grows a path that accepts an ID token from anywhere other
// than its own token-endpoint response, that reasoning collapses and the
// signature check becomes mandatory.
//
// # Verification status
//
// UNIT-TESTED, NOT SANDBOX-VERIFIED — the same register docs/PAYMENTS.md uses
// for payment adapters. Every test here runs against a local httptest server
// standing in for Google's token endpoint. No test in this repository has ever
// contacted Google, and nothing here has been exercised against a real Google
// Cloud OAuth client. What is tested is this code's protocol handling; what is
// not tested is Google's behaviour.

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProviderNameGoogle is the stable Name() this provider registers under and
// the value stored in the oauth_identities.provider column.
const ProviderNameGoogle = "google"

// The ONLY place Google credentials may come from: the environment. There is
// no default, no generated fallback, and no config file. Both must be set for
// the provider to exist at all; see FromEnvGoogle.
const (
	EnvGoogleClientID     = "CACKLE_GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret = "CACKLE_GOOGLE_CLIENT_SECRET"
)

// Google's live endpoints. Constants rather than configuration: an operator
// has no legitimate reason to point Cackle's "Google sign-in" at a different
// host, and making it settable would turn a misconfiguration into a
// credential-exfiltration channel. Tests override them through an unexported
// option that is not reachable from outside this package.
const (
	googleAuthEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint = "https://oauth2.googleapis.com/token"
)

// googleIssuers are the two issuer strings Google documents for ID tokens.
// Both are accepted; anything else is refused.
var googleIssuers = []string{"https://accounts.google.com", "accounts.google.com"}

// googleScope is deliberately minimal: enough to identify the operator and
// nothing more. No Drive, no calendar, no offline access — Cackle never
// acts on a user's behalf at Google, it only asks who they are.
const googleScope = "openid email profile"

const (
	// googleHTTPTimeout bounds the one outbound call this package can make,
	// applied even when the caller's context has no deadline. Sign-in must
	// not be able to hang a request thread indefinitely.
	googleHTTPTimeout = 15 * time.Second

	// googleMaxResponseBytes caps the token-endpoint response we will read.
	// An ID token is a few hundred bytes; anything approaching this is not a
	// token response.
	googleMaxResponseBytes = 1 << 20 // 1 MiB

	// googleClockSkew is how far the ID token's iat may sit in the future
	// before it is refused, allowing for ordinary clock drift between this
	// box and Google.
	googleClockSkew = 60 * time.Second
)

// OAuth failure modes that a handler must be able to tell apart. Each one is
// a refusal to sign anyone in.
var (
	// ErrOAuthEmailUnverified is returned when the provider hands back an
	// email address it does not itself vouch for. See the account-linking
	// note on LoginWithOAuth for why this is fatal rather than a downgrade.
	ErrOAuthEmailUnverified = errors.New("auth: oauth provider did not verify this email address")

	// ErrOAuthTokenInvalid covers every way the ID token failed inspection:
	// wrong issuer, wrong audience, expired, issued in the future,
	// malformed, or a nonce that is not the one we asked for. They are
	// deliberately one error: the client is told "sign-in failed" in all
	// cases, and the specific reason is a server-side log line.
	ErrOAuthTokenInvalid = errors.New("auth: oauth id token failed validation")

	// ErrOAuthExchangeFailed is a transport or provider-side failure of the
	// code-for-token exchange.
	ErrOAuthExchangeFailed = errors.New("auth: oauth code exchange failed")

	// ErrOAuthMisconfigured is a half-configured provider: one of the two
	// environment variables set and not the other.
	ErrOAuthMisconfigured = errors.New("auth: oauth provider is only half configured")
)

// OIDCProvider is the optional extension of OAuthProvider for providers that
// return an OpenID Connect ID token and therefore support a nonce.
//
// It is a separate interface rather than a widened OAuthProvider because the
// seam and its stub predate this file and are used by tests that have no
// nonce to offer. A caller that holds an OAuthProvider can type-assert to
// this one; internal/httpapi does exactly that, and requires it — the OAuth
// routes refuse to mount for a provider that cannot carry a nonce, so
// "provider does not support nonce" can never silently become "nonce not
// checked".
type OIDCProvider interface {
	OAuthProvider

	// AuthURLWithNonce is AuthURL plus an OIDC nonce, which the provider
	// must echo back inside the ID token.
	AuthURLWithNonce(state, nonce, redirectURI string) string

	// ExchangeWithNonce is Exchange, and additionally REQUIRES that the ID
	// token's nonce claim equals wantNonce exactly. A token with no nonce
	// when one was asked for, or a different nonce, is refused.
	ExchangeWithNonce(ctx context.Context, code, redirectURI, wantNonce string) (OAuthUserInfo, error)
}

// GoogleProvider implements OAuthProvider and OIDCProvider against Google's
// OpenID Connect endpoints.
//
// The zero value is not usable; construct with NewGoogleProvider or
// FromEnvGoogle. It holds a client secret and therefore has no String(),
// no exported fields, and no method that returns either credential — a
// value of this type cannot be logged into revealing anything.
type GoogleProvider struct {
	clientID     string
	clientSecret string

	authEndpoint  string
	tokenEndpoint string

	httpClient *http.Client
	now        func() time.Time
}

// googleOption configures a GoogleProvider. Unexported on purpose: the two
// things it can change (endpoints, clock) are test seams, and an operator
// being able to redirect "Google sign-in" at another host would be a
// credential-exfiltration channel rather than a feature.
type googleOption func(*GoogleProvider)

func withGoogleEndpoints(authURL, tokenURL string) googleOption {
	return func(p *GoogleProvider) {
		p.authEndpoint = authURL
		p.tokenEndpoint = tokenURL
	}
}

func withGoogleClock(now func() time.Time) googleOption {
	return func(p *GoogleProvider) { p.now = now }
}

// NewGoogleProvider builds a live Google provider. Both credentials are
// required — there is no anonymous or "public client" mode, because the
// authorization-code flow's security argument here rests on the token
// endpoint being reached with the client secret over TLS (see the file
// comment on signature validation).
func NewGoogleProvider(clientID, clientSecret string, opts ...googleOption) (*GoogleProvider, error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("%w: both %s and %s are required", ErrOAuthMisconfigured, EnvGoogleClientID, EnvGoogleClientSecret)
	}
	p := &GoogleProvider{
		clientID:      clientID,
		clientSecret:  clientSecret,
		authEndpoint:  googleAuthEndpoint,
		tokenEndpoint: googleTokenEndpoint,
		httpClient:    &http.Client{Timeout: googleHTTPTimeout},
		now:           time.Now,
	}
	for _, o := range opts {
		o(p)
	}
	return p, nil
}

// FromEnvGoogle builds the Google provider from the environment, and is the
// single decision point for whether Google sign-in exists on this box.
//
// Three outcomes, mirroring how internal/config treats event-key material:
//
//   - NEITHER variable set: (nil, nil). Not configured is not an error. This
//     is the default for every deployment that never asks for Google
//     sign-in, and it is what keeps a stock Cackle from issuing any external
//     request at all.
//   - BOTH set: a live provider.
//   - Exactly ONE set: ErrOAuthMisconfigured, which cmd/cackle turns into a
//     refusal to start. Falling back to "off" would mean an operator who
//     believes they enabled Google sign-in silently gets a box where the
//     button never appears and nobody finds out why.
//
// A variable set to whitespace counts as set-but-empty, i.e. misconfigured,
// for the same reason CACKLE_KEY_PASSPHRASE= is an error there: an empty
// value is an operator who thinks they configured something.
func FromEnvGoogle() (*GoogleProvider, error) {
	id, idSet := os.LookupEnv(EnvGoogleClientID)
	secret, secretSet := os.LookupEnv(EnvGoogleClientSecret)
	if !idSet && !secretSet {
		return nil, nil
	}
	switch {
	case !idSet || strings.TrimSpace(id) == "":
		return nil, fmt.Errorf("%w: %s is set but %s is missing or empty", ErrOAuthMisconfigured, EnvGoogleClientSecret, EnvGoogleClientID)
	case !secretSet || strings.TrimSpace(secret) == "":
		return nil, fmt.Errorf("%w: %s is set but %s is missing or empty", ErrOAuthMisconfigured, EnvGoogleClientID, EnvGoogleClientSecret)
	}
	return NewGoogleProvider(id, secret)
}

// Name implements OAuthProvider.
func (p *GoogleProvider) Name() string { return ProviderNameGoogle }

// AuthURL implements OAuthProvider: the nonce-free form. Because Exchange
// then requires the ID token to carry NO nonce, the two halves stay
// consistent — whichever pair a caller uses, the nonce it asked for is the
// nonce it insists on getting back.
//
// internal/httpapi does not use this pair; it requires OIDCProvider and uses
// AuthURLWithNonce/ExchangeWithNonce.
func (p *GoogleProvider) AuthURL(state, redirectURI string) string {
	return p.AuthURLWithNonce(state, "", redirectURI)
}

// AuthURLWithNonce implements OIDCProvider.
func (p *GoogleProvider) AuthURLWithNonce(state, nonce, redirectURI string) string {
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", googleScope)
	q.Set("state", state)
	if nonce != "" {
		q.Set("nonce", nonce)
	}
	// online: Cackle never wants a refresh token. It does not act at Google
	// on anyone's behalf after sign-in, so asking for offline access would
	// be asking for a credential with nothing to spend it on.
	q.Set("access_type", "online")
	// An operator signing in to their own box is often on a shared machine
	// at a venue; make the account choice explicit rather than silently
	// reusing whichever Google session the browser happens to hold.
	q.Set("prompt", "select_account")
	return p.authEndpoint + "?" + q.Encode()
}

// Exchange implements OAuthProvider, requiring the ID token to carry no
// nonce. See AuthURL.
func (p *GoogleProvider) Exchange(ctx context.Context, code, redirectURI string) (OAuthUserInfo, error) {
	return p.ExchangeWithNonce(ctx, code, redirectURI, "")
}

// ExchangeWithNonce implements OIDCProvider: swap the authorization code for
// an ID token and resolve it to an identity.
//
// redirectURI is sent to the token endpoint and MUST be byte-identical to
// the one sent in the authorization request — Google checks it, and so does
// the caller (internal/httpapi derives it once from config and uses the same
// string in both places). It is a parameter rather than provider state
// because the OAuth spec makes it part of the exchange, not part of the
// client's identity.
//
// NOTHING in this method logs. The code, the access token and the ID token
// all pass through it, and none of them appears in a returned error either —
// error text carries at most an HTTP status and Google's own short error
// code (e.g. "invalid_grant").
func (p *GoogleProvider) ExchangeWithNonce(ctx context.Context, code, redirectURI, wantNonce string) (OAuthUserInfo, error) {
	if code == "" {
		return OAuthUserInfo{}, fmt.Errorf("%w: no authorization code", ErrOAuthExchangeFailed)
	}
	if redirectURI == "" {
		return OAuthUserInfo{}, fmt.Errorf("%w: no redirect URI", ErrOAuthExchangeFailed)
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.clientSecret)
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthUserInfo{}, fmt.Errorf("%w: build request", ErrOAuthExchangeFailed)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		// err can name the host and the transport failure; it cannot contain
		// the request body, so no credential travels with it.
		return OAuthUserInfo{}, fmt.Errorf("%w: %v", ErrOAuthExchangeFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, googleMaxResponseBytes))
	if err != nil {
		return OAuthUserInfo{}, fmt.Errorf("%w: read response", ErrOAuthExchangeFailed)
	}

	var tok struct {
		IDToken          string `json:"id_token"`
		ErrorCode        string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	// Decode errors are not fatal on the failure path — a non-200 with an
	// unparseable body still gets a status-only error below.
	_ = json.Unmarshal(body, &tok)

	if resp.StatusCode != http.StatusOK {
		// tok.ErrorCode is a short enum from Google ("invalid_grant",
		// "redirect_uri_mismatch"); error_description is deliberately NOT
		// included, since it is free text from a third party and this string
		// reaches a log file.
		if tok.ErrorCode != "" {
			return OAuthUserInfo{}, fmt.Errorf("%w: token endpoint status %d (%s)", ErrOAuthExchangeFailed, resp.StatusCode, tok.ErrorCode)
		}
		return OAuthUserInfo{}, fmt.Errorf("%w: token endpoint status %d", ErrOAuthExchangeFailed, resp.StatusCode)
	}
	if tok.IDToken == "" {
		return OAuthUserInfo{}, fmt.Errorf("%w: token response carried no id_token", ErrOAuthExchangeFailed)
	}

	return p.identityFromIDToken(tok.IDToken, wantNonce)
}

// googleIDClaims is the subset of the ID token this code reads.
//
// Aud and EmailVerified are `any` because both have more than one legal
// shape: aud is a string or an array of strings (RFC 7519 §4.1.3), and
// email_verified has been observed from Google as both a JSON boolean and
// the string "true". Parsing them loosely and then judging them strictly is
// safer than failing to parse and treating that as absent.
type googleIDClaims struct {
	Iss           string `json:"iss"`
	Aud           any    `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce"`
	Exp           int64  `json:"exp"`
	Iat           int64  `json:"iat"`
}

// identityFromIDToken inspects an ID token obtained from the token endpoint
// and turns it into an OAuthUserInfo, or refuses.
//
// Every check here is load-bearing and each has a test that fails if it is
// removed. The order is: shape, issuer, audience, time, nonce, subject,
// email verification.
func (p *GoogleProvider) identityFromIDToken(idToken, wantNonce string) (OAuthUserInfo, error) {
	claims, err := parseJWTClaims(idToken)
	if err != nil {
		return OAuthUserInfo{}, fmt.Errorf("%w: %v", ErrOAuthTokenInvalid, err)
	}

	if !containsString(googleIssuers, claims.Iss) {
		return OAuthUserInfo{}, fmt.Errorf("%w: unexpected issuer", ErrOAuthTokenInvalid)
	}

	if !audienceMatches(claims.Aud, p.clientID) {
		return OAuthUserInfo{}, fmt.Errorf("%w: audience is not this client", ErrOAuthTokenInvalid)
	}

	now := p.now()
	if claims.Exp == 0 || now.After(time.Unix(claims.Exp, 0)) {
		return OAuthUserInfo{}, fmt.Errorf("%w: expired", ErrOAuthTokenInvalid)
	}
	if claims.Iat != 0 && time.Unix(claims.Iat, 0).After(now.Add(googleClockSkew)) {
		return OAuthUserInfo{}, fmt.Errorf("%w: issued in the future", ErrOAuthTokenInvalid)
	}

	// Constant-time, and an exact match in both directions: a token with no
	// nonce satisfies only a request that asked for none.
	if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(wantNonce)) != 1 {
		return OAuthUserInfo{}, fmt.Errorf("%w: nonce mismatch", ErrOAuthTokenInvalid)
	}

	if claims.Sub == "" {
		return OAuthUserInfo{}, fmt.Errorf("%w: no subject", ErrOAuthTokenInvalid)
	}

	// The account-linking decision, enforced at its earliest possible point.
	// An unverified address from the provider is refused here, before any
	// store lookup, so no code path downstream can be tempted to link on it.
	// See LoginWithOAuth for the full reasoning.
	if !truthy(claims.EmailVerified) || claims.Email == "" {
		return OAuthUserInfo{}, ErrOAuthEmailUnverified
	}

	return OAuthUserInfo{
		Provider:      ProviderNameGoogle,
		Subject:       claims.Sub,
		Email:         claims.Email,
		Name:          claims.Name,
		EmailVerified: true,
	}, nil
}

// parseJWTClaims splits a compact JWS and decodes its payload. It does NOT
// verify the signature — see this file's header for why that is sound for
// the one source this code accepts a token from, and for what would make it
// unsound.
func parseJWTClaims(token string) (googleIDClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return googleIDClaims{}, errors.New("not a compact JWS")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleIDClaims{}, errors.New("payload is not base64url")
	}
	var claims googleIDClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return googleIDClaims{}, errors.New("payload is not JSON")
	}
	return claims, nil
}

// audienceMatches reports whether aud (a string or an array of strings)
// contains want exactly.
func audienceMatches(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return subtle.ConstantTimeCompare([]byte(v), []byte(want)) == 1
	case []any:
		for _, item := range v {
			s, ok := item.(string)
			if ok && subtle.ConstantTimeCompare([]byte(s), []byte(want)) == 1 {
				return true
			}
		}
	}
	return false
}

// truthy reads a claim that may be a JSON boolean or the string "true".
// Anything else — absent, null, "false", 1, "yes" — is false. A claim this
// code cannot confidently read as "yes" is a "no".
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	}
	return false
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
