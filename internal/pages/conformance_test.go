package pages

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The conformance corpus for the host page document format.
//
// docs/host-page-vectors.json is the executable half of docs/HOST-PAGES.md,
// held to the same standard as docs/ticket-format-vectors.json: frozen inputs,
// the exact outcome each one must produce, and no -update flag. If the format
// genuinely needs to change, that is a `version` bump plus a deliberate
// regeneration plus a CHANGELOG entry — not something a test run rubber-stamps.
//
// One runner, not two. TICKET-FORMAT.md has a Go runner and a JavaScript one
// because two verifiers ship in this repo and both have to agree byte for byte.
// The host page document has exactly one implementation — this package, server
// side — so a second runner here would be ceremony rather than a check on
// anything. The corpus is published in docs/ regardless, because the audience
// for it is a host writing their own client-side validator against
// docs/HOST-PAGES.md, and that implementation has something to be checked
// against on the day it exists.

const vectorsPath = "../../docs/host-page-vectors.json"

// Floors, not targets. A corpus that shrank is a broken corpus, not a tidy one.
const (
	minAcceptVectors = 10
	minRejectVectors = 45
	minRenderVectors = 10
)

type vectorFile struct {
	Format        string         `json:"format"`
	Version       int            `json:"version"`
	Limits        map[string]int `json:"limits"`
	BlockTypes    []string       `json:"block_types"`
	ReasonClasses []string       `json:"reason_classes"`
	Accept        []acceptVector `json:"accept"`
	Reject        []rejectVector `json:"reject"`
	Render        []renderVector `json:"render"`
}

type acceptVector struct {
	Name     string          `json:"name"`
	Note     string          `json:"note"`
	Document json.RawMessage `json:"document"`
}

type rejectVector struct {
	Name            string          `json:"name"`
	Note            string          `json:"note"`
	Reason          string          `json:"reason"`
	Path            string          `json:"path"`
	MessageContains string          `json:"message_contains"`
	Document        json.RawMessage `json:"document"`
	Raw             string          `json:"raw"`
}

type renderVector struct {
	Name           string          `json:"name"`
	Document       json.RawMessage `json:"document"`
	Event          *vectorEvent    `json:"event"`
	Tickets        []vectorTicket  `json:"tickets"`
	AllowedImages  []vectorImage   `json:"allowed_images"`
	BuyHref        string          `json:"buy_href"`
	MustContain    []string        `json:"must_contain"`
	MustNotContain []string        `json:"must_not_contain"`
}

type vectorEvent struct {
	Title    string `json:"title"`
	Status   string `json:"status"`
	Currency string `json:"currency"`
}

type vectorTicket struct {
	Name       string `json:"name"`
	PriceMinor int64  `json:"price_minor"`
	SoldOut    bool   `json:"sold_out"`
}

type vectorImage struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func loadVectors(t *testing.T) *vectorFile {
	t.Helper()
	raw, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatalf("read %s: %v", vectorsPath, err)
	}
	var vf vectorFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatalf("parse %s: %v", vectorsPath, err)
	}
	if vf.Format != "cackle-host-page-document" {
		t.Fatalf("unexpected corpus format %q", vf.Format)
	}
	if vf.Version != Version {
		t.Fatalf("corpus declares version %d but this package implements %d", vf.Version, Version)
	}
	return &vf
}

// expand applies the corpus's two documented expansion rules. It is
// deliberately the only preprocessing there is: everything else in a vector is
// used exactly as written.
func expand(v any) any {
	switch t := v.(type) {
	case string:
		if rest, ok := strings.CutPrefix(t, "@rep:"); ok {
			// "@rep:<unit>:<count>" — split on the LAST colon so a unit
			// containing one is still expressible.
			i := strings.LastIndexByte(rest, ':')
			if i < 0 {
				return t
			}
			n, err := strconv.Atoi(rest[i+1:])
			if err != nil || n < 0 {
				return t
			}
			return strings.Repeat(rest[:i], n)
		}
		return t
	case []any:
		// Rule 2: [{"@rep": N, "value": V}] -> N copies of V.
		if len(t) == 1 {
			if m, ok := t[0].(map[string]any); ok {
				if nf, ok := m["@rep"].(float64); ok {
					val := expand(m["value"])
					out := make([]any, 0, int(nf))
					for i := 0; i < int(nf); i++ {
						out = append(out, val)
					}
					return out
				}
			}
		}
		out := make([]any, len(t))
		for i := range t {
			out[i] = expand(t[i])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = expand(val)
		}
		return out
	default:
		return v
	}
}

// documentBytes decodes a vector's document, expands it, and re-encodes it —
// producing the exact bytes a host would have POSTed.
func documentBytes(t *testing.T, name string, raw json.RawMessage) []byte {
	t.Helper()
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("%s: decode document: %v", name, err)
	}
	out, err := json.Marshal(expand(generic))
	if err != nil {
		t.Fatalf("%s: re-encode document: %v", name, err)
	}
	return out
}

