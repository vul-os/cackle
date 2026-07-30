package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/vul-os/cackle/internal/store"
)

// OAuthUserInfo is the normalised identity an OAuthProvider hands back
// after a successful exchange.
type OAuthUserInfo struct {
	Provider string
	Subject  string
	Email    string
	Name     string
	// EmailVerified is the provider's own assertion that the person signing
	// in controls Email. It is NOT a convenience flag: LoginWithOAuth
	// refuses outright when it is false, because every account-linking
	// decision downstream rests on it. See LoginWithOAuth.
	EmailVerified bool
}

// OAuthProvider is the seam for third-party sign-in. Only a stub
// implementation ships here — a live provider (Google, GitHub, etc.) is
// intentionally not implemented in this package.
type OAuthProvider interface {
	// Name identifies the provider, e.g. "google". Used as the `provider`
	// column in oauth_identities.
	Name() string
	// AuthURL returns the URL to redirect the user to begin the flow.
	AuthURL(state, redirectURI string) string
	// Exchange completes the flow: given the callback code, resolve the
	// provider's user info.
	Exchange(ctx context.Context, code, redirectURI string) (OAuthUserInfo, error)
}

// StubOAuthProvider is a fixed, no-network OAuthProvider for tests and
// --demo. Exchange always returns the configured Info regardless of the
// code given — it never calls out to a real provider.
type StubOAuthProvider struct {
	ProviderName string
	Info         OAuthUserInfo
}

// NewStubOAuthProvider builds a stub provider that always resolves to info.
func NewStubOAuthProvider(name string, info OAuthUserInfo) *StubOAuthProvider {
	info.Provider = name
	return &StubOAuthProvider{ProviderName: name, Info: info}
}

func (p *StubOAuthProvider) Name() string { return p.ProviderName }

func (p *StubOAuthProvider) AuthURL(state, redirectURI string) string {
	return fmt.Sprintf("stub://%s/authorize?state=%s&redirect_uri=%s",
		p.ProviderName, url.QueryEscape(state), url.QueryEscape(redirectURI))
}

func (p *StubOAuthProvider) Exchange(_ context.Context, _ string, _ string) (OAuthUserInfo, error) {
	return p.Info, nil
}

// LoginWithOAuth resolves a completed OAuth exchange to a Cackle user:
//   - if (info.Provider, info.Subject) is already linked, returns that user;
//   - else if a user with info.Email already exists, links the identity to
//     that account (this is how "sign in with Google" attaches to an
//     existing native-password account);
//   - else creates a brand new account (with a random, unusable password —
//     native login stays impossible until the user sets one via
//     password-reset) and links the identity.
//
// # The account-linking decision, and why an unverified email is fatal
//
// Linking by email address is the whole reason this function is dangerous.
// If a provider says "this is bob@venue.example" and Cackle attaches that to
// the existing password account for bob@venue.example, then anybody who can
// make the provider say that sentence owns Bob's organisation — his events,
// his attendee roster, his payouts. With a provider that lets an account set
// an arbitrary unverified address on itself, that is a self-service account
// takeover, not an attack.
//
// So: EmailVerified is REQUIRED, and this is where the requirement is
// enforced, before any lookup. Not "link only when verified, otherwise
// create a fresh account" — refusing entirely is the only version with no
// second path to reason about. A fresh account keyed on an unverified
// address would still collide with the same address later, and the
// placeholder-email branch below would leave an operator staring at an
// account they cannot explain.
//
// What remains true after this rule, stated plainly rather than hidden: a
// verified email from the provider IS accepted as proof of control of that
// mailbox, and does link to an existing password account with the same
// address. That is the standard trade — it is what "verified" means — and it
// is the reason this is restricted to providers that make a verification
// assertion at all. If a future provider cannot make one, it does not belong
// here.
//
// GoogleProvider refuses an unverified address one layer earlier still, in
// identityFromIDToken, so a bad token never reaches this function. This check
// is the backstop, and the two are tested separately (see the mutation notes
// in oauth_handlers_test.go) so neither can mask the other.
func (s *Service) LoginWithOAuth(ctx context.Context, info OAuthUserInfo) (*store.User, error) {
	if info.Provider == "" || info.Subject == "" {
		return nil, errors.New("auth: oauth info missing provider/subject")
	}
	if !info.EmailVerified || info.Email == "" {
		return nil, ErrOAuthEmailUnverified
	}

	ident, err := s.store.GetOAuthIdentity(ctx, info.Provider, info.Subject)
	if err == nil {
		u, err := s.store.GetUserByID(ctx, ident.UserID)
		if err != nil {
			return nil, fmt.Errorf("auth: oauth linked user lookup: %w", err)
		}
		return u, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("auth: oauth identity lookup: %w", err)
	}

	u, err := s.findOrCreateOAuthUser(ctx, info)
	if err != nil {
		return nil, err
	}

	if err := s.store.CreateOAuthIdentity(ctx, &store.OAuthIdentity{
		Provider: info.Provider,
		Subject:  info.Subject,
		UserID:   u.ID,
	}); err != nil {
		return nil, fmt.Errorf("auth: oauth link identity: %w", err)
	}
	return u, nil
}

func (s *Service) findOrCreateOAuthUser(ctx context.Context, info OAuthUserInfo) (*store.User, error) {
	if info.Email != "" {
		if norm, err := normalizeEmail(info.Email); err == nil {
			existing, err := s.store.GetUserByEmail(ctx, norm)
			if err == nil {
				return existing, nil
			}
			if !errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("auth: oauth email lookup: %w", err)
			}
		}
	}

	randomPassword, _, err := newToken()
	if err != nil {
		return nil, err
	}
	hash, err := HashPassword(randomPassword)
	if err != nil {
		return nil, err
	}

	email := ""
	if info.Email != "" {
		if norm, err := normalizeEmail(info.Email); err == nil {
			email = norm
		}
	}
	if email == "" {
		// Reachable only when the provider asserted a VERIFIED address that
		// normalizeEmail then rejected as not an address at all — verified
		// garbage. LoginWithOAuth has already refused an empty one. Synthesize
		// a unique placeholder so the NOT NULL UNIQUE constraint on
		// users.email is satisfied, under a reserved TLD that can never
		// collide with a real address and can never receive mail.
		email = fmt.Sprintf("%s:%s@oauth.invalid", info.Provider, info.Subject)
	}

	u := &store.User{Email: email, PasswordHash: hash, Name: info.Name}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("auth: oauth create user: %w", err)
	}
	return u, nil
}
