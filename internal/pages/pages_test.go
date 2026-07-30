package pages

import (
	"errors"
	"strings"
	"testing"
)

// TestImageIDsFindsEveryReference backs the store-time ownership check: if this
// missed a reference, an image block naming another event's image would be
// stored unchecked. (The renderer would still drop it — see
// TestRenderDropsForeignImage — but a tenancy check that silently stops
// checking is worth its own test.)
func TestImageIDsFindsEveryReference(t *testing.T) {
	d := Document{
		Version: Version,
		Blocks: []Block{
			{Type: BlockHeading, Text: "Gallery", Level: 2},
			{Type: BlockImage, ImageID: "img-1", Alt: "one"},
			{Type: BlockDivider},
			{Type: BlockImage, ImageID: "img-2", Alt: "two"},
			{Type: BlockImage, ImageID: "img-1", Alt: "one again"},
		},
	}
	got := d.ImageIDs()
	want := []string{"img-1", "img-2", "img-1"}
	if len(got) != len(want) {
		t.Fatalf("ImageIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ImageIDs() = %v, want %v", got, want)
		}
	}
}

// TestCanonicalDropsWhatTheDecoderIgnored: what gets stored is a re-marshalling
// of the parsed struct, so a field the decoder did not populate cannot survive
// into the database even if the raw bytes carried it.
func TestCanonicalDropsWhatTheDecoderIgnored(t *testing.T) {
	// Only reachable by constructing a Document directly — Parse would have
	// refused an unknown field outright. This asserts the second half of the
	// property: even a struct built in Go marshals to exactly its known fields.
	d := Document{Version: Version, Blocks: []Block{{Type: BlockDivider}}}
	canon, err := d.Canonical()
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	s := string(canon)
	for _, unexpected := range []string{"text", "paragraphs", "image_id", "links", "items", "cta_href", "level"} {
		if strings.Contains(s, `"`+unexpected+`"`) {
			t.Errorf("canonical form carries an empty %q field: %s", unexpected, s)
		}
	}
}

// TestCanonicalRefusesAnInvalidDocument: the storage path cannot be used to
// smuggle in something Parse would have refused.
func TestCanonicalRefusesAnInvalidDocument(t *testing.T) {
	d := Document{Version: Version, Theme: Theme{Accent: "red"}}
	if _, err := d.Canonical(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
}

// TestValidateRejectsEveryDisallowedScheme is a broader sweep than the corpus
// carries, over schemes a browser or OS will act on.
func TestValidateRejectsEveryDisallowedScheme(t *testing.T) {
	for _, href := range []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"jAvAsCrIpT:alert(1)",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"blob:https://example.org/abc",
		"about:blank",
		"chrome://settings",
		"intent://scan/#Intent;scheme=zxing;end",
		"ws://example.org/",
		"ftp://example.org/",
	} {
		d := Document{Version: Version, Blocks: []Block{
			{Type: BlockLinks, Links: []Link{{Label: "x", Href: href}}},
		}}
		if err := d.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("href %q was accepted", href)
		}
	}
}

// TestValidateAcceptsOrdinaryWritingInManyScripts: the text gate must not be a
// de-facto ASCII filter. Emoji, combining marks, ideographs and RTL script are
// all ordinary content.
func TestValidateAcceptsOrdinaryWritingInManyScripts(t *testing.T) {
	for _, text := range []string{
		"Sound & Vision",
		"Café Oto — 20:00",
		"京都の夜",
		"Ноќен сет",
		"حفلة منتصف الليل",
		"नई दिल्ली",
		"Doors 19:00 🎧",
		"Ångström",
	} {
		d := Document{Version: Version, Blocks: []Block{{Type: BlockHeading, Text: text, Level: 2}}}
		if err := d.Validate(); err != nil {
			t.Errorf("legitimate text %q was rejected: %v", text, err)
		}
	}
}

// TestParseRejectsNonObjectAndEmptyBodies — the decoder's own failures must
// still come back as ErrInvalid so the HTTP layer answers 400 rather than 500.
func TestParseRejectsNonObjectAndEmptyBodies(t *testing.T) {
	for _, raw := range []string{"", "   ", "null", "[]", `"a string"`, "42", "{", `{"version":1,`} {
		if _, err := Parse([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) = %v, want an ErrInvalid", raw, err)
		}
	}
}

// TestValidateMessagesNameTheOffendingPath: a host has to be able to fix the
// document from the message alone, since the message is all the API returns.
func TestValidateMessagesNameTheOffendingPath(t *testing.T) {
	d := Document{Version: Version, Blocks: []Block{
		{Type: BlockDivider},
		{Type: BlockFAQ, Items: []FAQItem{
			{Question: "ok?", Answer: "yes"},
			{Question: "ok?", Answer: ""},
		}},
	}}
	err := d.Validate()
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if !strings.Contains(err.Error(), "blocks[1].items[1].answer") {
		t.Errorf("error does not name the offending path: %v", err)
	}
}