// TestConformanceCorpusIsWellFormed checks the corpus before trusting it: a
// truncated or half-written file must fail loudly rather than pass by running
// almost nothing.
func TestConformanceCorpusIsWellFormed(t *testing.T) {
	vf := loadVectors(t)

	if len(vf.Accept) < minAcceptVectors {
		t.Errorf("only %d accept vectors, expected at least %d", len(vf.Accept), minAcceptVectors)
	}
	if len(vf.Reject) < minRejectVectors {
		t.Errorf("only %d reject vectors, expected at least %d", len(vf.Reject), minRejectVectors)
	}
	if len(vf.Render) < minRenderVectors {
		t.Errorf("only %d render vectors, expected at least %d", len(vf.Render), minRenderVectors)
	}

	// Every published reason class must be exercised. This is what stops a
	// validation rule being deleted while the corpus still reports success.
	seen := map[string]int{}
	for _, v := range vf.Reject {
		if v.Reason == "" {
			t.Errorf("reject vector %q declares no reason class", v.Name)
			continue
		}
		seen[v.Reason]++
	}
	declared := map[string]bool{}
	for _, c := range vf.ReasonClasses {
		declared[c] = true
		if seen[c] == 0 {
			t.Errorf("reason class %q is declared but no reject vector exercises it", c)
		}
	}
	for c := range seen {
		if !declared[c] {
			t.Errorf("reject vector uses reason class %q, which is not in reason_classes", c)
		}
	}

	// The published limits must be the limits this package actually enforces,
	// or a host validating against the document builds to the wrong numbers.
	for name, want := range map[string]int{
		"max_document_bytes":  MaxDocumentBytes,
		"max_blocks":          MaxBlocks,
		"max_heading_runes":   MaxHeadingRunes,
		"max_paragraphs":      MaxParagraphs,
		"max_paragraph_runes": MaxParagraphRunes,
		"max_links":           MaxLinks,
		"max_label_runes":     MaxLabelRunes,
		"max_href_bytes":      MaxHrefBytes,
		"max_faq_items":       MaxFAQItems,
		"max_question_runes":  MaxQuestionRunes,
		"max_answer_runes":    MaxAnswerRunes,
		"max_alt_runes":       MaxAltRunes,
		"max_lang_bytes":      MaxLangBytes,
	} {
		if got, ok := vf.Limits[name]; !ok {
			t.Errorf("corpus does not publish limit %q", name)
		} else if got != want {
			t.Errorf("corpus publishes %s=%d but this package enforces %d", name, got, want)
		}
	}

	// The published block vocabulary must be the whole vocabulary.
	wantTypes := map[string]bool{
		BlockHeading: true, BlockText: true, BlockImage: true, BlockLinks: true,
		BlockFAQ: true, BlockTickets: true, BlockDetails: true, BlockDivider: true,
	}
	if len(vf.BlockTypes) != len(wantTypes) {
		t.Errorf("corpus publishes %d block types, this package has %d", len(vf.BlockTypes), len(wantTypes))
	}
	for _, bt := range vf.BlockTypes {
		if !wantTypes[bt] {
			t.Errorf("corpus publishes block type %q, which this package does not implement", bt)
		}
	}
}

func TestConformanceAccept(t *testing.T) {
	vf := loadVectors(t)
	ran := 0
	for _, v := range vf.Accept {
		t.Run(v.Name, func(t *testing.T) {
			raw := documentBytes(t, v.Name, v.Document)
			doc, err := Parse(raw)
			if err != nil {
				t.Fatalf("expected the document to be accepted, got: %v", err)
			}

			// Round trip: what gets stored is a re-marshalling of the parsed
			// struct, and re-parsing it must yield the same document. A field
			// that survives one pass but not the next would mean stored
			// content and rendered content can drift.
			canon, err := doc.Canonical()
			if err != nil {
				t.Fatalf("canonicalise: %v", err)
			}
			again, err := Parse(canon)
			if err != nil {
				t.Fatalf("the canonical form of an accepted document was rejected: %v", err)
			}
			canon2, err := again.Canonical()
			if err != nil {
				t.Fatalf("canonicalise (second pass): %v", err)
			}
			if string(canon) != string(canon2) {
				t.Errorf("canonical form is not stable:\n first: %s\nsecond: %s", canon, canon2)
			}
		})
		ran++
	}
	// This test must not be able to pass having run nothing: a corpus that
	// shrank, or a loop that quietly stopped iterating, has to fail loudly
	// here rather than rely on TestConformanceCorpusIsWellFormed alone.
	if ran != len(vf.Accept) {
		t.Fatalf("ran %d accept vectors, file has %d", ran, len(vf.Accept))
	}
}

