package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/events"
	"github.com/vul-os/cackle/internal/money"
	"github.com/vul-os/cackle/internal/scan"
	"github.com/vul-os/cackle/internal/store"
)

// NOTE on struct field names: internal/events was being written concurrently
// with this package (see WAVE2-CONTRACT.md) — only its Service method
// SIGNATURES were fixed ahead of time, not its struct field names. Field
// references below (ev.ID, ev.Title, ev.OrgID, ...) follow the schema in
// BUILD-SPEC.md with idiomatic Go naming and the same convention already
// used by internal/store's own types (User.ID, Org.Name, ...). These were
// reconciled against the real internal/events package once it landed — see
// the final report for anything that had to change.

// eventToMeta builds the minimal offline-gate context scan.Bundle needs
// from a full events.Event.
func eventToMeta(ev *events.Event) scan.EventMeta {
	return scan.EventMeta{
		EventID:   ev.ID,
		Title:     ev.Title,
		VenueName: ev.VenueName,
		StartsAt:  ev.StartsAt,
		EndsAt:    ev.EndsAt,
	}
}

// handleListPublicEvents serves GET /api/events?q=&from=&to=&limit= —
// published events only, no auth required.
func (s *server) handleListPublicEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := events.PublicFilter{Query: q.Get("q"), Category: q.Get("category")}

	if from := q.Get("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			badRequest(w, "from must be RFC3339")
			return
		}
		filter.From = t
	}
	if to := q.Get("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			badRequest(w, "to must be RFC3339")
			return
		}
		filter.To = t
	}
	if limit := q.Get("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n < 0 {
			badRequest(w, "limit must be a non-negative integer")
			return
		}
		filter.Limit = n
	}

	list, err := s.deps.Events.ListPublic(r.Context(), filter)
	if err != nil {
		internalError(w, s.log(), "list public events", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

// handleGetPublicEvent serves GET /api/events/{slugOrID} — public, published
// only (enforced by events.Service.GetBySlug per its own doc).
//
// It accepts either a slug or an event ULID. Slug-only lookup meant the
// organiser UI, whose routes are /admin/events/:id, 404'd on every event —
// the whole event editor rendered "Event not found".
func (s *server) handleGetPublicEvent(w http.ResponseWriter, r *http.Request) {
	ref := chi.URLParam(r, "id")
	ev, err := s.deps.Events.GetBySlug(r.Context(), ref)
	if errors.Is(err, store.ErrNotFound) {
		ev, err = s.deps.Events.Get(r.Context(), ref)
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		internalError(w, s.log(), "get public event", err)
		return
	}
	types, err := s.deps.Events.ListTicketTypes(r.Context(), ev.ID)
	if err != nil {
		internalError(w, s.log(), "list ticket types for public event", err)
		return
	}
	ring, err := s.deps.Events.IssuerPublicKeys(r.Context(), ev.ID)
	if err != nil {
		internalError(w, s.log(), "issuer keys for public event", err)
		return
	}
	images, err := s.deps.Store.ListImagesByEvent(r.Context(), ev.ID)
	if err != nil {
		internalError(w, s.log(), "gallery for public event", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":        ev,
		"ticket_types": types,
		"issuer_keys":  ring,
		"gallery":      toImageViews(images),
	})
}

// imageView is one gallery image over the wire.
type imageView struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func toImageViews(images []store.Image) []imageView {
	out := make([]imageView, 0, len(images))
	for _, img := range images {
		out = append(out, imageView{ID: img.ID, URL: "/media/" + img.ID, Width: img.Width, Height: img.Height})
	}
	return out
}

// handleListOrgEvents serves GET /api/orgs/{id}/events — every event
// belonging to the org, ANY status (draft, published, cancelled), most
// recently created first. This is the org-scoped counterpart to GET
// /api/events (which is public and published-only, and must stay that
// way): it is what lets an organiser's own draft events show up in their
// Events list and dashboard instead of vanishing the moment they navigate
// away (drafts have no other visible home — GET /api/events/{slugOrID}
// still resolves one directly by id/slug, but there was previously no
// listing route that included them).
//
// RBAC is scanner+ (any org member), matching the read-only bar already
// used for stats/attendees — every member of the org, not just
// admins/owners, has a legitimate reason to see what events exist.
func (s *server) handleListOrgEvents(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	orgID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, orgID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "list org events rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a member of this org")
		return
	}

	list, err := s.deps.Events.ListByOrg(r.Context(), orgID)
	if err != nil {
		internalError(w, s.log(), "list org events", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

// handleDeleteEvent serves DELETE /api/events/{id} — admin+ on the event's
// org. Refused with 409 conflict if the event has any issued tickets (see
// events.Service.Delete / events.ErrEventHasTickets): once real money and
// tickets exist behind an event, cancelling it (PATCH .../events/{id}
// {"status":"cancelled"}) is the correct move — buyers keep their order
// history and tickets aren't silently orphaned. An event with zero issued
// tickets (a draft nobody has bought into, or a published event that
// simply hasn't sold anything yet) can be deleted outright.
func (s *server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "delete event rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	if err := s.deps.Events.Delete(r.Context(), eventID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		if errors.Is(err, events.ErrEventHasTickets) {
			conflict(w, err.Error())
			return
		}
		internalError(w, s.log(), "delete event", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListCategories serves GET /api/categories — public, derived from
// currently published events only (a category with zero live events isn't
// worth a browse-page tab).
func (s *server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	counts, err := s.deps.Store.ListCategoryCounts(r.Context())
	if err != nil {
		internalError(w, s.log(), "list categories", err)
		return
	}

	type categoryView struct {
		Slug  string `json:"slug"`
		Label string `json:"label"`
		Count int    `json:"count"`
	}
	out := make([]categoryView, 0, len(counts))
	for _, c := range counts {
		out = append(out, categoryView{Slug: c.Slug, Label: categoryLabel(c.Slug), Count: c.Count})
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

// handleListCurrencies serves GET /api/currencies — public, static
// reference data straight from internal/money's ISO-4217 table. This is
// what an event-creation currency picker should source its options from,
// instead of a hardcoded handful of currencies: Cackle has no privileged
// currency, and the frontend shouldn't either.
func (s *server) handleListCurrencies(w http.ResponseWriter, r *http.Request) {
	type currencyView struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Exponent int    `json:"exponent"`
	}
	codes := money.SupportedCurrencies()
	out := make([]currencyView, 0, len(codes))
	for _, code := range codes {
		info, err := money.Lookup(code)
		if err != nil {
			// Every code from SupportedCurrencies() is, by construction,
			// in the same table Lookup reads — this should be
			// unreachable, but fail loudly rather than silently drop a
			// currency from the list if that invariant is ever broken.
			internalError(w, s.log(), "list currencies", err)
			return
		}
		out = append(out, currencyView{Code: info.Code, Name: info.Name, Exponent: info.Exponent})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	writeJSON(w, http.StatusOK, map[string]any{"currencies": out})
}

// categoryLabel reconstructs a human-friendly label from a normalised
// slug: "live-music" -> "Live Music".
func categoryLabel(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// handleCreateEvent serves POST /api/events — requires admin+ on the org
// the event will belong to. org_id is a wrapper field alongside whatever
// events.CreateEventInput itself decodes from the same JSON body.
func (s *server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())

	var body struct {
		OrgID string `json:"org_id"`
		events.CreateEventInput
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if body.OrgID == "" {
		badRequest(w, "org_id is required")
		return
	}

	ok, err := s.deps.Auth.CanManageOrg(r.Context(), user.ID, body.OrgID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "create event rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this org")
		return
	}

	ev, err := s.deps.Events.Create(r.Context(), body.OrgID, body.CreateEventInput)
	if err != nil {
		if errors.Is(err, events.ErrInvalidInput) {
			badRequest(w, err.Error())
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			badRequest(w, "org_id does not exist")
			return
		}
		internalError(w, s.log(), "create event", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"event": ev})
}

// handleUpdateEvent serves PATCH /api/events/{id}. RBAC resolves the
// event's org internally (auth.CanManageEvent), so the handler itself
// never needs to know the org id.
func (s *server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "update event rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	var in events.UpdateEventInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	ev, err := s.deps.Events.Update(r.Context(), eventID, in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		if errors.Is(err, events.ErrInvalidInput) || errors.Is(err, events.ErrInvalidTransition) {
			badRequest(w, err.Error())
			return
		}
		internalError(w, s.log(), "update event", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev})
}

// handlePublishEvent serves POST /api/events/{id}/publish.
func (s *server) handlePublishEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "publish event rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	ev, err := s.deps.Events.Publish(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		if errors.Is(err, events.ErrInvalidTransition) {
			badRequest(w, err.Error())
			return
		}
		internalError(w, s.log(), "publish event", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": ev})
}

// handleEventStats serves GET /api/events/{id}/stats. Any org member
// (scanner and up) may read stats — it's the door/box-office dashboard.
func (s *server) handleEventStats(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleScanner)
	if err != nil {
		internalError(w, s.log(), "event stats rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not a member of this event's org")
		return
	}

	st, err := s.deps.Events.Stats(r.Context(), eventID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "event not found")
			return
		}
		internalError(w, s.log(), "event stats", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": st})
}

// handleListTicketTypes serves GET /api/events/{id}/ticket-types (the
// management view — the public one is embedded in handleGetPublicEvent).
func (s *server) handleListTicketTypes(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "list ticket types rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	types, err := s.deps.Events.ListTicketTypes(r.Context(), eventID)
	if err != nil {
		internalError(w, s.log(), "list ticket types", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket_types": types})
}

// handleCreateTicketType serves POST /api/events/{id}/ticket-types.
func (s *server) handleCreateTicketType(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "create ticket type rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	var in events.TicketTypeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	tt, err := s.deps.Events.CreateTicketType(r.Context(), eventID, in)
	if err != nil {
		internalError(w, s.log(), "create ticket type", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ticket_type": tt})
}

// handleUpdateTicketType serves PATCH /api/ticket-types/{id}. The route is
// flat (no event id in the path), so RBAC first resolves which event this
// ticket type belongs to via a narrow read against the store — see
// dbutil.go's ticketTypeEventID.
func (s *server) handleUpdateTicketType(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	ttID := chi.URLParam(r, "id")

	eventID, err := ticketTypeEventID(r.Context(), s.deps.Store.DB(), ttID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "ticket type not found")
			return
		}
		internalError(w, s.log(), "update ticket type lookup", err)
		return
	}

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "update ticket type rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	var in events.TicketTypeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	tt, err := s.deps.Events.UpdateTicketType(r.Context(), ttID, in)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "ticket type not found")
			return
		}
		internalError(w, s.log(), "update ticket type", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket_type": tt})
}

// handleDeleteTicketType serves DELETE /api/ticket-types/{id}.
func (s *server) handleDeleteTicketType(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	ttID := chi.URLParam(r, "id")

	eventID, err := ticketTypeEventID(r.Context(), s.deps.Store.DB(), ttID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "ticket type not found")
			return
		}
		internalError(w, s.log(), "delete ticket type lookup", err)
		return
	}

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "delete ticket type rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	if err := s.deps.Events.DeleteTicketType(r.Context(), ttID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "ticket type not found")
			return
		}
		internalError(w, s.log(), "delete ticket type", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
