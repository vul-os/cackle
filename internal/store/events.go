package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Event is a ticketed event owned by an org. Status is one of "draft",
// "published", "cancelled" (enforced by a CHECK constraint).
type Event struct {
	ID          string
	OrgID       string
	Slug        string
	Title       string
	Summary     string
	Description string
	VenueName   string
	Address     string
	Lat         *float64
	Lng         *float64
	StartsAt    time.Time
	EndsAt      time.Time
	Timezone    string
	CoverImage  string
	Status      string
	Currency    string
	// Category is a free-text, normalised-to-slug classifier ("live-music",
	// "sports", ...) used for GET /api/events?category= filtering and
	// derived by GET /api/categories. Empty string means uncategorised.
	Category string
	// CoverImageID references images(id) — the event's chosen cover photo
	// from its own gallery. Nil means no cover image chosen (falls back to
	// the legacy CoverImage path field in the UI). Cleared automatically
	// (ON DELETE SET NULL) if the referenced image is deleted.
	CoverImageID *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreateEventWithKey inserts a new event AND its (single, freshly
// generated) Ed25519 issuer key in the same transaction. This is the only
// way an event should ever be created: per-event issuer authority is the
// whole point of the product, so an event must never exist without a
// signing key, even for an instant.
func (s *Store) CreateEventWithKey(ctx context.Context, e *Event, k *EventKey) error {
	if e.ID == "" {
		e.ID = NewID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = e.CreatedAt
	}
	if k.ID == "" {
		k.ID = NewID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now()
	}
	k.EventID = e.ID

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create event: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (id, org_id, slug, title, summary, description, venue_name, address,
			lat, lng, starts_at, ends_at, timezone, cover_image, status, currency, category, cover_image_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.OrgID, e.Slug, e.Title, e.Summary, e.Description, e.VenueName, e.Address,
		nullFloat(e.Lat), nullFloat(e.Lng), timeToText(e.StartsAt), timeToText(e.EndsAt),
		e.Timezone, e.CoverImage, e.Status, e.Currency, e.Category, nullString(e.CoverImageID),
		timeToText(e.CreatedAt), timeToText(e.UpdatedAt),
	); err != nil {
		return fmt.Errorf("store: create event: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_keys (id, event_id, public_key, private_key, created_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.EventID, []byte(k.PublicKey), []byte(k.PrivateKey), timeToText(k.CreatedAt), nullTimeToText(k.RevokedAt),
	); err != nil {
		return fmt.Errorf("store: create event issuer key: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: create event: commit: %w", err)
	}
	return nil
}

// UpdateEvent replaces every mutable column of an existing event by ID.
// Callers should read-modify-write via GetEventByID first.
func (s *Store) UpdateEvent(ctx context.Context, e *Event) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE events SET
			slug = ?, title = ?, summary = ?, description = ?, venue_name = ?, address = ?,
			lat = ?, lng = ?, starts_at = ?, ends_at = ?, timezone = ?, cover_image = ?,
			status = ?, currency = ?, category = ?, cover_image_id = ?, updated_at = ?
		WHERE id = ?`,
		e.Slug, e.Title, e.Summary, e.Description, e.VenueName, e.Address,
		nullFloat(e.Lat), nullFloat(e.Lng), timeToText(e.StartsAt), timeToText(e.EndsAt),
		e.Timezone, e.CoverImage, e.Status, e.Currency, e.Category, nullString(e.CoverImageID),
		timeToText(e.UpdatedAt), e.ID,
	)
	if err != nil {
		return fmt.Errorf("store: update event: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// SetEventStatus transitions an event's status column directly (used by
// Publish, and for administrative cancellation) without touching any other
// field.
func (s *Store) SetEventStatus(ctx context.Context, id, status string, updatedAt time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE events SET status = ?, updated_at = ? WHERE id = ?`,
		status, timeToText(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: set event status: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// GetEventByID looks up an event by primary key. Returns ErrNotFound if
// absent.
func (s *Store) GetEventByID(ctx context.Context, id string) (*Event, error) {
	return s.scanEvent(s.db.QueryRowContext(ctx, eventSelectColumns+` FROM events WHERE id = ?`, id))
}

// GetEventBySlug looks up an event by its unique slug. Returns ErrNotFound
// if absent.
func (s *Store) GetEventBySlug(ctx context.Context, slug string) (*Event, error) {
	return s.scanEvent(s.db.QueryRowContext(ctx, eventSelectColumns+` FROM events WHERE slug = ?`, slug))
}

const eventSelectColumns = `SELECT id, org_id, slug, title, summary, description, venue_name, address,
	lat, lng, starts_at, ends_at, timezone, cover_image, status, currency, category, cover_image_id, created_at, updated_at`

func (s *Store) scanEvent(row *sql.Row) (*Event, error) {
	var e Event
	var lat, lng sql.NullFloat64
	var coverImageID sql.NullString
	var startsAt, endsAt, createdAt, updatedAt string

	err := row.Scan(&e.ID, &e.OrgID, &e.Slug, &e.Title, &e.Summary, &e.Description, &e.VenueName, &e.Address,
		&lat, &lng, &startsAt, &endsAt, &e.Timezone, &e.CoverImage, &e.Status, &e.Currency, &e.Category, &coverImageID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan event: %w", err)
	}

	if lat.Valid {
		v := lat.Float64
		e.Lat = &v
	}
	if lng.Valid {
		v := lng.Float64
		e.Lng = &v
	}
	if coverImageID.Valid {
		e.CoverImageID = &coverImageID.String
	}
	if e.StartsAt, err = textToTime(startsAt); err != nil {
		return nil, fmt.Errorf("store: parse event starts_at: %w", err)
	}
	if e.EndsAt, err = textToTime(endsAt); err != nil {
		return nil, fmt.Errorf("store: parse event ends_at: %w", err)
	}
	if e.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse event created_at: %w", err)
	}
	if e.UpdatedAt, err = textToTime(updatedAt); err != nil {
		return nil, fmt.Errorf("store: parse event updated_at: %w", err)
	}
	return &e, nil
}

// ListPublishedEvents returns published events, most recently created
// first, optionally filtered by a case-insensitive substring match against
// title/summary/venue_name, an exact category match, and/or a starts_at
// range. limit <= 0 defaults to 50; it is always capped at 200. Only
// status='published' rows are ever returned — this is enforced here, not
// left to the caller, since this is the public listing route. category ==
// "" means no category filter.
func (s *Store) ListPublishedEvents(ctx context.Context, query, category string, from, to *time.Time, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := eventSelectColumns + ` FROM events WHERE status = 'published'`
	args := []any{}
	if query != "" {
		q += ` AND (title LIKE ? ESCAPE '\' OR summary LIKE ? ESCAPE '\' OR venue_name LIKE ? ESCAPE '\')`
		like := "%" + likeEscape(query) + "%"
		args = append(args, like, like, like)
	}
	if category != "" {
		q += ` AND category = ?`
		args = append(args, category)
	}
	if from != nil {
		q += ` AND starts_at >= ?`
		args = append(args, timeToText(*from))
	}
	if to != nil {
		q += ` AND starts_at <= ?`
		args = append(args, timeToText(*to))
	}
	q += ` ORDER BY starts_at ASC LIMIT ?`
	args = append(args, limit)

	return s.queryEvents(ctx, q, args...)
}

// CategoryCount is one category's slug, display label, and the number of
// currently published events in it — the shape GET /api/categories needs.
type CategoryCount struct {
	Slug  string
	Label string
	Count int
}

// ListCategoryCounts derives the set of categories in use across published
// events (uncategorised events, category = ”, are excluded), most popular
// first. Label is a human-friendly reconstruction of the slug (see
// internal/httpapi's categoryLabel) done at the HTTP layer, not here —
// this method only ever deals in the raw slug + count.
func (s *Store) ListCategoryCounts(ctx context.Context) ([]CategoryCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT category, COUNT(*) AS n
		FROM events
		WHERE status = 'published' AND category != ''
		GROUP BY category
		ORDER BY n DESC, category ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list category counts: %w", err)
	}
	defer rows.Close()

	var out []CategoryCount
	for rows.Next() {
		var c CategoryCount
		if err := rows.Scan(&c.Slug, &c.Count); err != nil {
			return nil, fmt.Errorf("store: scan category count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListEventsByOrg returns every event (any status) belonging to an org,
// most recently created first.
func (s *Store) ListEventsByOrg(ctx context.Context, orgID string) ([]Event, error) {
	return s.queryEvents(ctx, eventSelectColumns+` FROM events WHERE org_id = ? ORDER BY created_at DESC`, orgID)
}

// CountTicketsForEvent returns how many tickets have ever been issued for
// an event (any ticket status — valid, void, or refunded; a ticket only
// ever exists once at all if a paid order settled, see
// internal/store.SettleOrder). This is the check DeleteEvent's caller
// (internal/events.Service.Delete) uses to refuse hard-deleting an event
// that has real issued tickets behind it — cancelling is the correct move
// once money/tickets exist, deleting is only for an event nobody has
// bought into yet.
func (s *Store) CountTicketsForEvent(ctx context.Context, eventID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets WHERE event_id = ?`, eventID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count tickets for event: %w", err)
	}
	return n, nil
}

// DeleteEvent removes an event outright. Callers MUST have already verified
// there are no issued tickets against it (see CountTicketsForEvent) —
// this method itself does not re-check, it trusts internal/events.Delete to
// have done so. Foreign keys cascade: ticket_types, event_keys, payouts,
// and any still-pending (never-paid) orders/order_items for this event are
// removed along with it. Returns ErrNotFound if the event does not exist.
func (s *Store) DeleteEvent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete event: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

func (s *Store) queryEvents(ctx context.Context, query string, args ...any) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list events: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var lat, lng sql.NullFloat64
		var coverImageID sql.NullString
		var startsAt, endsAt, createdAt, updatedAt string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Slug, &e.Title, &e.Summary, &e.Description, &e.VenueName, &e.Address,
			&lat, &lng, &startsAt, &endsAt, &e.Timezone, &e.CoverImage, &e.Status, &e.Currency, &e.Category, &coverImageID, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan event row: %w", err)
		}
		if lat.Valid {
			v := lat.Float64
			e.Lat = &v
		}
		if lng.Valid {
			v := lng.Float64
			e.Lng = &v
		}
		if coverImageID.Valid {
			e.CoverImageID = &coverImageID.String
		}
		if e.StartsAt, err = textToTime(startsAt); err != nil {
			return nil, fmt.Errorf("store: parse event starts_at: %w", err)
		}
		if e.EndsAt, err = textToTime(endsAt); err != nil {
			return nil, fmt.Errorf("store: parse event ends_at: %w", err)
		}
		if e.CreatedAt, err = textToTime(createdAt); err != nil {
			return nil, fmt.Errorf("store: parse event created_at: %w", err)
		}
		if e.UpdatedAt, err = textToTime(updatedAt); err != nil {
			return nil, fmt.Errorf("store: parse event updated_at: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountAdmittedForEvent returns the number of admissions rows recorded with
// result='admitted' for an event — the "admitted" figure in event stats.
func (s *Store) CountAdmittedForEvent(ctx context.Context, eventID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM admissions WHERE event_id = ? AND result = 'admitted'`, eventID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count admitted: %w", err)
	}
	return n, nil
}

// TicketTypeStat is one ticket type's paid-sales figures, as computed by
// TicketTypeStatsForEvent.
type TicketTypeStat struct {
	TicketTypeID  string
	Name          string
	QuantityTotal int
	Sold          int
	RevenueMinor  int64
}

// TicketTypeStatsForEvent computes, per ticket type belonging to eventID,
// how many units were sold and how much revenue was collected — derived
// from order_items joined against PAID orders only (never from the
// quantity_sold reservation counter, which also includes still-pending
// unpaid orders and is an inventory control, not a revenue figure).
func (s *Store) TicketTypeStatsForEvent(ctx context.Context, eventID string) ([]TicketTypeStat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT tt.id, tt.name, tt.quantity_total,
			COALESCE(SUM(CASE WHEN o.status = 'paid' THEN oi.quantity ELSE 0 END), 0) AS sold,
			COALESCE(SUM(CASE WHEN o.status = 'paid' THEN oi.quantity * oi.unit_price_minor ELSE 0 END), 0) AS revenue
		FROM ticket_types tt
		LEFT JOIN order_items oi ON oi.ticket_type_id = tt.id
		LEFT JOIN orders o ON o.id = oi.order_id
		WHERE tt.event_id = ?
		GROUP BY tt.id, tt.name, tt.quantity_total, tt.sort_order
		ORDER BY tt.sort_order ASC, tt.name ASC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: ticket type stats: %w", err)
	}
	defer rows.Close()

	var out []TicketTypeStat
	for rows.Next() {
		var st TicketTypeStat
		if err := rows.Scan(&st.TicketTypeID, &st.Name, &st.QuantityTotal, &st.Sold, &st.RevenueMinor); err != nil {
			return nil, fmt.Errorf("store: scan ticket type stat: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func nullFloat(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

// likeEscape escapes SQLite LIKE wildcards (% and _) plus the escape
// character itself, so a caller-supplied search query is matched literally
// rather than as a pattern.
func likeEscape(s string) string {
	r := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '%', '_', '\\':
			r = append(r, '\\', s[i])
		default:
			r = append(r, s[i])
		}
	}
	return string(r)
}
