package httpapi

import (
	"context"

	"github.com/vul-os/cackle/internal/store"
)

type ctxKey int

const (
	ctxKeyUser ctxKey = iota
	ctxKeySessionToken
	ctxKeyAuthViaCookie
)

// userFromContext returns the authenticated user, if the request carried a
// valid session (Bearer or cookie). ok is false for an anonymous request.
func userFromContext(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(*store.User)
	return u, ok
}

// sessionTokenFromContext returns the raw plaintext session token that
// resolved the current request's user, if any, and whether it arrived via
// the httpOnly cookie (true) or an Authorization: Bearer header (false).
// It is only ever used server-side (CSRF double-submit check, logout) —
// never logged.
func sessionTokenFromContext(ctx context.Context) (token string, viaCookie bool) {
	t, _ := ctx.Value(ctxKeySessionToken).(string)
	vc, _ := ctx.Value(ctxKeyAuthViaCookie).(bool)
	return t, vc
}
