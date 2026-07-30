package substrate

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	kotvasync "github.com/vul-os/kotva/bindings/go"

	"github.com/vul-os/cackle/internal/scan"
)

const eventID = "evt_partition"

// cacheDir is one wazero compilation cache shared by every Open in this
// package. Compiling the module is by far the most expensive step and these
// tests open dozens of replicas; without the cache the package spends over a
// minute recompiling identical bytes. Sharing it also means Options.CacheDir is
// exercised on every run rather than only in a test written to check it.
var cacheDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cackle-substrate-cache")
	if err != nil {
		fmt.Fprintf(os.Stderr, "substrate tests: create compilation cache: %v\n", err)
		os.Exit(1)
	}
	cacheDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// openLedger opens a ledger with its own fresh identity — one replica, i.e.
// one gate or one server.
func openLedger(t *testing.T) *Ledger {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	return openLedgerWithKey(t, priv)
}

func openLedgerWithKey(t *testing.T, priv ed25519.PrivateKey) *Ledger {
	t.Helper()
	ctx := context.Background()
	l, err := Open(ctx, Options{
		EventID:  eventID,
		Signer:   kotvasync.InMemorySigner{PrivateKey: priv},
		CacheDir: cacheDir,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = l.Close(ctx) })
	return l
}

func claim(ticket, device, gate string, at time.Time, result scan.Status) scan.QueuedAdmission {
	return scan.QueuedAdmission{
		TicketID:  ticket,
		EventID:   eventID,
		GateID:    gate,
		DeviceID:  device,
		ScannedAt: at.UTC(),
		Result:    result,
	}
}

// fold is Fold with the replay-era receiver clock and a real clock far enough
// ahead that the future-claim bound never fires — the ordinary case.
func fold(t *testing.T, l *Ledger, a scan.QueuedAdmission) Record {
	t.Helper()
	rec, err := l.Fold(a, a.ScannedAt, a.ScannedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Fold(%s/%s): %v", a.TicketID, a.DeviceID, err)
	}
	return rec
}

