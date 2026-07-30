package pages

import (
	"strings"
	"testing"
	"time"
)

func sampleEvent() Event {
	return Event{
		Slug:      "midnight-set",
		Title:     "Midnight Set",
		Summary:   "One night only",
		VenueName: "The Assembly",
		Address:   "12 Harrington Street",
		StartsAt:  time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC),
		EndsAt:    time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC),
		Timezone:  "UTC",
		Status:    "published",
		Currency:  "ZAR",
	}
}

func mustRender(t *testing.T, in RenderInput) string {
	t.Helper()
	out, err := Render(in)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(out)
}

// TestRenderEscapesHostText is the core claim of this package: host text is
// text, in every context it lands in, and never markup.
func TestRenderEscapesHostText(t *testing.T) {
	doc := Document{
		Version: Version,
		Blocks: []Block{
			{Type: BlockHeading, Text: `<script>alert(1)</script>`, Level: 2},
			{Type: BlockText, Paragraphs: []string{`"><img src=x onerror=alert(1)>`, `</style><style>body{display:none}`}},
			{Type: BlockLinks, Links: []Link{{Label: `</a><script>alert(1)</script>`, Href: "https://example.org/?a=1&b=2"}}},
			{Type: BlockFAQ, Items: []FAQItem{{Question: `<svg onload=alert(1)>`, Answer: `'; alert(1); //`}}},
		},
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("document should be valid — the payloads are legal plain text: %v", err)
	}

	html := mustRender(t, RenderInput{Doc: doc, Event: sampleEvent(), Nonce: "n0nce"})

	// The exact bytes the host submitted must not appear anywhere in the
	// output. Escaped-and-present is the pass condition; verbatim-anywhere is
	// the failure, and asserting on the verbatim payload rather than on
	// fragments like "onerror=" avoids a false alarm from those same
	// characters appearing safely inside an escaped run of text.
	for _, payload := range []string{
		`<script>alert(1)</script>`,
		`"><img src=x onerror=alert(1)>`,
		`</style><style>body{display:none}`,
		`</a><script>alert(1)</script>`,
		`<svg onload=alert(1)>`,
	} {
		if strings.Contains(html, payload) {
			t.Errorf("rendered page contains host text verbatim (%q) — it reached the browser as markup", payload)
		}
	}
	// No tag opener may originate from host text at all.
	for _, forbidden := range []string{"<script", "</script", "<svg", "<img src=x", "<iframe", "javascript:"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("rendered page contains %q", forbidden)
		}
	}
	// It must still be PRESENT, escaped — dropping it silently would be a
	// different bug (a host whose legitimate text contains "<" gets nothing).
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;svg onload=alert(1)&gt;"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing the escaped form %q", want)
		}
	}
}

