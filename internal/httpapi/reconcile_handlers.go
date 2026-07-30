package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/scan"
	"github.com/vul-os/cackle/internal/scan/substrate"
	"github.com/vul-os/cackle/internal/store"
)

// This file serves the answer to the one question an organiser has after an
// offline event that Cackle could not previously answer at all: which tickets
// got somebody through TWO doors while the gates could not see each other, and
// therefore how many extra people are inside.
//
// The reconciliation itself runs on the shared DMTAP Sync engine
// (internal/scan/substrate) rather than on merge logic written here — a set of
// admission claims is a §4.3 add-only set, and union is the whole merge rule.
// See that package's doc for the mapping and, importantly, for what is NOT
// claimed: two partitioned gates cannot be PREVENTED from double-admitting a
// ticket, and nothing on this route should be read as saying they can.
//
// Nothing here is on the admission path. `internal/scan.Decide` and
// `DecideWithBundle` never touch this file, never touch the engine, and never
// touch the network — a gate keeps admitting people with this whole subsystem
// unreachable.

// admissionClaimView is one gate device's account of one scan, as served.
type admissionClaimView struct {
	DeviceID  string    `json:"device_id"`
	GateID    string    `json:"gate_id,omitempty"`
	ScannedAt time.Time `json:"scanned_at"`
	// Result is the device's own verdict — what it did at the door. This is
	// the field that says whether somebody walked through.
	Result string `json:"result"`
	// ServerResult is what the server concluded and stored. It differs from
	// Result exactly when this claim lost the reconciliation: the device
	// admitted, another gate's admission was already recorded, and the server
	// downgraded this row to 'duplicate'. That downgrade is correct — one
	// ticket, one admitted row — but it is not what happened at the door, and
	// conflating the two is what previously made this whole report impossible.
	ServerResult string `json:"server_result,omitempty"`
	Note         string `json:"note,omitempty"`
}

// admissionConflictView is one ticket that more than one device admitted.
type admissionConflictView struct {
	TicketID string `json:"ticket_id"`
	// Devices is how many distinct devices each believed they were the sole
	// admission. Always >= 2.
	Devices int `json:"devices"`
	// ExtraAdmissions is Devices - 1: how many people got in on this ticket
	// beyond the one who legitimately should have. It is stated as its own
	// field because it is the number an organiser actually needs.
	ExtraAdmissions int                  `json:"extra_admissions"`
	Claims          []admissionClaimView `json:"claims"`
}

type admissionConflictsResponse struct {
	Conflicts []admissionConflictView `json:"conflicts"`
	// ExtraAdmissions totals ExtraAdmissions across every conflict.
	ExtraAdmissions int `json:"extra_admissions"`
	// Algebra names the merge algebra the conflicts were computed under, and
	// Engine the engine revision that supplied it. They are reported rather
	// than assumed so an operator comparing two deployments' reports can see
	// whether they merged under the same rules.
	Algebra string `json:"algebra"`
	Engine  string `json:"engine"`
	// Complete is false when this answer is knowably partial. It is a
	// deliberate, permanent false-by-default-when-unsure: a report about who
	// got through a door must never look authoritative when a device's log
	// has not arrived.
	Complete bool `json:"complete"`
	// Caveat always carries the limits of this answer in plain language, on
	// every response including an empty one. An empty conflicts list means
	// "no conflict among the claims that reached this server", never "no
	// double admissions happened".
	Caveat string `json:"caveat"`
}

const conflictsCaveat = "This is an after-the-fact record, not a guard: two gates partitioned from " +
	"each other cannot be prevented from admitting the same ticket, and no merge rule changes that. " +
	"It is also only as complete as the sync — a device whose log never reached this server " +
	"contributes nothing here, and a conflict it was part of looks from here like a clean single " +
	"admission."

// ledgerCompiler lazily compiles the shared sync engine, once per process, on
// first use.
//
// Lazy rather than eager at New() for a reason worth stating: compiling the
// WebAssembly module costs a few hundred milliseconds, and a deployment that
// never opens a reconciliation report should not pay it at boot. It also keeps
// every existing test that builds a router free of the cost unless it asks for
// this route.
//
// The error is cached alongside the compiler. A module that failed to compile
// will fail again for the same reason, and retrying it per request would turn
// one broken deployment into a per-request several-hundred-millisecond stall.
type ledgerCompiler struct {
	once     sync.Once
	compiler *substrate.Compiler
	err      error
}

func (c *ledgerCompiler) get(ctx context.Context, cacheDir string) (*substrate.Compiler, error) {
	c.once.Do(func() {
		c.compiler, c.err = substrate.NewCompiler(ctx, cacheDir)
	})
	return c.compiler, c.err
}

