package httpapi

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// ipLimiter is a per-client-IP token bucket, used to rate-limit auth
// endpoints and /api/scan per the house security bar. It intentionally
// only trusts r.RemoteAddr (never X-Forwarded-For) — trusting a
// client-supplied header for rate-limit bucketing is a trivial bypass
// (attacker just sends a fresh value per request). Self-hosters running
// behind a reverse proxy that they trust can extend this later; the
// default must fail closed rather than be spoofable out of the box.
type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

func newIPLimiter(r rate.Limit, b int) *ipLimiter {
	return &ipLimiter{limiters: make(map[string]*rate.Limiter), r: r, b: b}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	lim, ok := l.limiters[ip]
	if !ok {
		lim = rate.NewLimiter(l.r, l.b)
		l.limiters[ip] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimit returns middleware that rejects requests once ip exceeds l's
// budget, with a JSON 429 matching the standard error shape.
func rateLimit(l *ipLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.allow(clientIP(r)) {
				tooManyRequests(w, "rate limit exceeded, try again shortly")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