// TestRenderHostileThemeNeverEscapesCSS pins the SECOND layer. A colour like
// this can never get past Validate; this test bypasses Validate deliberately
// (calling the template directly through newView) to prove that if it somehow
// did, html/template's CSS escaper still refuses to emit it.
func TestRenderHostileThemeNeverEscapesCSS(t *testing.T) {
	doc := Document{
		Version: Version,
		Theme:   Theme{Background: `red; } body { background: url("http://evil.example/x") } .x{`},
		Blocks:  []Block{{Type: BlockDivider}},
	}
	if err := doc.Validate(); err == nil {
		t.Fatal("layer one failed: Validate accepted a non-hex colour")
	}

	v, err := newView(RenderInput{Doc: doc, Event: sampleEvent(), Nonce: "n0nce"})
	if err != nil {
		t.Fatalf("newView: %v", err)
	}
	var sb strings.Builder
	if err := pageTemplate.Execute(&sb, v); err != nil {
		t.Fatalf("execute: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "evil.example") {
		t.Error("layer two failed: a hostile colour reached the stylesheet intact")
	}
	if !strings.Contains(html, "ZgotmplZ") {
		t.Error("expected html/template's CSS escaper to replace the value with ZgotmplZ")
	}
}

// TestRenderValidThemeIsNotMangled is the other half of the test above: the
// escaper must pass a legitimate colour through untouched, or the whole theming
// feature silently renders as ZgotmplZ.
func TestRenderValidThemeIsNotMangled(t *testing.T) {
	doc := Document{
		Version: Version,
		Theme:   Theme{Background: "#101014", Accent: "#F2C14E", Font: FontSerif, Corners: CornersRound, Direction: DirRTL},
		Blocks:  []Block{{Type: BlockDivider}},
	}
	html := mustRender(t, RenderInput{Doc: doc, Event: sampleEvent(), Nonce: "n0nce"})

	if strings.Contains(html, "ZgotmplZ") {
		t.Fatal("a valid six-digit hex colour was rejected by the CSS escaper")
	}
	for _, want := range []string{"--hp-bg: #101014;", "--hp-accent: #F2C14E;", `dir="rtl"`, "font-serif", "corn-round"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

// TestRenderContainsNoScriptOrThirdPartyReference enforces requirement (f):
// the default page works with zero configuration and makes no third-party
// fetch. A CDN font or a remote image added later fails here.
func TestRenderContainsNoScriptOrThirdPartyReference(t *testing.T) {
	html := mustRender(t, RenderInput{
		Doc:     Default(sampleEvent()),
		Event:   sampleEvent(),
		Tickets: []TicketType{{Name: "General", PriceMinor: 25000}},
		BuyHref: "/events/midnight-set",
		Nonce:   "n0nce",
	})

	for _, forbidden := range []string{"<script", "http://", "https://", "//cdn", "@import", "url(", "<iframe", "<object", "<embed", "<form", "style=\""} {
		if strings.Contains(html, forbidden) {
			t.Errorf("rendered default page contains %q — it must be self-contained, script-free and free of inline style attributes", forbidden)
		}
	}
}

// TestRenderDropsForeignImage is the cross-tenant check inside the renderer
// itself: an image id the event does not own never becomes a /media/ URL, even
// if it somehow reached storage.
func TestRenderDropsForeignImage(t *testing.T) {
	doc := Document{
		Version: Version,
		Blocks: []Block{
			{Type: BlockImage, ImageID: "01OTHERORGSIMAGEID0000000", Alt: "not ours"},
			{Type: BlockImage, ImageID: "01OURSOWNIMAGEID000000000", Alt: "ours"},
		},
	}
	html := mustRender(t, RenderInput{
		Doc:           doc,
		Event:         sampleEvent(),
		AllowedImages: []Image{{ID: "01OURSOWNIMAGEID000000000", Width: 800, Height: 600}},
		Nonce:         "n0nce",
	})

	if strings.Contains(html, "01OTHERORGSIMAGEID0000000") {
		t.Error("an image id outside the event's own gallery was rendered")
	}
	if !strings.Contains(html, "/media/01OURSOWNIMAGEID000000000") {
		t.Error("the event's own image was not rendered")
	}
}

// TestRenderMoneyIsCurrencyAgnostic: no hardcoded symbol, no hardcoded two
// decimal places, and a zero price is a free RSVP rather than "0.00".
func TestRenderMoneyIsCurrencyAgnostic(t *testing.T) {
	cases := []struct {
		currency string
		minor    int64
		want     string
	}{
		{"ZAR", 25000, "250.00"}, // exponent 2
		{"JPY", 25000, "25000"},  // exponent 0 — "cents" is not universal
		{"KWD", 25000, "25.000"}, // exponent 3
	}
	for _, tc := range cases {
		ev := sampleEvent()
		ev.Currency = tc.currency
		html := mustRender(t, RenderInput{
			Doc:     Document{Version: Version, Blocks: []Block{{Type: BlockTickets}}},
			Event:   ev,
			Tickets: []TicketType{{Name: "General", PriceMinor: tc.minor}},
			Nonce:   "n0nce",
		})
		if !strings.Contains(html, tc.want) {
			t.Errorf("%s %d minor: expected %q in the page", tc.currency, tc.minor, tc.want)
		}
		if strings.Contains(html, "$") || strings.Contains(html, "€") {
			t.Errorf("%s: a currency symbol was hardcoded into the page", tc.currency)
		}
	}

	html := mustRender(t, RenderInput{
		Doc:     Document{Version: Version, Blocks: []Block{{Type: BlockTickets}}},
		Event:   sampleEvent(),
		Tickets: []TicketType{{Name: "RSVP", PriceMinor: 0}},
		Nonce:   "n0nce",
	})
	if !strings.Contains(html, "Free") {
		t.Error("a zero-price ticket type should render as free, not as a zero amount")
	}
}

// TestRenderLabelsAreOverridable backs requirement (e): nothing assumes the
// event is in English.
func TestRenderLabelsAreOverridable(t *testing.T) {
	doc := Document{
		Version: Version,
		Lang:    "ar",
		Theme:   Theme{Direction: DirRTL},
		Labels:  Labels{Tickets: "التذاكر", GetTickets: "احصل على التذاكر", Free: "مجاني", When: "متى", Where: "أين"},
		Blocks:  []Block{{Type: BlockDetails}, {Type: BlockTickets}},
	}
	html := mustRender(t, RenderInput{
		Doc:     doc,
		Event:   sampleEvent(),
		Tickets: []TicketType{{Name: "دخول عام", PriceMinor: 0}},
		BuyHref: "/events/midnight-set",
		Nonce:   "n0nce",
	})

	for _, want := range []string{`lang="ar"`, `dir="rtl"`, "التذاكر", "مجاني", "متى"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
	for _, notWant := range []string{">Tickets<", ">When<", ">Where<"} {
		if strings.Contains(html, notWant) {
			t.Errorf("English chrome %q survived a full label override", notWant)
		}
	}
}

// TestRenderCancelledEventAlwaysSaysSo: a host cannot hide a cancellation by
// omitting blocks — the notice is the renderer's, not the document's.
func TestRenderCancelledEventAlwaysSaysSo(t *testing.T) {
	ev := sampleEvent()
	ev.Status = "cancelled"
	html := mustRender(t, RenderInput{
		Doc:   Document{Version: Version, Blocks: nil},
		Event: ev,
		Nonce: "n0nce",
	})
	if !strings.Contains(html, "cancelled") {
		t.Error("a cancelled event rendered no cancellation notice")
	}
}

func TestDefaultDocumentIsValidAndSubmittable(t *testing.T) {
	ev := sampleEvent()
	ev.Description = "Doors at seven.\n\nBring a friend."
	d := Default(ev)
	if err := d.Validate(); err != nil {
		t.Fatalf("the default document must be one a host could have submitted: %v", err)
	}
	if len(d.Blocks) != 3 {
		t.Fatalf("expected text+details+tickets, got %d blocks", len(d.Blocks))
	}
}

// TestDefaultDropsUnrenderableDescriptionParagraphs: the description column
// predates this format and is not held to it, so the default page must not
// become a bypass for content the host-facing API would refuse.
func TestDefaultDropsUnrenderableDescriptionParagraphs(t *testing.T) {
	ev := sampleEvent()
	ev.Description = "Fine paragraph.\n\n‮reversed‬\n\nAlso fine."
	d := Default(ev)
	if err := d.Validate(); err != nil {
		t.Fatalf("Default must always produce a valid document: %v", err)
	}
	for _, b := range d.Blocks {
		for _, p := range b.Paragraphs {
			if strings.ContainsRune(p, 0x202E) {
				t.Error("a bidi override from the description survived into the default document")
			}
		}
	}
}

func TestFormatRangeFallsBackToUTCAndSaysSo(t *testing.T) {
	tr := formatRange(
		time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 23, 30, 0, 0, time.UTC),
		"Not/AZone",
	)
	if !tr.Known {
		t.Fatal("expected a known range")
	}
	if tr.ZoneText != "UTC+00:00" {
		t.Errorf("expected an explicit UTC offset when the zone cannot be resolved, got %q", tr.ZoneText)
	}
	if tr.StartText != "2026-08-14 19:30" {
		t.Errorf("unexpected start text %q", tr.StartText)
	}
	if tr.StartISO != "2026-08-14T19:30:00Z" {
		t.Errorf("unexpected machine-readable start %q", tr.StartISO)
	}
}
