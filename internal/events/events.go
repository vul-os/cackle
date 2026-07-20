// Package events owns Cackle's org/event/ticket-type domain logic: creating
// and publishing events, managing ticket types, computing stats, and —
// critically — minting and holding each event's own Ed25519 issuer key.
//
// Per-event issuer authority is the whole design: every event gets its own
// keypair at creation (see Create), generated and persisted atomically with
// the event row (internal/store.CreateEventWithKey). There is never a
// global signing key. The raw private key never leaves this package —
// internal/orders (and anything else that needs to sign a ticket) must go
// through the exported IssueTicket wrapper rather than reading event_keys
// directly.
package events

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vul-os/cackle/internal/money"
	"github.com/vul-os/cackle/internal/store"
	"github.com/vul-os/cackle/internal/tickets"
)

// Sentinel errors. store.ErrNotFound is returned as-is (not wrapped) for
// "no such event/ticket type" so callers can match it directly with
// errors.Is against the one shared not-found sentinel.
var (
	// ErrInvalidInput covers basic validation failures on Create/Update/
	// CreateTicketType/UpdateTicketType input (empty required field,
	// starts_at not before ends_at, negative price/quantity, etc).
	ErrInvalidInput = errors.New("events: invalid input")
	// ErrInvalidTransition is returned by Publish when the event's current
	// status cannot move to "published" (only draft -> published is
	// allowed; a cancelled event cannot be published), and by Update when
	// asked to set a status other than "cancelled" directly (publishing
	// must go through Publish).
	ErrInvalidTransition = errors.New("events: invalid status transition")
	// ErrQuantityBelowSold is returned by UpdateTicketType when the new
	// QuantityTotal would be less than the number of units already sold.
	ErrQuantityBelowSold = errors.New("events: quantity_total cannot be less than quantity already sold")
	// ErrTicketTypeHasSales is returned by DeleteTicketType when the
	// ticket type has already reserved/sold inventory (quantity_sold > 0).
	ErrTicketTypeHasSales = errors.New("events: cannot delete a ticket type with sold or reserved tickets")
	// ErrEventHasTickets is returned by Delete when at least one ticket has
	// ever been issued for the event (i.e. at least one order settled —
	// see internal/store.SettleOrder). Hard-deleting would either orphan
	// those tickets/orders or cascade-delete real buyers' purchase
	// history; cancelling the event (Update with Status "cancelled") is
	// the correct move once real money/tickets exist behind it.
	ErrEventHasTickets = errors.New("events: cannot delete an event with issued tickets; cancel it instead")
)

// Event is a ticketed event owned by an org.
type Event struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	VenueName   string    `json:"venue_name"`
	Address     string    `json:"address"`
	Lat         *float64  `json:"lat,omitempty"`
	Lng         *float64  `json:"lng,omitempty"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Timezone    string    `json:"timezone"`
	CoverImage  string    `json:"cover_image"`
	Status      string    `json:"status"` // "draft", "published", "cancelled"
	Currency    string    `json:"currency"`
	// Category is a normalised slug ("live-music", "sports", ...); see
	// normalizeCategory. Empty means uncategorised.
	Category string `json:"category"`
	// CoverImageID is the id of an image in this event's own gallery
	// chosen as its cover, or nil if none has been chosen.
	CoverImageID *string   `json:"cover_image_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateEventInput is the input to Create. Status always starts "draft";
