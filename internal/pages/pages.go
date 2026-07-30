// Package pages implements Cackle's host-authored event pages: the document
// model a host submits, its validation, and the server-side renderer that
// turns it into HTML.
//
// # The one decision this package encodes
//
// A host NEVER supplies HTML. Not sanitised HTML, not "safe subset" HTML, not
// a markup dialect that compiles to HTML. A host page is a Document: a small,
// closed vocabulary of typed blocks carrying PLAIN TEXT, plus a theme whose
// every field is either a `#rrggbb` colour or a value from a fixed enum.
//
// This is deliberate, and it is the security design rather than a limitation
// that happened to fall out of one. Cackle serves the organiser SPA, the JSON
// API and (by default) these pages from a single origin. A script executing on
// that origin can read the JS-readable CSRF cookie, and the session cookie
// rides along on every same-origin request automatically — so script execution
// in host content is not "an XSS", it is organiser account takeover for
// whichever organiser happens to open the page. Accepting HTML and filtering it
// afterwards makes the whole product's tenancy boundary depend on a sanitiser
// never having a bypass, forever, across every future browser parser quirk.
// That is a bet this package refuses to take: there is no parser to bypass
// because there is no markup to parse.
//
// What a host gives up is inline markup. What a host gets instead is
// docs/API.md — the same public JSON the default page is built from — and full
// freedom to build any page they like on their own origin. See
// docs/HOST-PAGES.md, which specifies this document format precisely enough to
// build against without reading this file, and
// docs/host-page-vectors.json, the frozen corpus that pins every accept/reject
// decision below.
//
// # Two independent layers, on the way in and on the way out
//
//   - On the way IN, Validate rejects anything outside the closed vocabulary:
//     unknown block types, stowaway fields that do not belong to the block type
//     carrying them, control and bidi-override characters, colours that are not
//     six hex digits, URLs whose scheme is not http/https/mailto, and lengths
//     past the caps below.
//   - On the way OUT, Render uses html/template, whose contextual escaping is
//     applied to every host value in its real context — text as text, colours
//     inside a <style> block as CSS, hrefs as URLs. No template.HTML,
//     template.CSS or template.URL cast appears anywhere in this package, so
//     nothing here can opt a host value out of that escaping.
//
// Either layer alone would very probably be enough. The point is that a defect
// in one is not a vulnerability, because there is no host input that reaches
// the browser without passing both.
//
// # Purity
//
// This package imports no store, no HTTP and no service: it turns bytes into a
// Document and a Document into HTML, and that is all. Persistence and RBAC live
// in internal/httpapi, the same division internal/scan already uses (see its
// package doc). Keeping it pure is what lets the conformance corpus be run
// against it directly, with no database in the way.
package pages

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Version is the only document version this package accepts. A document
// declaring anything else is rejected rather than coerced — a future version
// will mean a real format change, and silently reading a v2 document as v1
// would render a page its author never wrote.
const Version = 1

// Limits. Every one of these is part of the published contract
// (docs/HOST-PAGES.md) so a host client can validate before submitting, and
// every one is enforced here rather than by a database column type.
//
// They are counts of RUNES, not bytes, wherever they bound human text: a cap
// expressed in bytes would silently give a Latin-script host four times the
// room of a host writing in an ideographic script, which is exactly the kind of
// built-in assumption this product does not make.
const (
	// MaxDocumentBytes bounds the raw JSON a host may submit. It is checked
	// by the HTTP layer before decoding, so an oversized body is refused
	// without ever being buffered in full.
	MaxDocumentBytes = 64 << 10

	MaxBlocks         = 64
	MaxHeadingRunes   = 200
	MaxParagraphs     = 24
	MaxParagraphRunes = 2000
	MaxLinks          = 24
	MaxLabelRunes     = 120
	MaxHrefBytes      = 2048
	MaxFAQItems       = 32
	MaxQuestionRunes  = 300
	MaxAnswerRunes    = 2000
	MaxAltRunes       = 300
	MaxLangBytes      = 35
)