// TestTwoPartitionedGatesDoubleAdmitAndTheDuplicateSurvivesConvergence is the
// test this whole package exists for.
//
// Two gates are offline from each other. The same ticket is presented at both.
// Each gate consults only its OWN local dedupe set, so each legitimately sees a
// first scan and each admits — one person, two entrances, two admits. That is
// not prevented and cannot be; the test asserts it happened.
//
// Then the partition heals and the gates exchange envelopes. The assertion
// that matters is that the union does NOT collapse the two admissions into
// one: after convergence each replica independently reports the same ticket
// with TWO surviving admitting claims from two devices. If the element were
// keyed on the ticket alone, this test would find one claim and a clean report,
// while two people were inside on one ticket.
func TestTwoPartitionedGatesDoubleAdmitAndTheDuplicateSurvivesConvergence(t *testing.T) {
	gateA := openLedger(t)
	gateB := openLedger(t)

	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	ticket := "tkt_shared"

	// --- during the partition -------------------------------------------
	//
	// Each gate's local SeenSet is empty for this ticket, so each admits. The
	// two Folds happen on two replicas that cannot see each other.
	admitA := claim(ticket, "device-a", "North", base, scan.Admitted)
	admitB := claim(ticket, "device-b", "South", base.Add(90*time.Second), scan.Admitted)

	recA := fold(t, gateA, admitA)
	recB := fold(t, gateB, admitB)

	// Neither replica can see the other's admission yet. This is the honest
	// state of the world mid-partition, and it is why prevention is impossible.
	if got, err := gateA.Claims(ticket); err != nil {
		t.Fatalf("gateA.Claims: %v", err)
	} else if len(got) != 1 {
		t.Fatalf("mid-partition gate A should see only its own claim, got %d: %+v", len(got), got)
	}
	if got, err := gateB.Claims(ticket); err != nil {
		t.Fatalf("gateB.Claims: %v", err)
	} else if len(got) != 1 {
		t.Fatalf("mid-partition gate B should see only its own claim, got %d: %+v", len(got), got)
	}
	if conflicts, err := gateA.Conflicts(); err != nil {
		t.Fatalf("gateA.Conflicts: %v", err)
	} else if len(conflicts) != 0 {
		t.Fatalf("mid-partition gate A cannot know about a conflict, got %+v", conflicts)
	}

	// --- the partition heals --------------------------------------------
	//
	// The gates exchange the envelopes they minted. Direction is irrelevant to
	// the outcome, which is the point of a union merge.
	ingest(t, gateA, recB, admitB.ScannedAt)
	ingest(t, gateB, recA, admitA.ScannedAt)

	// --- after convergence ----------------------------------------------
	for name, l := range map[string]*Ledger{"gate A": gateA, "gate B": gateB} {
		claims, err := l.Claims(ticket)
		if err != nil {
			t.Fatalf("%s Claims: %v", name, err)
		}
		if len(claims) != 2 {
			t.Fatalf("%s: the two admissions must BOTH survive the union; got %d: %+v",
				name, len(claims), claims)
		}
		// Deterministic order: earliest scan first.
		if claims[0].DeviceID != "device-a" || claims[1].DeviceID != "device-b" {
			t.Fatalf("%s: claims not in scan order: %+v", name, claims)
		}
		if !claims[0].ScannedAt.Equal(admitA.ScannedAt) || !claims[1].ScannedAt.Equal(admitB.ScannedAt) {
			t.Fatalf("%s: scan times did not round-trip: %+v", name, claims)
		}
		if claims[0].GateID != "North" || claims[1].GateID != "South" {
			t.Fatalf("%s: gate ids did not round-trip: %+v", name, claims)
		}

		conflicts, err := l.Conflicts()
		if err != nil {
			t.Fatalf("%s Conflicts: %v", name, err)
		}
		if len(conflicts) != 1 {
			t.Fatalf("%s: expected exactly one surfaced conflict, got %d: %+v", name, len(conflicts), conflicts)
		}
		c := conflicts[0]
		if c.TicketID != ticket {
			t.Fatalf("%s: conflict names ticket %q, want %q", name, c.TicketID, ticket)
		}
		if len(c.Admitted) != 2 || c.Devices != 2 {
			t.Fatalf("%s: conflict must show 2 admitting claims from 2 devices, got %d/%d: %+v",
				name, len(c.Admitted), c.Devices, c)
		}
	}

	// Both replicas hold the same two ops, so the §6.1 state roots agree byte
	// for byte — a stronger convergence check than comparing rendered claims,
	// because it covers every element including ones no report displays.
	rootA, err := gateA.StateRoot()
	if err != nil {
		t.Fatalf("gateA.StateRoot: %v", err)
	}
	rootB, err := gateB.StateRoot()
	if err != nil {
		t.Fatalf("gateB.StateRoot: %v", err)
	}
	if rootA != rootB {
		t.Fatalf("converged replicas disagree on state root:\n  A %s\n  B %s", rootA, rootB)
	}
	if rootA == "" {
		t.Fatal("state root is empty")
	}
}

func ingest(t *testing.T, l *Ledger, rec Record, era time.Time) {
	t.Helper()
	a, fresh, err := l.Ingest(rec.Cose, era, era.Add(time.Hour))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if !fresh {
		t.Fatalf("Ingest reported a peer's unseen claim as already known: %+v", a)
	}
}