// use Publish to move it to "published".
type CreateEventInput struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	VenueName   string    `json:"venue_name"`
	Address     string    `json:"address"`
	Lat         *float64  `json:"lat,omitempty"`
	Lng         *float64  `json:"lng,omitempty"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Timezone    string    `json:"timezone"`
	CoverImage  string    `json:"cover_image"`
	Currency    string    `json:"currency"`
	Category    string    `json:"category"`
}

// UpdateEventInput is the input to Update. Every field is a pointer;
// nil means "leave unchanged". Status may only be set to "cancelled" here
// — moving to "published" must go through Publish, which enforces the
// draft -> published gate.
type UpdateEventInput struct {
	Slug        *string    `json:"slug,omitempty"`
	Title       *string    `json:"title,omitempty"`
	Summary     *string    `json:"summary,omitempty"`
	Description *string    `json:"description,omitempty"`
	VenueName   *string    `json:"venue_name,omitempty"`
	Address     *string    `json:"address,omitempty"`
	Lat         *float64   `json:"lat,omitempty"`
	Lng         *float64   `json:"lng,omitempty"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Timezone    *string    `json:"timezone,omitempty"`
	CoverImage  *string    `json:"cover_image,omitempty"`
	Currency    *string    `json:"currency,omitempty"`
	Status      *string    `json:"status,omitempty"`
	Category    *string    `json:"category,omitempty"`
	// CoverImageID sets the event's chosen gallery cover image. A non-nil
	// pointer to an empty string clears the cover (sets it to nil); a
	// non-nil pointer to a non-empty id sets it, after verifying that
	// image belongs to THIS event (ErrInvalidInput otherwise — an org
	// cannot point an event's cover at another event's, or another org's,
	// image).
	CoverImageID *string `json:"cover_image_id,omitempty"`
}

