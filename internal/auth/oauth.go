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
func (s *Service) LoginWithOAuth(ctx context.Context, info OAuthUserInfo) (*store.User, error) {
	if info.Provider == "" || info.Subject == "" {
		return nil, errors.New("auth: oauth info missing provider/subject")
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
		// Provider gave no usable email: synthesize a unique placeholder
		// so the NOT NULL UNIQUE constraint on users.email is satisfied.
		email = fmt.Sprintf("%s:%s@oauth.invalid", info.Provider, info.Subject)
	}

	u := &store.User{Email: email, PasswordHash: hash, Name: info.Name}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("auth: oauth create user: %w", err)
	}
	return u, nil
}