// TestConvergenceIsOrderIndependent exchanges the same envelopes in both
// possible orders and asserts the merged view is identical. Union merge must
// not care which message arrived first.
func TestConvergenceIsOrderIndependent(t *testing.T) {
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	ticket := "tkt_order"

	mint := func() []Record {
		l := openLedger(t)
		return []Record{
			fold(t, l, claim(ticket, "device-a", "North", base, scan.Admitted)),
			fold(t, l, claim(ticket, "device-b", "South", base.Add(time.Minute), scan.Admitted)),
			fold(t, l, claim(ticket, "device-c", "East", base.Add(2*time.Minute), scan.Duplicate)),
		}
	}
	recs := mint()

	render := func(order []int) string {
		l := openLedger(t)
		for _, i := range order {
			// Re-ingesting an already-known op is a no-op, which the "fresh"
			// assertion in ingest would flag, so call Ingest directly here.
			if _, _, err := l.Ingest(recs[i].Cose, base, base.Add(time.Hour)); err != nil {
				t.Fatalf("Ingest %d: %v", i, err)
			}
		}
		claims, err := l.Claims(ticket)
		if err != nil {
			t.Fatalf("Claims: %v", err)
		}
		var sb strings.Builder
		for _, c := range claims {
			sb.WriteString(c.ScannedAt.Format(time.RFC3339Nano) + "|" + c.DeviceID + "|" + string(c.Result) + "\n")
		}
		return sb.String()
	}

	want := render([]int{0, 1, 2})
	for _, order := range [][]int{{2, 1, 0}, {1, 0, 2}, {2, 0, 1}, {1, 2, 0}, {0, 2, 1}} {
		if got := render(order); got != want {
			t.Fatalf("order %v produced a different merged view:\n got %s\nwant %s", order, got, want)
		}
	}
	// Three claims but only two ADMITTING devices: the refusing gate is
	// reported in Claims and excluded from the conflict count.
	l := openLedger(t)
	for i := range recs {
		if _, _, err := l.Ingest(recs[i].Cose, base, base.Add(time.Hour)); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}
	conflicts, err := l.Conflicts()
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Devices != 2 || len(conflicts[0].Claims) != 3 {
		t.Fatalf("expected 1 conflict over 3 claims from 2 admitting devices, got %+v", conflicts)
	}
}

// TestFoldIsIdempotent is the property the derived reconciliation view depends
// on: the `admissions` table is re-folded on every report, so folding the same
// row twice must not grow the set.
func TestFoldIsIdempotent(t *testing.T) {
	l := openLedger(t)
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	a := claim("tkt_idem", "device-a", "North", base, scan.Admitted)

	first := fold(t, l, a)
	second := fold(t, l, a)
	if first.ID != second.ID {
		t.Fatalf("folding the same claim twice produced two content addresses: %s vs %s", first.ID, second.ID)
	}

	claims, err := l.Claims("tkt_idem")
	if err != nil {
		t.Fatalf("Claims: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("re-folding one claim must leave one element, got %d: %+v", len(claims), claims)
	}
	if conflicts, err := l.Conflicts(); err != nil {
		t.Fatalf("Conflicts: %v", err)
	} else if len(conflicts) != 0 {
		t.Fatalf("one device's single claim is not a conflict, got %+v", conflicts)
	}
}

// TestSameTicketSameDeviceSameMillisecondCollapses pins the one collapse this
// mapping accepts, and asserts it is the collapse the sync idempotency key
// already defined rather than an accident.
func TestSameTicketSameDeviceSameMillisecondCollapses(t *testing.T) {
	l := openLedger(t)
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)

	// Identical (ticket, device, scanned_at) — by scan.SyncKey's contract this
	// IS one scan event, re-uploaded.
	fold(t, l, claim("tkt_same", "device-a", "North", base, scan.Admitted))
	fold(t, l, claim("tkt_same", "device-a", "North", base, scan.Admitted))
	if claims, err := l.Claims("tkt_same"); err != nil {
		t.Fatalf("Claims: %v", err)
	} else if len(claims) != 1 {
		t.Fatalf("one scan event must be one element, got %d", len(claims))
	}

	// One millisecond apart on the same device is two elements, so nothing
	// wider than the documented key collapses.
	fold(t, l, claim("tkt_same", "device-a", "North", base.Add(time.Millisecond), scan.Admitted))
	if claims, err := l.Claims("tkt_same"); err != nil {
		t.Fatalf("Claims: %v", err)
	} else if len(claims) != 2 {
		t.Fatalf("two distinct scan times must be two elements, got %d", len(claims))
	}
	// Still not a cross-gate conflict: one device cannot double-admit, because
	// its own dedupe claim is atomic.
	if conflicts, err := l.Conflicts(); err != nil {
		t.Fatalf("Conflicts: %v", err)
	} else if len(conflicts) != 0 {
		t.Fatalf("two claims from ONE device are not a cross-gate conflict, got %+v", conflicts)
	}
}