// PublicFilter narrows ListPublic's results. From/To use the zero
// time.Time value to mean "no bound" (rather than a pointer) — a
// handler parsing optional ?from=&to= query params can assign straight
// into these fields.
type PublicFilter struct {
	Query    string    `json:"q"`
	Category string    `json:"category"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Limit    int       `json:"limit"`
}

// TicketTypeStats is one ticket type's paid-sales figures within Stats.
type TicketTypeStats struct {
	TicketTypeID  string `json:"ticket_type_id"`
	Name          string `json:"name"`
	QuantityTotal int    `json:"quantity_total"`
	Sold          int    `json:"sold"`
	RevenueMinor  int64  `json:"revenue_minor"`
}

// Stats is an event's sales/attendance summary.
type Stats struct {
	Sold         int               `json:"sold"`
	RevenueMinor int64             `json:"revenue_minor"`
	Admitted     int               `json:"admitted"`
	ByType       []TicketTypeStats `json:"by_type"`
}

// Service is the entrypoint for all event/ticket-type operations.
type Service struct {
	store *store.Store
}

// New constructs an events Service backed by st.
func New(st *store.Store) *Service {
	return &Service{store: st}
}

// Create creates a new draft event for orgID and generates its own,
// dedicated Ed25519 issuer keypair — atomically, in the same database
// transaction as the event row itself (internal/store.CreateEventWithKey),
// so an event can never exist even momentarily without a signing key.
func (s *Service) Create(ctx context.Context, orgID string, in CreateEventInput) (*Event, error) {
	if strings.TrimSpace(orgID) == "" {
		return nil, fmt.Errorf("%w: org id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Slug) == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		return nil, fmt.Errorf("%w: starts_at and ends_at are required", ErrInvalidInput)
	}
	if !in.EndsAt.After(in.StartsAt) {
		return nil, fmt.Errorf("%w: ends_at must be after starts_at", ErrInvalidInput)
	}

	org, err := s.store.GetOrgByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("%w: org %s", store.ErrNotFound, orgID)
		}
		return nil, fmt.Errorf("events: create: look up org: %w", err)
	}

	// Currency is per-event, defaulting from the org (never a hardcoded
	// literal — Cackle has no privileged currency). Either way it is
	// validated/normalized via internal/money at this edge, so an event
	// can never be created with an empty or unrecognised currency code.
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = org.DefaultCurrency
	}
	currency, err = money.Normalize(currency)
	if err != nil {
		return nil, fmt.Errorf("%w: currency: %v", ErrInvalidInput, err)
	}

	now := time.Now()
	e := &store.Event{
		ID:          store.NewID(),
		OrgID:       orgID,
		Slug:        in.Slug,
		Title:       in.Title,
		Summary:     in.Summary,
		Description: in.Description,
		VenueName:   in.VenueName,
		Address:     in.Address,
		Lat:         in.Lat,
		Lng:         in.Lng,
		StartsAt:    in.StartsAt,
		EndsAt:      in.EndsAt,
		Timezone:    defaultStr(in.Timezone, "UTC"),
		CoverImage:  in.CoverImage,
		Status:      "draft",
		Currency:    currency,
		Category:    normalizeCategory(in.Category),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	ik, err := tickets.GenerateIssuerKey(e.ID)
	if err != nil {
		return nil, fmt.Errorf("events: create: generate issuer key: %w", err)
	}
	ek := &store.EventKey{
		ID:         store.NewID(),
		EventID:    e.ID,
		PublicKey:  ik.PublicKey,
		PrivateKey: ik.PrivateKey,
		CreatedAt:  ik.CreatedAt,
	}

	if err := s.store.CreateEventWithKey(ctx, e, ek); err != nil {
		return nil, fmt.Errorf("events: create: %w", err)
	}

	out := toEvent(e)
	return &out, nil
}

// Update applies a partial update to an existing event. Status may only be
// set to "cancelled" via this method — moving to "published" must go
// through Publish.
func (s *Service) Update(ctx context.Context, eventID string, in UpdateEventInput) (*Event, error) {
	e, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if in.Slug != nil {
		if strings.TrimSpace(*in.Slug) == "" {
			return nil, fmt.Errorf("%w: slug cannot be empty", ErrInvalidInput)
		}
		e.Slug = *in.Slug
	}
	if in.Title != nil {
		if strings.TrimSpace(*in.Title) == "" {
			return nil, fmt.Errorf("%w: title cannot be empty", ErrInvalidInput)
		}
		e.Title = *in.Title
	}
	if in.Summary != nil {
		e.Summary = *in.Summary
	}
	if in.Description != nil {
		e.Description = *in.Description
	}
	if in.VenueName != nil {
		e.VenueName = *in.VenueName
	}
	if in.Address != nil {
		e.Address = *in.Address
	}
	if in.Lat != nil {
		e.Lat = in.Lat
	}
	if in.Lng != nil {
		e.Lng = in.Lng
	}
	if in.StartsAt != nil {
		e.StartsAt = *in.StartsAt
	}
	if in.EndsAt != nil {
		e.EndsAt = *in.EndsAt
	}
	if in.Timezone != nil {
		e.Timezone = *in.Timezone
	}
	if in.CoverImage != nil {
		e.CoverImage = *in.CoverImage
	}
	if in.Currency != nil {
		norm, err := money.Normalize(*in.Currency)
		if err != nil {
			return nil, fmt.Errorf("%w: currency: %v", ErrInvalidInput, err)
		}
		e.Currency = norm
	}
	if in.Category != nil {
		e.Category = normalizeCategory(*in.Category)
	}
	if in.CoverImageID != nil {
		if *in.CoverImageID == "" {
			e.CoverImageID = nil
		} else {
			imgEventID, err := s.store.ImageEventID(ctx, *in.CoverImageID)
			if errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("%w: cover_image_id does not exist", ErrInvalidInput)
			}
			if err != nil {
				return nil, fmt.Errorf("events: update: look up cover image: %w", err)
			}
			if imgEventID != eventID {
				return nil, fmt.Errorf("%w: cover_image_id must belong to this event", ErrInvalidInput)
			}
			id := *in.CoverImageID
			e.CoverImageID = &id
		}
	}
	if in.Status != nil {
		if *in.Status != "cancelled" {
			return nil, fmt.Errorf("%w: use Publish to publish an event; Update may only set status to \"cancelled\"", ErrInvalidTransition)
		}
		e.Status = "cancelled"
	}

	if !e.EndsAt.After(e.StartsAt) {
		return nil, fmt.Errorf("%w: ends_at must be after starts_at", ErrInvalidInput)
	}

	e.UpdatedAt = time.Now()
	if err := s.store.UpdateEvent(ctx, e); err != nil {
		return nil, fmt.Errorf("events: update: %w", err)
	}

	out := toEvent(e)
	return &out, nil
}

// Publish moves a draft event to published. Publishing an already-
// published event is idempotent (returns it unchanged, no error).
// Publishing a cancelled event is rejected with ErrInvalidTransition.
func (s *Service) Publish(ctx context.Context, eventID string) (*Event, error) {
	e, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	switch e.Status {
	case "published":
		out := toEvent(e)
		return &out, nil
	case "draft":
		now := time.Now()
		if err := s.store.SetEventStatus(ctx, eventID, "published", now); err != nil {
			return nil, fmt.Errorf("events: publish: %w", err)
		}
		e.Status = "published"
		e.UpdatedAt = now
		out := toEvent(e)
		return &out, nil
	default:
		return nil, fmt.Errorf("%w: cannot publish an event with status %q", ErrInvalidTransition, e.Status)
	}
}

// Delete removes an event outright. Refused with ErrEventHasTickets if any
// ticket has ever been issued against it (regardless of that ticket's
// current status — valid, void, or refunded all count: a refund doesn't
// erase the fact that a real sale and admission history exist) — cancel
// the event instead (Update with Status "cancelled") once that's true.
// Returns store.ErrNotFound if the event does not exist.
func (s *Service) Delete(ctx context.Context, eventID string) error {
	if _, err := s.store.GetEventByID(ctx, eventID); err != nil {
		return err
	}
	n, err := s.store.CountTicketsForEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("events: delete: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w: %d ticket(s) issued", ErrEventHasTickets, n)
	}
	if err := s.store.DeleteEvent(ctx, eventID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return err
		}
		return fmt.Errorf("events: delete: %w", err)
	}
	return nil
}

// Get looks up an event by ID. Returns store.ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, eventID string) (*Event, error) {
	e, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := toEvent(e)
	return &out, nil
}

// GetBySlug looks up an event by its unique slug. Returns store.ErrNotFound
// if absent.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*Event, error) {
	e, err := s.store.GetEventBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	out := toEvent(e)
	return &out, nil
}

// ListPublic returns published events matching f, most imminent first.
func (s *Service) ListPublic(ctx context.Context, f PublicFilter) ([]Event, error) {
	var from, to *time.Time
	if !f.From.IsZero() {
		from = &f.From
	}
	if !f.To.IsZero() {
		to = &f.To
	}
	rows, err := s.store.ListPublishedEvents(ctx, f.Query, normalizeCategory(f.Category), from, to, f.Limit)
	if err != nil {
		return nil, err
	}
	return toEvents(rows), nil
}

// ListByOrg returns every event (any status) belonging to an org, most
// recently created first.
func (s *Service) ListByOrg(ctx context.Context, orgID string) ([]Event, error) {
	rows, err := s.store.ListEventsByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return toEvents(rows), nil
}

// Stats computes an event's sales/attendance summary. Sold and RevenueMinor
// are derived from order_items on PAID orders only (never from the
// quantity_sold reservation counter, which also includes still-pending
// unpaid reservations and exists purely to prevent oversell, not to report
// revenue).
func (s *Service) Stats(ctx context.Context, eventID string) (*Stats, error) {
	if _, err := s.store.GetEventByID(ctx, eventID); err != nil {
		return nil, err
	}

	rows, err := s.store.TicketTypeStatsForEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("events: stats: %w", err)
	}
	admitted, err := s.store.CountAdmittedForEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("events: stats: %w", err)
	}

	stats := &Stats{Admitted: admitted, ByType: make([]TicketTypeStats, 0, len(rows))}
	for _, r := range rows {
		stats.Sold += r.Sold
		stats.RevenueMinor += r.RevenueMinor
		stats.ByType = append(stats.ByType, TicketTypeStats{
			TicketTypeID:  r.TicketTypeID,
			Name:          r.Name,
			QuantityTotal: r.QuantityTotal,
			Sold:          r.Sold,
			RevenueMinor:  r.RevenueMinor,
		})
	}
	return stats, nil
}

// IssuerPublicKeys returns the KeyRing of every currently non-revoked
// issuer public key for eventID — what a scanner pins for offline
// verification (see internal/tickets and internal/scan). Only public
// halves ever leave this package via this method.
func (s *Service) IssuerPublicKeys(ctx context.Context, eventID string) (tickets.KeyRing, error) {
	keys, err := s.store.ActiveEventKeys(ctx, eventID)
	if err != nil {
		return tickets.KeyRing{}, fmt.Errorf("events: issuer public keys: %w", err)
	}
	ring := tickets.NewKeyRing(eventID)
	for _, k := range keys {
		ring.Add(tickets.KeyID(k.PublicKey), k.PublicKey)
	}
	return ring, nil
}

// signingKey returns the event's current active issuer key id and private
// key, for signing a new ticket. It is unexported: the raw private key
// never leaves this package. Callers outside this package that need a
// ticket signed must go through IssueTicket instead.
func (s *Service) signingKey(ctx context.Context, eventID string) (kid string, priv ed25519.PrivateKey, err error) {
	k, err := s.store.LatestActiveEventKey(ctx, eventID)
	if err != nil {
		return "", nil, fmt.Errorf("events: signing key: %w", err)
	}
	return tickets.KeyID(k.PublicKey), k.PrivateKey, nil
}

// IssueTicket signs payload with the event's own current active issuer
// key and returns the compact capability token plus the key id it was
// signed with. This is the only way any other package (in particular
// internal/orders, at Settle time) may obtain a signed ticket capability —
// the raw ed25519.PrivateKey itself is never returned to a caller outside
// this package.
func (s *Service) IssueTicket(ctx context.Context, eventID string, payload tickets.Payload) (capability string, kid string, err error) {
	kid, priv, err := s.signingKey(ctx, eventID)
	if err != nil {
		return "", "", err
	}
	payload.KID = kid
	cap, err := tickets.Issue(payload, priv)
	if err != nil {
		return "", "", fmt.Errorf("events: issue ticket: %w", err)
	}
	return cap, kid, nil
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// normalizeCategory folds free-text category input down to a stable,
// URL/query-string-safe slug: lower-cased, non-alphanumeric runs collapsed
// to a single hyphen, leading/trailing hyphens trimmed. "Live Music!" and
// "  live-music  " both normalise to "live-music", so GET
// /api/events?category=live-music matches events however their category
// was originally typed. Empty input normalises to "".
func normalizeCategory(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	var b strings.Builder
	b.Grow(len(c))
	prevHyphen := false
	for _, r := range c {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func toEvent(e *store.Event) Event {
	return Event{
		ID:           e.ID,
		OrgID:        e.OrgID,
		Slug:         e.Slug,
		Title:        e.Title,
		Summary:      e.Summary,
		Description:  e.Description,
		VenueName:    e.VenueName,
		Address:      e.Address,
		Lat:          e.Lat,
		Lng:          e.Lng,
		StartsAt:     e.StartsAt,
		EndsAt:       e.EndsAt,
		Timezone:     e.Timezone,
		CoverImage:   e.CoverImage,
		Status:       e.Status,
		Currency:     e.Currency,
		Category:     e.Category,
		CoverImageID: e.CoverImageID,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

func toEvents(rows []store.Event) []Event {
	out := make([]Event, len(rows))
	for i := range rows {
		out[i] = toEvent(&rows[i])
	}
	return out
}