func TestConformanceReject(t *testing.T) {
	vf := loadVectors(t)
	ran := 0
	for _, v := range vf.Reject {
		t.Run(v.Name, func(t *testing.T) {
			var raw []byte
			switch {
			case v.Raw != "":
				raw = []byte(expand(v.Raw).(string))
			case len(v.Document) > 0:
				raw = documentBytes(t, v.Name, v.Document)
			default:
				t.Fatal("vector has neither a document nor a raw body")
			}

			_, err := Parse(raw)
			if err == nil {
				t.Fatal("expected the document to be REJECTED; it was accepted")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("rejection must wrap ErrInvalid so callers can classify it; got %v", err)
			}
			if v.MessageContains != "" && !strings.Contains(err.Error(), v.MessageContains) {
				t.Errorf("error %q does not contain %q", err.Error(), v.MessageContains)
			}
			if v.Path != "" && !strings.Contains(err.Error(), v.Path) {
				t.Errorf("error %q does not name the offending path %q", err.Error(), v.Path)
			}
		})
		ran++
	}
	if ran != len(vf.Reject) {
		t.Fatalf("ran %d reject vectors, file has %d", ran, len(vf.Reject))
	}
}

func TestConformanceRender(t *testing.T) {
	vf := loadVectors(t)
	ran := 0
	for _, v := range vf.Render {
		t.Run(v.Name, func(t *testing.T) {
			raw := documentBytes(t, v.Name, v.Document)
			doc, err := Parse(raw)
			if err != nil {
				t.Fatalf("render vectors must carry a valid document: %v", err)
			}

			ev := Event{
				Slug:      "midnight-set",
				Title:     "Midnight Set",
				VenueName: "The Assembly",
				Address:   "12 Harrington Street",
				StartsAt:  time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC),
				EndsAt:    time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC),
				Timezone:  "UTC",
				Status:    "published",
				Currency:  "ZAR",
			}
			if v.Event != nil {
				if v.Event.Title != "" {
					ev.Title = v.Event.Title
				}
				if v.Event.Status != "" {
					ev.Status = v.Event.Status
				}
				if v.Event.Currency != "" {
					ev.Currency = v.Event.Currency
				}
			}

			in := RenderInput{Doc: doc, Event: ev, BuyHref: v.BuyHref, Nonce: "test-nonce"}
			for _, tk := range v.Tickets {
				in.Tickets = append(in.Tickets, TicketType{Name: tk.Name, PriceMinor: tk.PriceMinor, SoldOut: tk.SoldOut})
			}
			for _, img := range v.AllowedImages {
				in.AllowedImages = append(in.AllowedImages, Image{ID: img.ID, Width: img.Width, Height: img.Height})
			}

			out, err := Render(in)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			html := string(out)

			for _, want := range v.MustContain {
				if !strings.Contains(html, want) {
					t.Errorf("rendered page is missing %q", want)
				}
			}
			for _, notWant := range v.MustNotContain {
				if strings.Contains(html, notWant) {
					t.Errorf("rendered page must not contain %q", notWant)
				}
			}
		})
		ran++
	}
	if ran != len(vf.Render) {
		t.Fatalf("ran %d render vectors, file has %d", ran, len(vf.Render))
	}
}

// TestConformanceEveryAcceptedDocumentRenders: an accepted document that then
// fails to render would be a format whose two halves disagree.
func TestConformanceEveryAcceptedDocumentRenders(t *testing.T) {
	vf := loadVectors(t)
	ran := 0
	for _, v := range vf.Accept {
		t.Run(v.Name, func(t *testing.T) {
			doc, err := Parse(documentBytes(t, v.Name, v.Document))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			out, err := Render(RenderInput{
				Doc:     doc,
				Event:   sampleEvent(),
				Tickets: []TicketType{{Name: "General", PriceMinor: 15000}},
				BuyHref: "/events/midnight-set",
				Nonce:   "test-nonce",
			})
			if err != nil {
				t.Fatalf("an accepted document failed to render: %v", err)
			}
			html := string(out)
			// The invariant that holds for EVERY accepted document, whatever
			// it contains: no script, and nothing that would make the browser
			// fetch from another origin.
			for _, forbidden := range []string{"<script", "<iframe", "<object", "<embed", "javascript:", "@import"} {
				if strings.Contains(html, forbidden) {
					t.Errorf("rendered page contains %q", forbidden)
				}
			}
		})
		ran++
	}
	if ran != len(vf.Accept) {
		t.Fatalf("ran %d accept vectors, file has %d", ran, len(vf.Accept))
	}
}

// TestConformanceCorpusHasNoDuplicateNames keeps the corpus honest as it grows:
// two vectors with the same name means one of them is invisible in test output.
func TestConformanceCorpusHasNoDuplicateNames(t *testing.T) {
	vf := loadVectors(t)
	for _, group := range []struct {
		label string
		names []string
	}{
		{"accept", namesOf(vf.Accept, func(v acceptVector) string { return v.Name })},
		{"reject", namesOf(vf.Reject, func(v rejectVector) string { return v.Name })},
		{"render", namesOf(vf.Render, func(v renderVector) string { return v.Name })},
	} {
		seen := map[string]bool{}
		for _, n := range group.names {
			if n == "" {
				t.Errorf("%s: a vector has no name", group.label)
			}
			if seen[n] {
				t.Errorf("%s: duplicate vector name %q", group.label, n)
			}
			seen[n] = true
		}
	}
}

func namesOf[T any](in []T, f func(T) string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}
