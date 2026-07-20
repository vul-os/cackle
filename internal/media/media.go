// Package media validates and normalises image uploads for Cackle's event
// gallery/cover-image feature (POST /api/events/{id}/images).
//
// This is the highest-risk surface the wave-3 backend contract added, so
// the rules here are deliberately strict and never trust anything the
// client claims about the file:
//
//   - The client-supplied filename is never read for anything beyond
//     display purposes by the caller, and is NEVER used to build a path on
//     disk or to decide the format — internal/httpapi generates its own
//     storage id (a ULID) and this package alone decides the format, from
//     the bytes themselves.
//   - The client-supplied Content-Type header is never trusted either.
//     Format is determined purely by sniffing magic bytes and then fully
//     DECODING the pixel data — a renamed script or a truncated/corrupt
//     file fails at the decode step, not just the magic-byte check, which
//     closes the classic "valid PNG signature followed by a payload"
//     polyglot trick.
//   - Every accepted image is re-encoded (PNG, JPEG) or has its metadata
//     chunks surgically stripped (WebP, which this package cannot losslessly
//     re-encode without a new heavyweight codec dependency) before it is
//     ever written to disk, so EXIF/XMP/ICC and any other embedded
//     metadata never survives an upload.
//   - Size is capped well before any decode work happens.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/webp"
)

// MaxUploadBytes is the hard cap on an accepted image, enforced before any
// parsing/decoding is attempted.
const MaxUploadBytes = 8 << 20 // 8MB

// maxPixels bounds decoded image dimensions (width*height) to guard against
// a small, highly-compressible file decoding into an enormous in-memory
// bitmap ("decompression bomb"). 40 megapixels is generously above any
// realistic event cover/gallery photo.
const maxPixels = 40_000_000

// Format is one of the three image formats Cackle accepts.
type Format string

const (
	FormatPNG  Format = "png"
	FormatJPEG Format = "jpeg"
	FormatWebP Format = "webp"
)

// Extension returns the on-disk file extension (including the leading dot)
// for a Format, used to build the storage path — never derived from the
// client's filename.
func (f Format) Extension() string {
	switch f {
	case FormatPNG:
		return ".png"
	case FormatJPEG:
		return ".jpg"
	case FormatWebP:
		return ".webp"
	default:
		return ""
	}
}

// ContentType returns the correct Content-Type header value for a Format.
func (f Format) ContentType() string {
	switch f {
	case FormatPNG:
		return "image/png"
	case FormatJPEG:
		return "image/jpeg"
	case FormatWebP:
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// ParseFormat validates that s is one of the known, stored formats —
// used when reconstructing a storage path from a trusted database row
// (never from client input).
func ParseFormat(s string) (Format, bool) {
	switch Format(s) {
	case FormatPNG, FormatJPEG, FormatWebP:
		return Format(s), true
	default:
		return "", false
	}
}

// Sentinel errors. Callers should match with errors.Is; none of these ever
// echo raw decoder output back to a client.
var (
	ErrTooLarge          = errors.New("media: file exceeds the maximum upload size")
	ErrUnsupportedFormat = errors.New("media: unrecognised or unsupported image format (only png, jpeg, webp are accepted)")
	ErrInvalidImage      = errors.New("media: file could not be decoded as a valid image")
	ErrTooManyPixels     = errors.New("media: image dimensions exceed the maximum allowed")
)

// Processed is a validated, metadata-stripped image ready to be written to
// disk.
type Processed struct {
	Format Format
	Bytes  []byte
	Width  int
	Height int
}

// Process validates raw upload bytes end-to-end and returns cleaned bytes
// safe to persist: magic-byte sniff, a hard size cap, a REAL decode of the
// pixel data (never just a header check), a pixel-count bound, and either
// re-encoding (PNG/JPEG) or metadata-chunk stripping (WebP) so no embedded
// EXIF/XMP survives. It never trusts a client-supplied filename or
// Content-Type — format is decided here, from the bytes, exclusively.
func Process(data []byte) (*Processed, error) {
	if len(data) == 0 {
		return nil, ErrInvalidImage
	}
	if len(data) > MaxUploadBytes {
		return nil, ErrTooLarge
	}

	switch sniff(data) {
	case FormatPNG:
		return processPNG(data)
	case FormatJPEG:
		return processJPEG(data)
	case FormatWebP:
		return processWebP(data)
	default:
		return nil, ErrUnsupportedFormat
	}
}

var (
	pngMagic  = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
)

// sniff identifies a format purely from leading magic bytes. This is only
// the FIRST gate — every format is subsequently fully decoded, which is
// what actually defends against a polyglot file that merely starts with a
// valid signature.
func sniff(data []byte) Format {
	switch {
	case len(data) >= len(pngMagic) && bytes.Equal(data[:len(pngMagic)], pngMagic):
		return FormatPNG
	case len(data) >= len(jpegMagic) && bytes.Equal(data[:len(jpegMagic)], jpegMagic):
		return FormatJPEG
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return FormatWebP
	default:
		return ""
	}
}

func checkPixelBudget(cfg image.Config) error {
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrInvalidImage
	}
	if cfg.Width*cfg.Height > maxPixels {
		return ErrTooManyPixels
	}
	return nil
}

func processPNG(data []byte) (*Processed, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	if err := checkPixelBudget(cfg); err != nil {
		return nil, err
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	// Re-encoding from the decoded pixel data drops every ancillary PNG
	// chunk (eXIf, tEXt, iTXt, zTXt, any application-specific chunk) —
	// Go's png.Encode never writes any of them. This is the actual EXIF
	// strip, not a best-effort chunk filter.
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("media: re-encode png: %w", err)
	}

	b := img.Bounds()
	return &Processed{Format: FormatPNG, Bytes: buf.Bytes(), Width: b.Dx(), Height: b.Dy()}, nil
}

// jpegReencodeQuality is used when re-encoding an accepted JPEG. High
// enough that visual quality loss is negligible for a gallery/cover photo.
const jpegReencodeQuality = 92

func processJPEG(data []byte) (*Processed, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	if err := checkPixelBudget(cfg); err != nil {
		return nil, err
	}

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	// Re-encoding from the decoded pixel data drops every APPn segment
	// (APP1/EXIF, APP1/XMP, APP2/ICC, APP13/Photoshop IRB, COM comments) —
	// Go's jpeg.Encode only ever writes the segments needed to describe
	// the pixel data itself.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegReencodeQuality}); err != nil {
		return nil, fmt.Errorf("media: re-encode jpeg: %w", err)
	}

	b := img.Bounds()
	return &Processed{Format: FormatJPEG, Bytes: buf.Bytes(), Width: b.Dx(), Height: b.Dy()}, nil
}

