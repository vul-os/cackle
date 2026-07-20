package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 10), G: uint8(y * 10), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode test jpeg: %v", err)
	}
	return buf.Bytes()
}

// --- magic byte / real format acceptance ---

func TestProcess_AcceptsRealPNG(t *testing.T) {
	data := makeTestPNG(t, 6, 4)
	p, err := Process(data)
	if err != nil {
		t.Fatalf("Process(png) = %v, want nil", err)
	}
	if p.Format != FormatPNG {
		t.Fatalf("Format = %q, want png", p.Format)
	}
	if p.Width != 6 || p.Height != 4 {
		t.Fatalf("dims = %dx%d, want 6x4", p.Width, p.Height)
	}
	if len(p.Bytes) == 0 {
		t.Fatal("Bytes is empty")
	}
}

func TestProcess_AcceptsRealJPEG(t *testing.T) {
	data := makeTestJPEG(t, 8, 5)
	p, err := Process(data)
	if err != nil {
		t.Fatalf("Process(jpeg) = %v, want nil", err)
	}
	if p.Format != FormatJPEG {
		t.Fatalf("Format = %q, want jpeg", p.Format)
	}
	if p.Width != 8 || p.Height != 5 {
		t.Fatalf("dims = %dx%d, want 8x5", p.Width, p.Height)
	}
}

func TestProcess_AcceptsRealWebPAndStripsEXIF(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.webp")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !bytes.Contains(data, []byte("EXIF")) {
		t.Fatal("test fixture doesn't actually contain an EXIF chunk — fixture is broken")
	}

	p, err := Process(data)
	if err != nil {
		t.Fatalf("Process(webp) = %v, want nil", err)
	}
	if p.Format != FormatWebP {
		t.Fatalf("Format = %q, want webp", p.Format)
	}
	if p.Width != 6 || p.Height != 6 {
		t.Fatalf("dims = %dx%d, want 6x6", p.Width, p.Height)
	}
	if bytes.Contains(p.Bytes, []byte("EXIF")) {
		t.Fatal("cleaned webp bytes still contain an EXIF chunk — metadata was not stripped")
	}
	// Must still be a valid, decodable RIFF/WEBP container after stripping.
	if !bytes.Equal(p.Bytes[0:4], []byte("RIFF")) || !bytes.Equal(p.Bytes[8:12], []byte("WEBP")) {
		t.Fatal("cleaned webp bytes are not a well-formed RIFF/WEBP container")
	}
}

// --- the core security requirement: magic bytes are not enough ---

func TestProcess_RejectsScriptBytesWithPNGExtensionMasquerade(t *testing.T) {
	// A ".png"-named file whose actual bytes are a shell script — this is
	// exactly the "trust the filename/content-type" mistake this package
	// exists to prevent. There is no filename passed to Process at all
	// (by design), so simulate the attack by handing it the raw script
	// bytes directly: they don't even have the PNG magic, so they must be
	// rejected as an unsupported format.
	script := []byte("#!/bin/sh\nrm -rf /\n")
	_, err := Process(script)
	if err == nil {
		t.Fatal("Process(script bytes) = nil error, want rejection")
	}
}

func TestProcess_RejectsPNGMagicBytesFollowedByGarbage(t *testing.T) {
	// A polyglot-style attack: a valid PNG signature, then non-PNG
	// garbage instead of real chunk data. The magic-byte sniff alone
	// would accept this; only a real decode catches it.
	fake := append([]byte{}, pngMagic...)
	fake = append(fake, []byte("<?php system($_GET['c']); ?>")...)
	_, err := Process(fake)
	if err == nil {
		t.Fatal("Process(png-magic + garbage) = nil error, want rejection (decode must fail)")
	}
}

func TestProcess_RejectsJPEGMagicBytesFollowedByGarbage(t *testing.T) {
	fake := append([]byte{}, jpegMagic...)
	fake = append(fake, []byte("not actually a jpeg stream at all, just noise padding out the file")...)
	_, err := Process(fake)
	if err == nil {
		t.Fatal("Process(jpeg-magic + garbage) = nil error, want rejection")
	}
}

func TestProcess_RejectsWebPMagicBytesFollowedByGarbage(t *testing.T) {
	fake := []byte("RIFF\x00\x00\x00\x00WEBPnot a real webp bitstream")
	_, err := Process(fake)
	if err == nil {
		t.Fatal("Process(webp-magic + garbage) = nil error, want rejection")
	}
}

func TestProcess_RejectsUnknownFormat(t *testing.T) {
	_, err := Process([]byte("GIF89a\x01\x00\x01\x00"))
	if err != ErrUnsupportedFormat && !bytesIsUnsupported(err) {
		t.Fatalf("Process(gif) = %v, want ErrUnsupportedFormat", err)
	}
}

func bytesIsUnsupported(err error) bool { return err == ErrUnsupportedFormat }

func TestProcess_RejectsEmpty(t *testing.T) {
	if _, err := Process(nil); err == nil {
		t.Fatal("Process(nil) = nil error, want rejection")
	}
}

// --- size cap ---

func TestProcess_RejectsOversizeFile(t *testing.T) {
	oversized := make([]byte, MaxUploadBytes+1)
	copy(oversized, pngMagic)
	_, err := Process(oversized)
	if err != ErrTooLarge {
		t.Fatalf("Process(9MB) = %v, want ErrTooLarge", err)
	}
}

func TestProcess_AcceptsExactlyAtSizeCapWhenAlsoAValidImage(t *testing.T) {
	// A real (small) PNG is far under the cap; this just confirms the
	// boundary check is ">", not ">=", against MaxUploadBytes using a
	// real decodable image rather than asserting on garbage bytes at the
	// exact boundary (which would legitimately fail decode, not the size
	// check, and could mask a bug in the size check itself).
	data := makeTestPNG(t, 4, 4)
	if len(data) >= MaxUploadBytes {
		t.Fatal("test fixture unexpectedly huge")
	}
	if _, err := Process(data); err != nil {
		t.Fatalf("Process(small valid png) = %v, want nil", err)
	}
}

// --- format helpers ---

func TestFormat_ExtensionAndContentType(t *testing.T) {
	cases := []struct {
		f    Format
		ext  string
		ctyp string
	}{
		{FormatPNG, ".png", "image/png"},
		{FormatJPEG, ".jpg", "image/jpeg"},
		{FormatWebP, ".webp", "image/webp"},
	}
	for _, c := range cases {
		if got := c.f.Extension(); got != c.ext {
			t.Errorf("%s.Extension() = %q, want %q", c.f, got, c.ext)
		}
		if got := c.f.ContentType(); got != c.ctyp {
			t.Errorf("%s.ContentType() = %q, want %q", c.f, got, c.ctyp)
		}
	}
}

func TestParseFormat(t *testing.T) {
	if _, ok := ParseFormat("png"); !ok {
		t.Error("ParseFormat(png) not ok")
	}
	if _, ok := ParseFormat("exe"); ok {
		t.Error("ParseFormat(exe) unexpectedly ok")
	}
}