// TestDistinctDevicesNeverCollapse is the direct guard against the BeepBite
// trap, inverted for Cackle: two gates' claims about the same ticket differing
// ONLY in device must remain two elements. If they collapsed, the report would
// come back clean while two people were inside.
func TestDistinctDevicesNeverCollapse(t *testing.T) {
	l := openLedger(t)
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)

	// Same ticket, same gate name, same instant, same result, same note.
	// Device is the ONLY difference.
	fold(t, l, claim("tkt_collapse", "device-a", "North", base, scan.Admitted))
	fold(t, l, claim("tkt_collapse", "device-b", "North", base, scan.Admitted))

	claims, err := l.Claims("tkt_collapse")
	if err != nil {
		t.Fatalf("Claims: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("two devices' admissions collapsed into %d element(s) — the duplicate would be invisible: %+v",
			len(claims), claims)
	}
	conflicts, err := l.Conflicts()
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(conflicts) != 1 || conflicts[0].Devices != 2 {
		t.Fatalf("expected a 2-device conflict, got %+v", conflicts)
	}
}

// TestFutureClaimRefused covers the bound that replaces the engine's §3 skew
// check on the replay path. It must refuse without folding anything.
func TestFutureClaimRefused(t *testing.T) {
	l := openLedger(t)
	now := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)

	future := claim("tkt_future", "device-a", "North", now.Add(DefaultMaxFutureSkew+time.Minute), scan.Admitted)
	if _, err := l.Fold(future, future.ScannedAt, now); !errors.Is(err, ErrFutureClaim) {
		t.Fatalf("expected ErrFutureClaim, got %v", err)
	}
	if claims, err := l.Claims("tkt_future"); err != nil {
		t.Fatalf("Claims: %v", err)
	} else if len(claims) != 0 {
		t.Fatalf("a refused claim must leave the replica untouched, got %+v", claims)
	}
	if l.Stats().Refused != 1 {
		t.Fatalf("refusal not counted: %+v", l.Stats())
	}

	// Just inside the bound is accepted, so the guard is a bound and not a
	// blanket refusal of anything not in the past.
	ok := claim("tkt_edge", "device-a", "North", now.Add(time.Minute), scan.Admitted)
	if _, err := l.Fold(ok, ok.ScannedAt, now); err != nil {
		t.Fatalf("a claim one minute ahead is within the bound, got %v", err)
	}
}

// TestIngestRefusesForeignNamespace covers §7: a claim about another event's
// door is not this replica's to merge.
func TestIngestRefusesForeignNamespace(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	other, err := Open(ctx, Options{
		EventID: "evt_elsewhere", Signer: kotvasync.InMemorySigner{PrivateKey: priv}, CacheDir: cacheDir})
	if err != nil {
		t.Fatalf("Open other: %v", err)
	}
	defer other.Close(ctx) //nolint:errcheck // test cleanup

	foreign := scan.QueuedAdmission{
		TicketID: "tkt_foreign", EventID: "evt_elsewhere", GateID: "North",
		DeviceID: "device-a", ScannedAt: base, Result: scan.Admitted,
	}
	rec, err := other.Fold(foreign, base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("Fold on the other ledger: %v", err)
	}

	l := openLedger(t)
	if _, _, err := l.Ingest(rec.Cose, base, base.Add(time.Hour)); err == nil {
		t.Fatal("ingesting a claim from another event's namespace must be refused")
	} else if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("refusal should name the namespace, got %v", err)
	}
	if l.Stats().Ingested != 0 {
		t.Fatalf("a refused claim must not count as ingested: %+v", l.Stats())
	}
}

// TestIngestRefusesTamperedEnvelope asserts the fail-closed signature check.
func TestIngestRefusesTamperedEnvelope(t *testing.T) {
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	src := openLedger(t)
	rec := fold(t, src, claim("tkt_tamper", "device-a", "North", base, scan.Admitted))

	tampered := make([]byte, len(rec.Cose))
	copy(tampered, rec.Cose)
	tampered[len(tampered)/2] ^= 0xff

	l := openLedger(t)
	if _, _, err := l.Ingest(tampered, base, base.Add(time.Hour)); err == nil {
		t.Fatal("a tampered envelope must be refused")
	}
	if claims, err := l.Claims("tkt_tamper"); err != nil {
		t.Fatalf("Claims: %v", err)
	} else if len(claims) != 0 {
		t.Fatalf("a refused envelope must leave the replica untouched, got %+v", claims)
	}
}