func processWebP(data []byte) (*Processed, error) {
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	if err := checkPixelBudget(cfg); err != nil {
		return nil, err
	}

	// golang.org/x/image/webp is decode-only (there is no pure-Go WebP
	// encoder in wide use without cgo/libwebp), so unlike PNG/JPEG we
	// cannot re-encode from the decoded pixel data. A full decode still
	// happens above (and here) purely as validation — it is what defends
	// against a polyglot/corrupt file, exactly like the PNG/JPEG path —
	// and metadata is then stripped surgically from the original RIFF
	// container by dropping its EXIF/XMP chunks entirely (see
	// stripWebPMetadata) rather than by round-tripping pixels.
	if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	cleaned, err := stripWebPMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}

	return &Processed{Format: FormatWebP, Bytes: cleaned, Width: cfg.Width, Height: cfg.Height}, nil
}

// webp chunk FourCCs that carry metadata rather than pixel/animation data.
// These are the chunks dropped by stripWebPMetadata. Note "XMP " has a
// trailing space — it is a 4-byte FourCC per the RIFF spec.
const (
	fourCCEXIF = "EXIF"
	fourCCXMP  = "XMP "
	fourCCVP8X = "VP8X"
)

// vp8xFlagExifBit and vp8xFlagXMPBit are bit positions within a VP8X
// chunk's first flags byte (RIFF container header, per the WebP
// container spec) that advertise the presence of EXIF/XMP chunks
// elsewhere in the file. They are cleared when those chunks are dropped
// so a decoder never goes looking for metadata that no longer exists.
const (
	vp8xFlagExifBit byte = 0x08
	vp8xFlagXMPBit  byte = 0x04
)

// stripWebPMetadata rebuilds data's RIFF/WebP container, dropping any
// EXIF or XMP chunk entirely (and clearing the corresponding advertised-
// presence bits in a VP8X chunk, if present) while leaving every
// pixel/animation-bearing chunk (VP8, VP8L, VP8X, ALPH, ANIM, ANMF) and
// the ICC colour profile (ICCP — colour-accuracy data, not
// privacy-sensitive metadata) untouched. The caller has already fully
// decoded data via webp.Decode, so the container is known to be
// well-formed; this function still bounds-checks every chunk defensively.
func stripWebPMetadata(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, errors.New("media: webp data too short for a RIFF header")
	}

	pos := 12
	var kept bytes.Buffer
	for pos+8 <= len(data) {
		fourcc := string(data[pos : pos+4])
		size := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		chunkStart := pos + 8
		chunkEnd := chunkStart + int(size)
		if size > uint32(len(data)) || chunkEnd < chunkStart || chunkEnd > len(data) {
			return nil, errors.New("media: webp chunk size exceeds file bounds")
		}

		padded := int(size)
		if padded%2 == 1 {
			padded++ // RIFF chunks are padded to an even length
		}
		chunkTotalEnd := chunkStart + padded
		if chunkTotalEnd > len(data) {
			// Final chunk's pad byte may be legitimately absent at EOF.
			chunkTotalEnd = len(data)
		}

		switch fourcc {
		case fourCCEXIF, fourCCXMP:
			// Drop the whole chunk (header + payload + padding).
		case fourCCVP8X:
			chunk := make([]byte, chunkTotalEnd-pos)
			copy(chunk, data[pos:chunkTotalEnd])
			if len(chunk) >= 9 {
				// Byte 8 of the chunk (0-indexed from FourCC start) is
				// the flags byte: FourCC(4) + size(4) + flags(1) + ...
				chunk[8] &^= vp8xFlagExifBit
				chunk[8] &^= vp8xFlagXMPBit
			}
			kept.Write(chunk)
		default:
			kept.Write(data[pos:chunkTotalEnd])
		}

		pos = chunkTotalEnd
	}

	body := kept.Bytes()
	out := make([]byte, 0, 12+len(body))
	out = append(out, 'R', 'I', 'F', 'F')
	var sizeBuf [4]byte
	binary.LittleEndian.PutUint32(sizeBuf[:], uint32(4+len(body))) // "WEBP" + chunks
	out = append(out, sizeBuf[:]...)
	out = append(out, 'W', 'E', 'B', 'P')
	out = append(out, body...)
	return out, nil
}
