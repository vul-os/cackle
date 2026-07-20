package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/store"
)

// attendeeView is the wire shape for one row of GET
// /api/events/{id}/attendees. It intentionally omits the buyer's email —
// see handleListEventAttendees' doc for why.
type attendeeView struct {
	TicketID       string  `json:"ticket_id"`
	OrderID        string  `json:"order_id"`
	Serial         string  `json:"serial"`
	HolderName     string  `json:"holder_name"`
	Status         string  `json:"status"`
	TicketTypeID   string  `json:"ticket_type_id"`
	TicketTypeName string  `json:"ticket_type_name"`
	IssuedAt       string  `json:"issued_at"`
	VoidedAt       *string `json:"voided_at,omitempty"`
	AdmittedAt     *string `json:"admitted_at,omitempty"`
	Admitted       bool    `json:"admitted"`
}

func toAttendeeView(a store.AttendeeRow) attendeeView {
	v := attendeeView{
		TicketID:       a.TicketID,
		OrderID:        a.OrderID,
		Serial:         a.Serial,
		HolderName:     a.HolderName,
		Status:         a.Status,
		TicketTypeID:   a.TicketTypeID,
		TicketTypeName: a.TicketTypeName,
		IssuedAt:       nowText(a.IssuedAt),
		Admitted:       a.AdmittedAt != nil,
	}
	if a.VoidedAt != nil {
		s := nowText(*a.VoidedAt)
		v.VoidedAt = &s
	}
	if a.AdmittedAt != nil {
		s := nowText(*a.AdmittedAt)
		v.AdmittedAt = &s
	}
	return v
}

// handleListEventAttendees serves GET /api/events/{id}/attendees —
// scanner-and-up on the event's org (same bar as event stats and the
// scan-bundle: the door team needs this list as much as the organiser
// does; it is read-only either way).
//
// This is the roster the old attendees.jsx notice explained was missing:
// per-ticket holder name, ticket type, serial, order id, issue date, and
// admission status/time. It exists precisely so an organiser can answer
// "who is coming to my event" — a core feature no prior route covered
// (GET /api/tickets and GET /api/orders are both scoped to the calling
// user's own purchases, not an event's full attendee list).
//
// Personal-data decision: the response carries holder_name but never the
// buyer's email. holder_name is who shows up at the door — that's what an
// organiser/scanner needs to check someone in or answer a "is so-and-so on
// the list" question. buyer_email is contact information for whoever paid
// (frequently a different person than every holder on a multi-ticket
// order, e.g. someone buying for a group), and this route has no
// legitimate need to hand a bulk, CSV-exportable list of attendee emails
// to every scanner-role account on an org. If bulk attendee email/contact
// ever becomes a real feature (e.g. "email everyone with a ticket"), that
// should be its own narrowly-scoped, admin-only, server-side-send
// endpoint that never returns raw addresses to the client — not a field
// bolted onto this roster.
func (s *server) handleListEventAttendees(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "list event attendees rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a member of this event's org")
		return
	}

	q := r.URL.Query()
	filter := store.AttendeeFilter{
		Query:  q.Get("q"),
		Status: q.Get("status"),
	}
	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 0 {
			badRequest(w, "limit must be a non-negative integer")
			return
		}
		filter.Limit = n
	}
	if offset := q.Get("offset"); offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			badRequest(w, "offset must be a non-negative integer")
			return
		}
		filter.Offset = n
	}

	rows, total, err := s.deps.Store.ListEventAttendees(r.Context(), eventID, filter)
	if err != nil {
		internalError(w, s.log(), "list event attendees", err)
		return
	}

	out := make([]attendeeView, 0, len(rows))
	for _, a := range rows {
		out = append(out, toAttendeeView(a))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = store.DefaultAttendeeLimit
	}
	if limit > store.MaxAttendeeLimit {
		limit = store.MaxAttendeeLimit
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attendees": out,
		"total":     total,
		"limit":     limit,
		"offset":    filter.Offset,
	})
}