// TestFoldRefusesUnusableClaims covers the inputs that would silently break
// the mapping's one guarantee.
func TestFoldRefusesUnusableClaims(t *testing.T) {
	l := openLedger(t)
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)

	cases := map[string]scan.QueuedAdmission{
		// Without a device the element cannot tell two gates apart, which is
		// the one thing this mapping exists to do.
		"no device":     claim("tkt_x", "", "North", base, scan.Admitted),
		"no ticket":     claim("", "device-a", "North", base, scan.Admitted),
		"slash in id":   claim("tkt/x", "device-a", "North", base, scan.Admitted),
		"another event": {TicketID: "tkt_x", EventID: "evt_other", DeviceID: "device-a", ScannedAt: base, Result: scan.Admitted},
	}
	for name, a := range cases {
		if _, err := l.Fold(a, base, base.Add(time.Hour)); err == nil {
			t.Fatalf("%s: expected a refusal", name)
		}
	}
}

// TestElementRoundTripsEveryField guards the canonical encoding: a field that
// silently fails to round-trip would make a report describe the wrong door.
func TestElementRoundTripsEveryField(t *testing.T) {
	want := scan.QueuedAdmission{
		TicketID:  "tkt_round",
		EventID:   eventID,
		GateID:    "North Gate — Lane 3",
		DeviceID:  "device-α/β",
		ScannedAt: time.Date(2026, 7, 30, 19, 4, 5, 123000000, time.UTC),
		Result:    scan.Duplicate,
		Note:      `ticket already admitted; note with "quotes" and a \backslash`,
	}
	el, err := element(want)
	if err != nil {
		t.Fatalf("element: %v", err)
	}
	got, err := decodeElement(el)
	if err != nil {
		t.Fatalf("decodeElement: %v", err)
	}
	if got != want {
		t.Fatalf("round trip lost data:\n got %+v\nwant %+v", got, want)
	}
}

// TestDecodeElementRefusesStampPayloadMismatch asserts the readback
// consistency check: the device tag and the payload's device id are one fact
// recorded twice, and a disagreement means the element was not built here.
func TestDecodeElementRefusesStampPayloadMismatch(t *testing.T) {
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	el, err := element(claim("tkt_stamp", "device-a", "North", base, scan.Admitted))
	if err != nil {
		t.Fatalf("element: %v", err)
	}
	raw, err := untagBytes(el)
	if err != nil {
		t.Fatalf("untagBytes: %v", err)
	}
	// Flip a bit inside the device tag, leaving the payload alone.
	raw[12] ^= 0x01
	if _, err := decodeElement(kotvasync.Bytes(raw)); err == nil {
		t.Fatal("a stamp that disagrees with its payload must be refused")
	}
}

// TestOpKindIsReadFromTheEngine asserts the §4.2 number is not hard-coded.
func TestOpKindIsReadFromTheEngine(t *testing.T) {
	l := openLedger(t)
	k, err := l.in.OpKinds()
	if err != nil {
		t.Fatalf("OpKinds: %v", err)
	}
	if l.setAdd != k.SetAdd {
		t.Fatalf("ledger uses kind %d, engine reports set_add = %d", l.setAdd, k.SetAdd)
	}
	// And the mapping models set_add ONLY — an op of any other kind must be
	// refused rather than coerced.
	if l.setAdd == k.LWWSet {
		t.Fatal("set_add and lww_set cannot be the same number")
	}
	v, err := l.AlgebraVersion()
	if err != nil {
		t.Fatalf("AlgebraVersion: %v", err)
	}
	if v.Substrate == "" || v.HLCSkewMS == 0 {
		t.Fatalf("engine did not report a substrate revision and skew bound: %+v", v)
	}
}

