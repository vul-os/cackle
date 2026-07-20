package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"

	"github.com/vul-os/cackle/internal/auth"
	"github.com/vul-os/cackle/internal/media"
	"github.com/vul-os/cackle/internal/store"
)

// maxUploadRequestBytes bounds the ENTIRE multipart request body — not
// just the file part — so a caller can't exhaust memory/disk with
// oversized form fields or a runaway body before internal/media ever gets
// a chance to reject it. The slack above media.MaxUploadBytes covers
// multipart boundary/header overhead only.
const maxUploadRequestBytes = media.MaxUploadBytes + (1 << 20) // +1MiB slack

// maxMultipartMemory is how much of the multipart form ParseMultipartForm
// is allowed to buffer in memory before spilling to temp files; kept small
// since the actual file is re-read and bounded again below regardless.
const maxMultipartMemory = 4 << 20

// imagePath builds the on-disk path for a stored image from its server-
// generated id and format — NEVER from anything client-supplied. id is
// always a ULID minted by store.NewID(), so this can never escape
// mediaDir via path traversal.
func imagePath(mediaDir, id string, format media.Format) string {
	return filepath.Join(mediaDir, id+format.Extension())
}

// handleUploadImage serves POST /api/events/{id}/images — admin+ on the
// event's org. This is the highest-risk handler in the wave 3 backend
// contract: the uploaded file's claimed filename and Content-Type are
// NEVER trusted for anything (format, path, or otherwise) — see
// internal/media's package doc for the full validation approach (magic
// bytes, then a real decode, then re-encode/metadata-strip).
func (s *server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	eventID := chi.URLParam(r, "id")

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "upload image rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		badRequest(w, "request body missing or exceeds the maximum upload size")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll() //nolint:errcheck // best-effort temp file cleanup
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		badRequest(w, `multipart field "file" is required`)
		return
	}
	defer file.Close()

	// Read at most MaxUploadBytes+1: enough to detect "too large" as a
	// clean validation error rather than relying solely on
	// MaxBytesReader's harder cutoff (which serves the request-body-wide
	// bound above, not the format-specific one).
	data, err := io.ReadAll(io.LimitReader(file, media.MaxUploadBytes+1))
	if err != nil {
		internalError(w, s.log(), "upload image read", err)
		return
	}

	processed, err := media.Process(data)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrTooLarge):
			badRequest(w, "file exceeds the 8MB maximum upload size")
		case errors.Is(err, media.ErrUnsupportedFormat):
			badRequest(w, "unsupported image format: only png, jpeg, and webp are accepted")
		case errors.Is(err, media.ErrTooManyPixels):
			badRequest(w, "image dimensions are too large")
		default:
			badRequest(w, "file could not be read as a valid image")
		}
		return
	}

	mediaDir := s.deps.mediaDir()
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		internalError(w, s.log(), "upload image mkdir", err)
		return
	}

	id := store.NewID()
	path := imagePath(mediaDir, id, processed.Format)
	if err := os.WriteFile(path, processed.Bytes, 0o600); err != nil {
		internalError(w, s.log(), "upload image write", err)
		return
	}

	img := &store.Image{
		ID:         id,
		EventID:    eventID,
		Format:     string(processed.Format),
		Width:      processed.Width,
		Height:     processed.Height,
		SizeBytes:  int64(len(processed.Bytes)),
		UploadedBy: &user.ID,
	}
	if err := s.deps.Store.CreateImage(r.Context(), img); err != nil {
		_ = os.Remove(path) // don't leave an orphaned file if the DB row fails
		internalError(w, s.log(), "upload image create row", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     img.ID,
		"url":    "/media/" + img.ID,
		"width":  img.Width,
		"height": img.Height,
	})
}

// handleDeleteImage serves DELETE /api/images/{id} — admin+ on the owning
// event's org, resolved via the image's event_id (the route itself only
// identifies the image, mirroring how /api/ticket-types/{id} resolves its
// event via dbutil.go's ticketTypeEventID).
func (s *server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	imageID := chi.URLParam(r, "id")

	eventID, err := s.deps.Store.ImageEventID(r.Context(), imageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "image not found")
			return
		}
		internalError(w, s.log(), "delete image lookup", err)
		return
	}

	ok, err := s.deps.Auth.CanManageEvent(r.Context(), user.ID, eventID, auth.RoleAdmin)
	if err != nil {
		internalError(w, s.log(), "delete image rbac", err)
		return
	}
	if !ok {
		forbidden(w, "you are not an admin/owner of this event's org")
		return
	}

	img, err := s.deps.Store.GetImageByID(r.Context(), imageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "image not found")
			return
		}
		internalError(w, s.log(), "delete image get", err)
		return
	}

	// Delete the DB row first: if this fails, nothing changed. If it
	// succeeds but the subsequent unlink fails, we're left with an
	// orphaned file consuming disk but zero dangling references — the
	// safer failure mode than the reverse order.
	if err := s.deps.Store.DeleteImage(r.Context(), imageID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "image not found")
			return
		}
		internalError(w, s.log(), "delete image row", err)
		return
	}

	format, ok := media.ParseFormat(img.Format)
	if ok {
		path := imagePath(s.deps.mediaDir(), imageID, format)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			s.log().Warn("delete image: failed to remove file from disk", "image_id", imageID, "error", err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleServeMedia serves GET /media/{id} — public, no auth. Uploaded
// event images are not sensitive; this is exactly what an <img src> tag on
// a public event page needs to load directly. Cache-Control is aggressive
// (immutable) because an image id is never reused or mutated in place —
// DELETE removes the row and file outright rather than ever replacing
// bytes at the same id.
func (s *server) handleServeMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	img, err := s.deps.Store.GetImageByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "image not found")
			return
		}
		internalError(w, s.log(), "serve media lookup", err)
		return
	}

	format, ok := media.ParseFormat(img.Format)
	if !ok {
		// The format CHECK constraint should make this unreachable; fail
		// closed rather than guess a content type for unknown stored data.
		internalError(w, s.log(), "serve media", errors.New("image row has unrecognised format"))
		return
	}

	path := imagePath(s.deps.mediaDir(), id, format)
	f, err := os.Open(path)
	if err != nil {
		// The DB row exists but the file doesn't — a data integrity
		// problem, not a client error.
		internalError(w, s.log(), "serve media open file", err)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		internalError(w, s.log(), "serve media stat file", err)
		return
	}

	w.Header().Set("Content-Type", format.ContentType())
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", stat.ModTime(), f)
}
