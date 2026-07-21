package store

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestMigrateRunsCleanTwice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cackle.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	versions1, err := st.AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrations: %v", err)
	}
	// The schema is a single folded baseline (0001_init), so a clean open
	// records exactly one applied migration. The point of this test is the
	// idempotency checks below, not the count.
	if len(versions1) < 1 {
		t.Fatalf("expected the baseline migration applied, got %v", versions1)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-open the same DB file: Migrate must be idempotent.
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer st2.Close()

	versions2, err := st2.AppliedMigrations(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrations (2nd): %v", err)
	}
	if len(versions1) != len(versions2) {
		t.Fatalf("migration count changed across reopen: %v vs %v", versions1, versions2)
	}

	// Running Migrate explicitly again against the live *sql.DB must also
	// be a no-op.
	if err := Migrate(st2.DB()); err != nil {
		t.Fatalf("re-running Migrate: %v", err)
	}
}

func TestUserEmailUniqueConstraint(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u1 := &User{Email: "dup@example.com", PasswordHash: "x", Name: "One"}
	if err := st.CreateUser(ctx, u1); err != nil {
		t.Fatalf("create first user: %v", err)
	}

	u2 := &User{Email: "dup@example.com", PasswordHash: "y", Name: "Two"}
	if err := st.CreateUser(ctx, u2); err == nil {
		t.Fatal("expected unique constraint violation on duplicate email, got nil error")
	}
}