// handleAdmissionConflicts serves GET /api/events/{id}/admission-conflicts.
func (s *server) handleAdmissionConflicts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := userFromContext(ctx)
	eventID := chi.URLParam(r, "id")

	// Scanner is the right floor: the people who staffed the doors are the
	// people who need to know a door was double-used, and it is the same role
	// /api/scan and /api/scan/sync already require.
	ok, err := s.deps.Auth.CanManageEvent(ctx, user.ID, eventID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "admission conflicts rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a scanner/admin/owner on this event's org")
		return
	}

	rows, err := s.deps.Store.ListAdmissionClaimsForEvent(ctx, eventID)
	if err != nil {
		internalError(w, s.log(), "admission conflicts query", err)
		return
	}

	compiler, err := s.ledgers.get(ctx, s.deps.Config.ScanCacheDir())
	if err != nil {
		internalError(w, s.log(), "admission conflicts engine", err)
		return
	}

	// An ephemeral identity is correct here and the reasoning is in
	// substrate.NewEphemeralSigner: this view is DERIVED from the admissions
	// table on every request and no envelope is persisted or sent anywhere, so
	// there is nothing for a durable node key to authenticate — and no key
	// material to defend on an internet-facing host in exchange.
	signer, err := substrate.NewEphemeralSigner()
	if err != nil {
		internalError(w, s.log(), "admission conflicts identity", err)
		return
	}
	ledger, err := compiler.Ledger(ctx, substrate.Options{EventID: eventID, Signer: signer})
	if err != nil {
		internalError(w, s.log(), "admission conflicts ledger", err)
		return
	}
	defer ledger.Close(ctx) //nolint:errcheck // read-only replica, nothing to flush

	// serverResult keeps the server's stored verdict alongside the claim, so
	// the response can show both without the ledger having to model a field
	// that is not part of the replicated fact. Keyed on the sync idempotency
	// key, which is exactly what makes a claim unique.
	type claimKey struct {
		ticket, device string
		scannedAt      int64
	}
	serverResult := make(map[claimKey]string, len(rows))

	now := time.Now()
	complete := true
	for _, row := range rows {
		a := scan.QueuedAdmission{
			TicketID:  row.TicketID,
			EventID:   row.EventID,
			GateID:    row.GateID,
			DeviceID:  row.DeviceID,
			ScannedAt: row.ScannedAt,
			// The DEVICE's claim is the replicated fact. Folding the server's
			// already-reconciled `result` instead would fold the conclusion
			// back into its own premise: every downgraded row would arrive as
			// 'duplicate' and the conflict would be invisible — which is
			// precisely the bug migration 0002 exists to fix.
			Result: scan.Status(claimedResult(row)),
			Note:   row.Note,
		}
		serverResult[claimKey{row.TicketID, row.DeviceID, row.ScannedAt.UTC().UnixMilli()}] = row.Result

		// receiverNow is the claim's own era: this is a replay of a stored log,
		// and checking an hours-old admission against today's clock would
		// refuse every row. See substrate's package doc on what that costs and
		// what replaces it.
		if _, err := ledger.Fold(a, row.ScannedAt, now); err != nil {
			// One unfoldable row must not fail the whole report — an operator
			// with one corrupt row still needs the other conflicts. But the
			// answer is no longer complete, and it says so rather than looking
			// authoritative.
			complete = false
			s.log().Warn("admission conflicts: claim refused by the sync engine",
				"event_id", eventID, "ticket_id", row.TicketID, "device_id", row.DeviceID, "err", err)
			continue
		}
	}

	conflicts, err := ledger.Conflicts()
	if err != nil {
		internalError(w, s.log(), "admission conflicts merge", err)
		return
	}

	version, err := ledger.AlgebraVersion()
	if err != nil {
		internalError(w, s.log(), "admission conflicts engine version", err)
		return
	}

	resp := admissionConflictsResponse{
		Conflicts: make([]admissionConflictView, 0, len(conflicts)),
		Algebra:   substrate.AlgebraName,
		Engine:    version.Substrate,
		Complete:  complete,
		Caveat:    conflictsCaveat,
	}
	for _, c := range conflicts {
		view := admissionConflictView{
			TicketID:        c.TicketID,
			Devices:         c.Devices,
			ExtraAdmissions: c.Devices - 1,
			Claims:          make([]admissionClaimView, 0, len(c.Claims)),
		}
		for _, cl := range c.Claims {
			stored := serverResult[claimKey{cl.TicketID, cl.DeviceID, cl.ScannedAt.UTC().UnixMilli()}]
			cv := admissionClaimView{
				DeviceID:  cl.DeviceID,
				GateID:    cl.GateID,
				ScannedAt: cl.ScannedAt,
				Result:    string(cl.Result),
				Note:      cl.Note,
			}
			if stored != string(cl.Result) {
				cv.ServerResult = stored
			}
			view.Claims = append(view.Claims, cv)
		}
		resp.ExtraAdmissions += view.ExtraAdmissions
		resp.Conflicts = append(resp.Conflicts, view)
	}
	writeJSON(w, http.StatusOK, resp)
}

// claimedResult reads the scanning device's own verdict off an admission row.
//
// ReportedResult is the device's claim, stored verbatim by migration 0002.
// Falling back to Result covers two cases, both correct: a row written before
// that migration existed, and a row from the online /api/scan path where the
// server WAS the device's oracle and its verdict is the device's verdict.
func claimedResult(row store.AdmissionClaim) string {
	if row.ReportedResult != "" {
		return row.ReportedResult
	}
	return row.Result
}
