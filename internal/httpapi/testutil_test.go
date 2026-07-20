package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/config"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/orders"
	"github.com/vul-os/cackle/internal/payments"
	"github.com/vul-os/cackle/internal/store"
)

// testHarness wires up a full, in-memory Cackle stack (real SQLite via
// :memory:, real auth/events/orders services, the stub payment provider)
// behind the real httpapi.New router — no mocks below the HTTP layer.
type testHarness struct {
	t        *testing.T
	handler  http.Handler
	store    *store.Store
	auth     *auth.Service
	events   *events.Service
	orders   *orders.Service
	payments *payments.Registry
	cfg      *config.Config
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	authSvc := auth.NewService(st)
	eventsSvc := events.New(st)

	reg := payments.NewRegistry()
	stub, err := payments.NewStub(true)
	if err != nil {
		t.Fatalf("new stub provider: %v", err)
	}
	if err := reg.Register(stub); err != nil {
		t.Fatalf("register stub provider: %v", err)
	}

	ordersSvc := orders.New(st, eventsSvc, reg)

	cfg := &config.Config{
		Addr:          ":0",
		DB:            ":memory:",
		BaseURL:       "http://localhost:8080",
		SessionSecret: "test-only-session-secret-not-for-prod",
	}

	h := New(Deps{
		Store:    st,
		Auth:     authSvc,
		Events:   eventsSvc,
		Orders:   ordersSvc,
		Payments: reg,
		Config:   cfg,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	return &testHarness{
		t: t, handler: h, store: st, auth: authSvc, events: eventsSvc,
		orders: ordersSvc, payments: reg, cfg: cfg,
	}
}

// do issues an HTTP request straight into the router (httptest, no real
// socket) and returns the recorder. body, if non-nil, is JSON-encoded.
func (h *testHarness) do(method, path, token string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal request body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.RemoteAddr = "203.0.113.10:12345"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
	return v
}

// signupUser creates a brand new account over HTTP and returns its bearer
// token and user id.
func (h *testHarness) signupUser(email, password, name string) (token, userID string) {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/auth/signup", "", signupRequest{Email: email, Password: password, Name: name})
	if rec.Code != http.StatusOK {
		h.t.Fatalf("signup %s: status %d body %s", email, rec.Code, rec.Body.String())
	}
	resp := decodeBody[struct {
		User  userView `json:"user"`
		Token string   `json:"token"`
	}](h.t, rec)
	return resp.Token, resp.User.ID
}

// newOrgWithOwner creates an org directly against the store (there is no
// HTTP route to create an org in BUILD-SPEC's API — org membership is
// assumed to already exist, e.g. via --demo seed data or an
// out-of-band/admin path) and makes userID its owner.
func (h *testHarness) newOrgWithOwner(name, slug, userID string) string {
	h.t.Helper()
	org := &store.Org{Name: name, Slug: slug}
	if err := h.store.CreateOrg(h.t.Context(), org); err != nil {
		h.t.Fatalf("create org: %v", err)
	}
	if err := h.store.AddOrgMember(h.t.Context(), &store.OrgMember{OrgID: org.ID, UserID: userID, Role: "owner"}); err != nil {
		h.t.Fatalf("add org member: %v", err)
	}
	return org.ID
}