// ErrInvalid is the sentinel every validation failure wraps. Callers match it
// with errors.Is and surface err.Error() to the client: the messages are
// written to be shown to a host verbatim (they name the exact JSON path that
// was wrong) and deliberately contain nothing internal.
var ErrInvalid = errors.New("pages: invalid document")

// Block types. This list IS the vocabulary — Validate rejects anything else,
// and there is no "raw"/"html"/"embed" member by design.
const (
	// BlockHeading is a section heading. Level is 2 or 3; level 1 is the
	// event title, which the renderer owns and a host cannot displace.
	BlockHeading = "heading"
	// BlockText is one or more plain-text paragraphs.
	BlockText = "text"
	// BlockImage is one image from THIS EVENT'S OWN gallery, by id. It can
	// never address an image belonging to another event or another org —
	// see AllowedImages in the render input and the ownership check the
	// HTTP layer runs before storing.
	BlockImage = "image"
	// BlockLinks is a list of labelled links.
	BlockLinks = "links"
	// BlockFAQ is a list of question/answer pairs.
	BlockFAQ = "faq"
	// BlockTickets renders the event's live ticket types and the call to
	// action that starts the purchase flow.
	BlockTickets = "tickets"
	// BlockDetails renders when/where from the event's own record.
	BlockDetails = "details"
	// BlockDivider is a horizontal rule.
	BlockDivider = "divider"
)

// Theme fields with closed value sets.
const (
	FontSystem = "system"
	FontSerif  = "serif"
	FontMono   = "mono"

	DirLTR  = "ltr"
	DirRTL  = "rtl"
	DirAuto = "auto"

	CornersSharp = "sharp"
	CornersSoft  = "soft"
	CornersRound = "round"
)

