package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Image is one uploaded, validated, metadata-stripped image belonging to an
// event's gallery (see internal/media for the validation/stripping and
// internal/httpapi's image handlers for the HTTP surface). Format is one of
// "png", "jpeg", "webp" (enforced by a CHECK constraint) and, together with
// ID, is exactly enough to reconstruct the on-disk path
// (CACKLE_MEDIA_DIR/<id><ext>) — the row never stores a path or filename
// itself, so there is nothing here a client could ever have influenced.
type Image struct {
	ID         string
	EventID    string
	Format     string
	Width      int
	Height     int
	SizeBytes  int64
	UploadedBy *string
	CreatedAt  time.Time
}

// CreateImage inserts a new image row. If ID or CreatedAt are zero they are
// populated.
func (s *Store) CreateImage(ctx context.Context, img *Image) error {
	if img.ID == "" {
		img.ID = NewID()
	}
	if img.CreatedAt.IsZero() {
		img.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO images (id, event_id, format, width, height, size_bytes, uploaded_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		img.ID, img.EventID, img.Format, img.Width, img.Height, img.SizeBytes,
		nullString(img.UploadedBy), timeToText(img.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("store: create image: %w", err)
	}
	return nil
}

// GetImageByID looks up an image by primary key. Returns ErrNotFound if
// absent.
func (s *Store) GetImageByID(ctx context.Context, id string) (*Image, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, event_id, format, width, height, size_bytes, uploaded_by, created_at
		FROM images WHERE id = ?`, id)
	return scanImage(row)
}

// ImageEventID resolves the event_id an image belongs to, so a handler can
// run RBAC (auth.CanManageEvent) before allowing a mutation on an image
// that the URL only identifies by its own id — the same pattern as
// internal/httpapi's ticketTypeEventID. Returns ErrNotFound if absent.
func (s *Store) ImageEventID(ctx context.Context, imageID string) (string, error) {
	var eventID string
	err := s.db.QueryRowContext(ctx, `SELECT event_id FROM images WHERE id = ?`, imageID).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: image event id: %w", err)
	}
	return eventID, nil
}

// DeleteImage removes an image row. events.cover_image_id referencing this
// row is cleared automatically by the schema's ON DELETE SET NULL — callers
// never need a second write for that. Returns ErrNotFound if absent.
func (s *Store) DeleteImage(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM images WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete image: %w", err)
	}
	return rowsAffectedOrNotFound(res)
}

// ListImagesByEvent returns every image belonging to an event (its
// gallery), oldest first.
func (s *Store) ListImagesByEvent(ctx context.Context, eventID string) ([]Image, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, format, width, height, size_bytes, uploaded_by, created_at
		FROM images WHERE event_id = ? ORDER BY created_at ASC`, eventID)
	if err != nil {
		return nil, fmt.Errorf("store: list images by event: %w", err)
	}
	defer rows.Close()

	var out []Image
	for rows.Next() {
		img, err := scanImageRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *img)
	}
	return out, rows.Err()
}

func scanImage(row *sql.Row) (*Image, error) {
	var img Image
	var uploadedBy sql.NullString
	var createdAt string
	err := row.Scan(&img.ID, &img.EventID, &img.Format, &img.Width, &img.Height, &img.SizeBytes, &uploadedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan image: %w", err)
	}
	if uploadedBy.Valid {
		img.UploadedBy = &uploadedBy.String
	}
	if img.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse image created_at: %w", err)
	}
	return &img, nil
}

// scanImageRow shares scanning logic with the single-row lookup above,
// using the same rowScanner interface internal/store/ticket_types.go
// already defines for this exact "*sql.Row or *sql.Rows, either way just
// Scan" purpose.
func scanImageRow(rows rowScanner) (*Image, error) {
	var img Image
	var uploadedBy sql.NullString
	var createdAt string
	if err := rows.Scan(&img.ID, &img.EventID, &img.Format, &img.Width, &img.Height, &img.SizeBytes, &uploadedBy, &createdAt); err != nil {
		return nil, fmt.Errorf("store: scan image row: %w", err)
	}
	if uploadedBy.Valid {
		img.UploadedBy = &uploadedBy.String
	}
	var err error
	if img.CreatedAt, err = textToTime(createdAt); err != nil {
		return nil, fmt.Errorf("store: parse image created_at: %w", err)
	}
	return &img, nil
}