func TestGetUserNotFound(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.GetUserByID(ctx, "nonexistent"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := st.GetUserByEmail(ctx, "nobody@example.com"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u := &User{Email: "sess@example.com", PasswordHash: "x", Name: "Sess"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	sess := &Session{
		TokenHash: "deadbeef",
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := st.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := st.GetSessionByTokenHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.UserID != u.ID {
		t.Fatalf("session user id = %q, want %q", got.UserID, u.ID)
	}

	if err := st.DeleteSession(ctx, "deadbeef"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := st.GetSessionByTokenHash(ctx, "deadbeef"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Deleting an already-deleted / unknown session is not an error.
	if err := st.DeleteSession(ctx, "deadbeef"); err != nil {
		t.Fatalf("delete session (idempotent): %v", err)
	}
}

func TestOrgMembershipRoleConstraint(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	u := &User{Email: "org@example.com", PasswordHash: "x", Name: "Org"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	o := &Org{Name: "Acme", Slug: "acme"}
	if err := st.CreateOrg(ctx, o); err != nil {
		t.Fatalf("create org: %v", err)
	}

	bad := &OrgMember{OrgID: o.ID, UserID: u.ID, Role: "superadmin"}
	if err := st.AddOrgMember(ctx, bad); err == nil {
		t.Fatal("expected CHECK constraint violation for invalid role")
	}

	good := &OrgMember{OrgID: o.ID, UserID: u.ID, Role: "owner"}
	if err := st.AddOrgMember(ctx, good); err != nil {
		t.Fatalf("add valid org member: %v", err)
	}

	m, err := st.GetOrgMember(ctx, o.ID, u.ID)
	if err != nil {
		t.Fatalf("get org member: %v", err)
	}
	if m.Role != "owner" {
		t.Fatalf("role = %q, want owner", m.Role)
	}

	orgs, err := st.ListOrgsForUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("list orgs for user: %v", err)
	}
	if len(orgs) != 1 || orgs[0].Role != "owner" || orgs[0].Slug != "acme" {
		t.Fatalf("unexpected orgs for user: %+v", orgs)
	}
}

func TestListValidTicketIDsForEvent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	org := &Org{Name: "Acme", Slug: "acme-tix"}
	if err := st.CreateOrg(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	ev := &Event{
		OrgID: org.ID, Slug: "fest", Title: "Fest", Status: "published",
		StartsAt: time.Now(), EndsAt: time.Now().Add(time.Hour),
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	if err := st.CreateEventWithKey(ctx, ev, &EventKey{PublicKey: pub, PrivateKey: priv}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	tt := &TicketType{EventID: ev.ID, Name: "General", QuantityTotal: 10}
	if err := st.CreateTicketType(ctx, tt); err != nil {
		t.Fatalf("create ticket type: %v", err)
	}

	// A fresh event with no orders yet: the index must be empty, not an
	// error — this is the "no tickets issued for this event" fallback
	// case internal/scan.DecideWithBundle relies on.
	ids, err := st.ListValidTicketIDsForEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ListValidTicketIDsForEvent (empty): %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no valid ticket ids yet, got %v", ids)
	}

	order := &Order{EventID: ev.ID, BuyerEmail: "buyer@example.com"}
	if _, err := st.CreateOrderWithItems(ctx, order, []OrderLine{{TicketTypeID: tt.ID, Quantity: 3, UnitPriceMinor: 1000}}); err != nil {
		t.Fatalf("create order: %v", err)
	}
	tickets := []Ticket{
		{ID: "tik-1", OrderID: order.ID, EventID: ev.ID, TicketTypeID: tt.ID, Serial: "S1", Capability: "cap-1", IssuedAt: time.Now()},
		{ID: "tik-2", OrderID: order.ID, EventID: ev.ID, TicketTypeID: tt.ID, Serial: "S2", Capability: "cap-2", IssuedAt: time.Now()},
		{ID: "tik-3", OrderID: order.ID, EventID: ev.ID, TicketTypeID: tt.ID, Serial: "S3", Capability: "cap-3", IssuedAt: time.Now()},
	}
	if settled, err := st.SettleOrder(ctx, order.ID, time.Now(), tickets); err != nil || !settled {
		t.Fatalf("settle order: settled=%v err=%v", settled, err)
	}

	ids, err = st.ListValidTicketIDsForEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ListValidTicketIDsForEvent: %v", err)
	}
	assertSameIDs(t, ids, []string{"tik-1", "tik-2", "tik-3"})

	// Void one ticket (e.g. a refund) — it must drop out of the index
	// immediately, which is the entire point of this method existing.
	if err := st.VoidTicket(ctx, "tik-2", time.Now()); err != nil {
		t.Fatalf("void ticket: %v", err)
	}
	ids, err = st.ListValidTicketIDsForEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("ListValidTicketIDsForEvent (after void): %v", err)
	}
	assertSameIDs(t, ids, []string{"tik-1", "tik-3"})
}

func assertSameIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("id count = %d, want %d (got %v, want %v)", len(got), len(want), got, want)
	}
	seen := make(map[string]bool, len(got))
	for _, id := range got {
		seen[id] = true
	}
	for _, id := range want {
		if !seen[id] {
			t.Fatalf("expected id %q in result, got %v", id, got)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	// sessions.user_id references users(id); inserting for a nonexistent
	// user must fail if foreign_keys is actually ON.
	sess := &Session{TokenHash: "abc", UserID: "nonexistent", ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.CreateSession(ctx, sess); err == nil {
		t.Fatal("expected foreign key violation inserting session for nonexistent user")
	}
}

// --- payment_records (durable payment provider state) --------------------

func TestPaymentRecord_PutGetRoundtrip(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	if _, err := st.GetPaymentRecord(ctx, "manual", "ord-1"); err != ErrNotFound {
		t.Fatalf("GetPaymentRecord before insert: err = %v, want ErrNotFound", err)
	}

	rec := &PaymentRecord{
		Provider:     "manual",
		Reference:    "ord-1",
		AmountMinor:  123456,
		Currency:     "KWD", // three-decimal currency — nothing here should assume 2
		Status:       "pending",
		Instructions: "pay at the door",
	}
	if err := st.PutPaymentRecord(ctx, rec); err != nil {
		t.Fatalf("PutPaymentRecord: %v", err)
	}

	got, err := st.GetPaymentRecord(ctx, "manual", "ord-1")
	if err != nil {
		t.Fatalf("GetPaymentRecord: %v", err)
	}
	if got.AmountMinor != 123456 || got.Currency != "KWD" || got.Status != "pending" || got.Instructions != "pay at the door" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.MarkedAt != nil {
		t.Fatalf("MarkedAt = %v, want nil (never marked)", got.MarkedAt)
	}

	// Upsert: mark it paid, with the audit fields manual providers need.
	markedAt := time.Now().UTC().Truncate(time.Second)
	rec.Status = "paid"
	rec.MarkedBy = "owner@example.com"
	rec.MarkedAt = &markedAt
	if err := st.PutPaymentRecord(ctx, rec); err != nil {
		t.Fatalf("PutPaymentRecord (update): %v", err)
	}
	got2, err := st.GetPaymentRecord(ctx, "manual", "ord-1")
	if err != nil {
		t.Fatalf("GetPaymentRecord after update: %v", err)
	}
	if got2.Status != "paid" || got2.MarkedBy != "owner@example.com" {
		t.Fatalf("update mismatch: %+v", got2)
	}
	if got2.MarkedAt == nil || !got2.MarkedAt.Equal(markedAt) {
		t.Fatalf("MarkedAt = %v, want %v", got2.MarkedAt, markedAt)
	}

	list, err := st.ListPaymentRecords(ctx, "manual")
	if err != nil {
		t.Fatalf("ListPaymentRecords: %v", err)
	}
	if len(list) != 1 || list[0].Reference != "ord-1" {
		t.Fatalf("ListPaymentRecords = %+v, want exactly ord-1", list)
	}

	// A different provider's records are never mixed in.
	otherList, err := st.ListPaymentRecords(ctx, "lnbits")
	if err != nil {
		t.Fatalf("ListPaymentRecords(lnbits): %v", err)
	}
	if len(otherList) != 0 {
		t.Fatalf("ListPaymentRecords(lnbits) = %+v, want empty", otherList)
	}
}