var (
	// colourRE is the whole colour grammar: exactly six hex digits behind a
	// '#'. No named colours, no rgb()/hsl(), no 3-digit shorthand, no
	// alpha. A closed grammar this small is trivially auditable, and every
	// value matching it is inert in CSS.
	colourRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

	// langRE is a conservative BCP-47 shape for the <html lang> attribute:
	// subtags of ASCII letters/digits joined by hyphens. It is not a full
	// registry check — it does not need to be, because the value only ever
	// lands in a lang attribute, and anything outside this shape is refused.
	langRE = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

// Document is a host's page. The zero value is not a valid document; use
// Default to obtain the zero-configuration one.
type Document struct {
	// Version must equal Version.
	Version int `json:"version"`
	// Lang is an optional BCP-47 language tag for the <html lang>
	// attribute. Empty means "unspecified" and the renderer omits the
	// attribute rather than guessing a language.
	Lang string `json:"lang,omitempty"`
	// Theme is presentation only; every field is optional.
	Theme Theme `json:"theme"`
	// Labels overrides the renderer's own chrome strings. Cackle's defaults
	// are English because something has to be; a host running an event in
	// another language replaces them here rather than being stuck with a
	// page that is half-translated.
	Labels Labels `json:"labels"`
	// Blocks is the page body, in order.
	Blocks []Block `json:"blocks"`
}

// Theme carries presentation choices. Colours are `#rrggbb`; everything else
// is drawn from a closed enum. There is no field into which a CSS declaration,
// selector, url() or @import could be written, because there is no free-form
// CSS field at all.
type Theme struct {
	Background string `json:"background,omitempty"`
	Surface    string `json:"surface,omitempty"`
	Text       string `json:"text,omitempty"`
	Muted      string `json:"muted,omitempty"`
	Accent     string `json:"accent,omitempty"`
	AccentText string `json:"accent_text,omitempty"`
	// Font selects one of three built-in, locally-resolved stacks. There is
	// deliberately no way to name a font file or a font service: a page that
	// fetched a webfont would make a third-party request from a page Cackle
	// serves, which docs/HOST-PAGES.md rules out.
	Font string `json:"font,omitempty"`
	// Direction sets the document's base direction explicitly. Note that
	// Validate REJECTS Unicode bidi override and isolate characters in text,
	// so direction is something a host declares here rather than something
	// smuggled into a string.
	Direction string `json:"direction,omitempty"`
	Corners   string `json:"corners,omitempty"`
}

// Labels are the renderer's own chrome strings, all overridable, all plain
// text. Empty means "use the built-in default" (see Labels.withDefaults).
type Labels struct {
	Tickets    string `json:"tickets,omitempty"`
	GetTickets string `json:"get_tickets,omitempty"`
	Free       string `json:"free,omitempty"`
	SoldOut    string `json:"sold_out,omitempty"`
	When       string `json:"when,omitempty"`
	Where      string `json:"where,omitempty"`
	Cancelled  string `json:"cancelled,omitempty"`
}

// Link is one labelled outbound link.
type Link struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

// FAQItem is one question/answer pair.
type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Block is one page element. Every block type reads a documented subset of
// these fields; Validate rejects a block that carries a field belonging to a
// DIFFERENT type ("stowaway fields"), so a document cannot quietly hold content
// that no renderer displays but a future one might.
type Block struct {
	Type string `json:"type"`

	// heading
	Text  string `json:"text,omitempty"`
	Level int    `json:"level,omitempty"`

	// text
	Paragraphs []string `json:"paragraphs,omitempty"`

	// image
	ImageID string `json:"image_id,omitempty"`
	Alt     string `json:"alt,omitempty"`

	// links
	Links []Link `json:"links,omitempty"`

	// faq
	Items []FAQItem `json:"items,omitempty"`

	// tickets
	CTAHref string `json:"cta_href,omitempty"`
}

// Parse decodes raw JSON into a Document and validates it. Unknown JSON fields
// are an ERROR, not a silent drop: a host who misspells "paragraphs" should be
// told, not served a blank section and left to guess.
func Parse(raw []byte) (Document, error) {
	if len(raw) > MaxDocumentBytes {
		return Document{}, fmt.Errorf("%w: document exceeds %d bytes", ErrInvalid, MaxDocumentBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var d Document
	if err := dec.Decode(&d); err != nil {
		return Document{}, fmt.Errorf("%w: %s", ErrInvalid, jsonErrText(err))
	}
	// Exactly one JSON value, nothing trailing.
	if dec.More() {
		return Document{}, fmt.Errorf("%w: trailing content after the document", ErrInvalid)
	}
	if err := d.Validate(); err != nil {
		return Document{}, err
	}
	return d, nil
}

// jsonErrText keeps decoder errors safe to echo to a client: encoding/json's
// messages name JSON types and field names only, never anything internal, but
// they can be long — this trims them to a single line.
func jsonErrText(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// Canonical re-serialises a validated Document. This — never the host's own
// bytes — is what gets stored: the stored form is a re-marshalling of a parsed,
// validated Go struct, so anything the decoder did not understand cannot
// survive a round trip even in principle.
func (d Document) Canonical() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("pages: canonicalise: %w", err)
	}
	return b, nil
}

// Validate reports whether d is a well-formed host page document. Every
// failure wraps ErrInvalid and names the JSON path at fault.
func (d Document) Validate() error {
	if d.Version != Version {
		return fmt.Errorf("%w: version must be %d", ErrInvalid, Version)
	}
	if d.Lang != "" {
		if len(d.Lang) > MaxLangBytes || !langRE.MatchString(d.Lang) {
			return fmt.Errorf("%w: lang must be a BCP-47 tag such as \"en\", \"pt-BR\" or \"ar\"", ErrInvalid)
		}
	}
	if err := d.Theme.validate(); err != nil {
		return err
	}
	if err := d.Labels.validate(); err != nil {
		return err
	}
	if len(d.Blocks) > MaxBlocks {
		return fmt.Errorf("%w: blocks: at most %d blocks (got %d)", ErrInvalid, MaxBlocks, len(d.Blocks))
	}
	for i, b := range d.Blocks {
		if err := b.validate(fmt.Sprintf("blocks[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

// ImageIDs returns every image id referenced by an image block, in order. The
// HTTP layer uses it to verify — before storing — that every one belongs to the
// event being edited.
func (d Document) ImageIDs() []string {
	var out []string
	for _, b := range d.Blocks {
		if b.Type == BlockImage && b.ImageID != "" {
			out = append(out, b.ImageID)
		}
	}
	return out
}

func (t Theme) validate() error {
	for _, f := range []struct {
		name, value string
	}{
		{"background", t.Background},
		{"surface", t.Surface},
		{"text", t.Text},
		{"muted", t.Muted},
		{"accent", t.Accent},
		{"accent_text", t.AccentText},
	} {
		if f.value == "" {
			continue
		}
		if !colourRE.MatchString(f.value) {
			return fmt.Errorf("%w: theme.%s must be a six-digit hex colour such as \"#1b1b1f\"", ErrInvalid, f.name)
		}
	}
	if err := oneOf("theme.font", t.Font, FontSystem, FontSerif, FontMono); err != nil {
		return err
	}
	if err := oneOf("theme.direction", t.Direction, DirLTR, DirRTL, DirAuto); err != nil {
		return err
	}
	return oneOf("theme.corners", t.Corners, CornersSharp, CornersSoft, CornersRound)
}

func (l Labels) validate() error {
	for _, f := range []struct {
		name, value string
	}{
		{"tickets", l.Tickets},
		{"get_tickets", l.GetTickets},
		{"free", l.Free},
		{"sold_out", l.SoldOut},
		{"when", l.When},
		{"where", l.Where},
		{"cancelled", l.Cancelled},
	} {
		if f.value == "" {
			continue
		}
		if err := plainText("labels."+f.name, f.value, MaxLabelRunes); err != nil {
			return err
		}
	}
	return nil
}

// oneOf enforces a closed enum. An empty value always passes and means "use
// the default"; anything else must be an exact, case-sensitive match.
func oneOf(path, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%w: %s must be one of %s", ErrInvalid, path, strings.Join(allowed, ", "))
}

func (b Block) validate(path string) error {
	switch b.Type {
	case BlockHeading:
		if err := b.noFieldsExcept(path, fieldText|fieldLevel); err != nil {
			return err
		}
		if err := requiredText(path+".text", b.Text, MaxHeadingRunes); err != nil {
			return err
		}
		if b.Level != 0 && b.Level != 2 && b.Level != 3 {
			return fmt.Errorf("%w: %s.level must be 2 or 3 (level 1 is the event title)", ErrInvalid, path)
		}
		return nil

	case BlockText:
		if err := b.noFieldsExcept(path, fieldParagraphs); err != nil {
			return err
		}
		if len(b.Paragraphs) == 0 {
			return fmt.Errorf("%w: %s.paragraphs must contain at least one paragraph", ErrInvalid, path)
		}
		if len(b.Paragraphs) > MaxParagraphs {
			return fmt.Errorf("%w: %s.paragraphs: at most %d paragraphs (got %d)", ErrInvalid, path, MaxParagraphs, len(b.Paragraphs))
		}
		for i, p := range b.Paragraphs {
			if err := requiredText(fmt.Sprintf("%s.paragraphs[%d]", path, i), p, MaxParagraphRunes); err != nil {
				return err
			}
		}
		return nil

	case BlockImage:
		if err := b.noFieldsExcept(path, fieldImageID|fieldAlt); err != nil {
			return err
		}
		if err := requiredText(path+".image_id", b.ImageID, 64); err != nil {
			return err
		}
		// Alt text is REQUIRED, not optional. A decorative-image escape
		// hatch would be the only way to get an unlabelled image onto a
		// public page, and an event page has no decorative images.
		return requiredText(path+".alt", b.Alt, MaxAltRunes)

	case BlockLinks:
		if err := b.noFieldsExcept(path, fieldLinks); err != nil {
			return err
		}
		if len(b.Links) == 0 {
			return fmt.Errorf("%w: %s.links must contain at least one link", ErrInvalid, path)
		}
		if len(b.Links) > MaxLinks {
			return fmt.Errorf("%w: %s.links: at most %d links (got %d)", ErrInvalid, path, MaxLinks, len(b.Links))
		}
		for i, l := range b.Links {
			lp := fmt.Sprintf("%s.links[%d]", path, i)
			if err := requiredText(lp+".label", l.Label, MaxLabelRunes); err != nil {
				return err
			}
			if err := validateHref(lp+".href", l.Href); err != nil {
				return err
			}
		}
		return nil

	case BlockFAQ:
		if err := b.noFieldsExcept(path, fieldItems); err != nil {
			return err
		}
		if len(b.Items) == 0 {
			return fmt.Errorf("%w: %s.items must contain at least one question", ErrInvalid, path)
		}
		if len(b.Items) > MaxFAQItems {
			return fmt.Errorf("%w: %s.items: at most %d items (got %d)", ErrInvalid, path, MaxFAQItems, len(b.Items))
		}
		for i, it := range b.Items {
			ip := fmt.Sprintf("%s.items[%d]", path, i)
			if err := requiredText(ip+".question", it.Question, MaxQuestionRunes); err != nil {
				return err
			}
			if err := requiredText(ip+".answer", it.Answer, MaxAnswerRunes); err != nil {
				return err
			}
		}
		return nil

	case BlockTickets:
		if err := b.noFieldsExcept(path, fieldCTAHref); err != nil {
			return err
		}
		if b.CTAHref != "" {
			return validateHref(path+".cta_href", b.CTAHref)
		}
		return nil

	case BlockDetails, BlockDivider:
		return b.noFieldsExcept(path, 0)

	case "":
		return fmt.Errorf("%w: %s.type is required", ErrInvalid, path)
	default:
		return fmt.Errorf("%w: %s.type %q is not a block type; see docs/HOST-PAGES.md", ErrInvalid, path, b.Type)
	}
}

// Block field bits, for the stowaway-field check.
const (
	fieldText = 1 << iota
	fieldLevel
	fieldParagraphs
	fieldImageID
	fieldAlt
	fieldLinks
	fieldItems
	fieldCTAHref
)

// noFieldsExcept rejects a block carrying a populated field that its own type
// does not read. Without this, `{"type":"divider","paragraphs":["..."]}`
// validates and stores content that is invisible today — and the day a
// renderer starts reading that field, it renders text nobody re-reviewed.
// Refusing it keeps "what is stored" and "what is shown" the same set.
func (b Block) noFieldsExcept(path string, allowed int) error {
	present := []struct {
		bit  int
		name string
		set  bool
	}{
		{fieldText, "text", b.Text != ""},
		{fieldLevel, "level", b.Level != 0},
		{fieldParagraphs, "paragraphs", len(b.Paragraphs) > 0},
		{fieldImageID, "image_id", b.ImageID != ""},
		{fieldAlt, "alt", b.Alt != ""},
		{fieldLinks, "links", len(b.Links) > 0},
		{fieldItems, "items", len(b.Items) > 0},
		{fieldCTAHref, "cta_href", b.CTAHref != ""},
	}
	for _, f := range present {
		if f.set && allowed&f.bit == 0 {
			return fmt.Errorf("%w: %s: a %q block does not take %q", ErrInvalid, path, b.Type, f.name)
		}
	}
	return nil
}

func requiredText(path, s string, maxRunes int) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, path)
	}
	return plainText(path, s, maxRunes)
}

// plainText is the text gate. It admits ordinary human writing in any script
// and refuses everything that is not that.
func plainText(path, s string, maxRunes int) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalid, path)
	}
	if n := utf8.RuneCountInString(s); n > maxRunes {
		return fmt.Errorf("%w: %s is %d characters; the maximum is %d", ErrInvalid, path, n, maxRunes)
	}
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			// Line structure is expressed by separate paragraphs, not by
			// characters inside one. Allowing them would put whitespace
			// handling — and therefore layout — back in the host's string,
			// which is the thing this format is built to avoid.
			return fmt.Errorf("%w: %s must not contain tabs or line breaks; use separate paragraphs", ErrInvalid, path)
		case r < 0x20 || r == 0x7f:
			return fmt.Errorf("%w: %s must not contain control characters", ErrInvalid, path)
		case isBidiControl(r):
			// Trojan-Source characters. The renderer supports right-to-left
			// text properly, via theme.direction and the dir attribute — so
			// there is no legitimate reason to reorder text with invisible
			// controls, and every illegitimate one (making a link label read
			// as a different URL, making a price read backwards) is a
			// spoofing primitive on a page other people's buyers trust.
			return fmt.Errorf("%w: %s must not contain bidirectional override characters; set theme.direction instead", ErrInvalid, path)
		case r == 0xFFFD:
			return fmt.Errorf("%w: %s must not contain the Unicode replacement character", ErrInvalid, path)
		}
	}
	return nil
}