// TestOpenRefusesIncompleteOptions keeps the constructor fail-closed.
func TestOpenRefusesIncompleteOptions(t *testing.T) {
	ctx := context.Background()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := Open(ctx, Options{Signer: kotvasync.InMemorySigner{PrivateKey: priv}, CacheDir: cacheDir}); err == nil {
		t.Fatal("Open with no event id must fail — the namespace is what keeps events apart")
	}
	if _, err := Open(ctx, Options{EventID: eventID, CacheDir: cacheDir}); err == nil {
		t.Fatal("Open with no signer must fail")
	}
}

// TestEphemeralSignerProducesStableClaims pins exactly what an ephemeral
// identity does and does not change, because the reconciliation view is derived
// under a fresh key on every request and its output has to be reproducible.
//
// Op IDs differ: the author is part of an op's canonical bytes and therefore
// part of its §4.1 content address. Claims, Conflicts and StateRoot do not
// differ: every element field is derived from the claim, and §6.1 addresses the
// merged OBSERVABLE state rather than the add-tags carrying it.
//
// The last assertion is the load-bearing one for StateRoot's usefulness here,
// so it is guarded against passing vacuously: a ledger holding a DIFFERENT set
// of claims must produce a different root.
func TestEphemeralSignerProducesStableClaims(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	a := claim("tkt_eph", "device-a", "North", base, scan.Admitted)
	b := claim("tkt_eph", "device-b", "South", base.Add(time.Minute), scan.Admitted)

	run := func(claims ...scan.QueuedAdmission) ([]scan.QueuedAdmission, string, string) {
		signer, err := NewEphemeralSigner()
		if err != nil {
			t.Fatalf("NewEphemeralSigner: %v", err)
		}
		l, err := Open(ctx, Options{EventID: eventID, Signer: signer, CacheDir: cacheDir})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer l.Close(ctx) //nolint:errcheck // test cleanup
		var lastID string
		for _, c := range claims {
			lastID = fold(t, l, c).ID
		}
		got, err := l.Claims("tkt_eph")
		if err != nil {
			t.Fatalf("Claims: %v", err)
		}
		root, err := l.StateRoot()
		if err != nil {
			t.Fatalf("StateRoot: %v", err)
		}
		return got, root, lastID
	}

	claims1, root1, id1 := run(a, b)
	claims2, root2, id2 := run(a, b)
	if len(claims1) != 2 || len(claims2) != 2 {
		t.Fatalf("expected 2 claims each time, got %d and %d", len(claims1), len(claims2))
	}
	for i := range claims1 {
		if claims1[i] != claims2[i] {
			t.Fatalf("claim %d differed across folds:\n %+v\n %+v", i, claims1[i], claims2[i])
		}
	}
	if id1 == id2 {
		t.Fatal("two different identities produced the same op id; the author is supposed to be " +
			"part of the content address")
	}
	if root1 != root2 {
		t.Fatalf("the same claims under two identities must give the same observable-state root:\n  %s\n  %s",
			root1, root2)
	}

	// Anti-vacuous guard: the root has to actually depend on the claims.
	_, rootDifferent, _ := run(a)
	if rootDifferent == root1 {
		t.Fatal("a ledger holding one claim has the same root as one holding two; " +
			"StateRoot is not addressing the state and the equality above proves nothing")
	}
}

// TestVersionVectorNamesThisReplica keeps the §5.1 surface honest — it is the
// seam a future pull protocol advertises from.
func TestVersionVectorNamesThisReplica(t *testing.T) {
	l := openLedger(t)
	base := time.Date(2026, 7, 30, 19, 0, 0, 0, time.UTC)
	fold(t, l, claim("tkt_vv", "device-a", "North", base, scan.Admitted))

	marks, err := l.VersionVector()
	if err != nil {
		t.Fatalf("VersionVector: %v", err)
	}
	if len(marks) != 1 {
		t.Fatalf("expected one author mark, got %d: %+v", len(marks), marks)
	}
	if marks[0].Author != l.Author() {
		t.Fatalf("mark names author %s, this replica is %s", marks[0].Author, l.Author())
	}
	if marks[0].HLC.Wall != uint64(base.UnixMilli()) {
		t.Fatalf("mark wall is %d, want the claim's scan time %d", marks[0].HLC.Wall, base.UnixMilli())
	}
}
