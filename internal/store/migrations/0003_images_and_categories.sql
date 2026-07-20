-- 0003_images_and_categories.sql — event gallery/cover images + event
-- categories (wave 3 backend contract). Images are stored on disk beside
-- the SQLite database (CACKLE_MEDIA_DIR); this table is only ever the
-- metadata row — see internal/media and internal/httpapi's image handlers.
--
-- images.id is a ULID generated server-side (internal/store.NewID),
-- NEVER derived from a client-supplied filename — that id is also the
-- on-disk filename stem, so a client can never influence the storage
-- path.

CREATE TABLE images (
    id          TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    format      TEXT NOT NULL CHECK (format IN ('png', 'jpeg', 'webp')),
    width       INTEGER NOT NULL,
    height      INTEGER NOT NULL,
    size_bytes  INTEGER NOT NULL,
    uploaded_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL
);
CREATE INDEX idx_images_event_id ON images(event_id);

ALTER TABLE events ADD COLUMN category TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_events_category ON events(category);

-- cover_image_id is nullable and clears itself automatically (ON DELETE
-- SET NULL) if the referenced image is ever deleted — see
-- internal/httpapi's handleDeleteImage, which relies on this rather than
-- doing a second write to clear a dangling reference.
ALTER TABLE events ADD COLUMN cover_image_id TEXT REFERENCES images(id) ON DELETE SET NULL;