// isBidiControl reports whether r is a Unicode bidi override, isolate or
// embedding control.
func isBidiControl(r rune) bool {
	switch r {
	case 0x061C, // ARABIC LETTER MARK
		0x200E, 0x200F, // LRM, RLM
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // LRE, RLE, PDF, LRO, RLO
		0x2066, 0x2067, 0x2068, 0x2069: // LRI, RLI, FSI, PDI
		return true
	}
	return false
}

// validateHref admits absolute http, https and mailto URLs and nothing else.
//
// html/template would already neutralise a `javascript:` href on the way out
// (it rewrites unknown schemes to "#ZgotmplZ"), and that is the second layer.
// This is the first: a document that would render a dead link is refused at
// submission time, when there is a human to tell, instead of silently
// producing a broken page.
func validateHref(path, href string) error {
	if strings.TrimSpace(href) == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, path)
	}
	if len(href) > MaxHrefBytes {
		return fmt.Errorf("%w: %s is longer than %d bytes", ErrInvalid, path, MaxHrefBytes)
	}
	for _, r := range href {
		if r < 0x20 || r == 0x7f || isBidiControl(r) {
			return fmt.Errorf("%w: %s must not contain control or bidirectional characters", ErrInvalid, path)
		}
	}
	u, err := url.Parse(href)
	if err != nil {
		return fmt.Errorf("%w: %s is not a valid URL", ErrInvalid, path)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			// Catches "http:/foo" and, importantly, protocol-relative
			// "//evil.example" (which url.Parse reads as scheme-less).
			return fmt.Errorf("%w: %s must include a host, e.g. https://example.org/path", ErrInvalid, path)
		}
		if u.User != nil {
			// https://cackle.example.org@evil.example reads as a Cackle URL
			// to a human and resolves to evil.example. There is no honest
			// use for userinfo in a link on a public page.
			return fmt.Errorf("%w: %s must not embed credentials before the host", ErrInvalid, path)
		}
		return nil
	case "mailto":
		if strings.TrimSpace(u.Opaque) == "" {
			return fmt.Errorf("%w: %s must include an address, e.g. mailto:box@example.org", ErrInvalid, path)
		}
		return nil
	case "":
		return fmt.Errorf("%w: %s must be an absolute URL beginning with https://, http:// or mailto:", ErrInvalid, path)
	default:
		return fmt.Errorf("%w: %s scheme %q is not allowed; use https, http or mailto", ErrInvalid, path, u.Scheme)
	}
}
