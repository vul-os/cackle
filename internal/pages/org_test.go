package pages

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Organisation-page renderer tests.
//
// The renderer cannot enforce WHICH events it is given — that is the HTTP
// layer's job and is tested there (internal/httpapi/org_page_test.go). What it
// can be held to is everything else: no script, no other origin, host data
// escaped in every context it lands in, an honest empty state, and a page that
// is readable at 390px in both themes.

func orgFixture(events ...OrgEvent) OrgRenderInput {
	return OrgRenderInput{Org: Organisation{Name: "The Bijou"}, Events: events, Nonce: "test-nonce"}
}

func sampleOrgEvent() OrgEvent {
	starts := time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC)
	return OrgEvent{
		Slug: "late-night-quartet", Title: "Late Night Quartet",
		Summary:   "Four players, one room, no amplification.",
		VenueName: "The Bijou", StartsAt: starts, EndsAt: starts.Add(3 * time.Hour),
		Timezone: "UTC",
	}
}

func renderOrg(t *testing.T, in OrgRenderInput) string {
	t.Helper()
	out, err := RenderOrg(in)
	if err != nil {
		t.Fatalf("RenderOrg: %v", err)
	}
	return string(out)
}

// The page is self-contained: no script of any kind, and nothing that would
// make a browser reach another origin. This is the same bar the event page is
// held to in internal/httpapi's TestHostPage_DefaultPageNeedsNoConfiguration,
// and the organisation page is stricter still — it has no /media images, so it
// has no subresources at all.
func TestRenderOrg_IsSelfContainedAndScriptFree(t *testing.T) {
	body := renderOrg(t, orgFixture(sampleOrgEvent()))

	for _, forbidden := range []string{
		"<script", "<iframe", "<object", "<embed", "<form", "@import",
		"http://", "https://", "//", "url(", "src=",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the organisation page contains %q — it must be self-contained and script-free", forbidden)
		}
	}
	if !strings.Contains(body, `nonce="test-nonce"`) {
		t.Error("the single stylesheet does not carry the caller's CSP nonce")
	}
	// One <style> element and no other. A second one would need a second
	// nonce and is a sign somebody started emitting CSS from data.
	if n := strings.Count(body, "<style"); n != 1 {
		t.Errorf("<style> elements = %d, want exactly 1", n)
	}
	if strings.Contains(body, "style=") {
		t.Error("the page carries an inline style attribute; the CSP has no 'unsafe-inline'")
	}
}

// Every value on this page is organisation data or event data, and every one of
// them lands in a context html/template escapes. There is no host-authored
// document here at all, so the attack surface is exactly these fields.
func TestRenderOrg_HostDataIsEscapedInEveryContext(t *testing.T) {
	starts := time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC)
	body := renderOrg(t, OrgRenderInput{
		Org: Organisation{Name: `Bijou "&" <script>alert(1)</script>`},
		Events: []OrgEvent{{
			// A slug that tries to break out of its path segment and add a
			// second attribute to the anchor.
			Slug:      `x" onclick="alert(1)`,
			Title:     `<img src=x onerror=alert(1)>`,
			Summary:   `</p><script>alert(2)</script>`,
			VenueName: `<b>bold</b>`,
			StartsAt:  starts, EndsAt: starts.Add(time.Hour), Timezone: "UTC",
		}},
		Nonce: "n",
	})

	for _, leaked := range []string{
		"<script>alert(1)", "<script>alert(2)", "<img src=x", "<b>bold</b>", `onclick="alert(1)"`,
	} {
		if strings.Contains(body, leaked) {
			t.Errorf("unescaped payload reached the output: %q", leaked)
		}
	}
	// The dangerous characters in the slug must be percent-encoded, not
	// merely HTML-escaped: an escaped `"` still closes the attribute once the
	// parser decodes it.
	if !strings.Contains(body, `href="/h/x%22%20onclick=%22alert%281%29"`) {
		t.Errorf("the slug was not percent-encoded into its own path segment; body:\n%s", body)
	}
}

// The href is built with url.PathEscape, and this is the part html/template
// does NOT do on its own: its URL normaliser escapes quotes and spaces, but it
// leaves '/', '?' and '#' alone, because in a URL those are structure rather
// than data. In a SLUG they are data — an event slug is validated only as
// non-empty — so without PathEscape a slug could climb out of its own path
// segment and rewrite the rest of the URL.
//
// Removing url.PathEscape from org.go must redden this test. It does not
// redden the escaping test above, which is why the two are separate.
func TestRenderOrg_SlugCannotEscapeItsOwnPathSegment(t *testing.T) {
	starts := time.Date(2026, 8, 14, 19, 30, 0, 0, time.UTC)
	body := renderOrg(t, OrgRenderInput{
		Org: Organisation{Name: "O"},
		Events: []OrgEvent{{
			Slug: `../../admin/events?take=1#top`, Title: "T",
			StartsAt: starts, EndsAt: starts.Add(time.Hour), Timezone: "UTC",
		}},
		Nonce: "n",
	})

	if !strings.Contains(body, `href="/h/..%2F..%2Fadmin%2Fevents%3Ftake=1%23top"`) {
		t.Errorf("the slug's '/', '?' and '#' were not percent-encoded, so it can rewrite the URL around it; body:\n%s", body)
	}
	// The properties that assertion is standing in for, stated so a change of
	// encoding that still holds them does not fail spuriously.
	href := body[strings.Index(body, `href="/h/`)+len(`href="`):]
	href = href[:strings.Index(href, `"`)]
	// Exactly the two separators "/h/" contributes, and no more: any third
	// one came out of the slug.
	if strings.Count(href, "/") != 2 {
		t.Errorf("href %q has %d path separators, want the 2 in \"/h/\"; the slug is not confined to one segment",
			href, strings.Count(href, "/"))
	}
	if strings.ContainsAny(href, "?#") {
		t.Errorf("href %q carries a query or fragment introduced by the slug", href)
	}
}

// An organisation between programmes is still that organisation. The empty
// state names it and says the true thing, and does not promise that more is
// coming — nobody adds events to somebody else's box.
func TestRenderOrg_EmptyStateIsHonest(t *testing.T) {
	body := renderOrg(t, orgFixture())

	if !strings.Contains(body, "Nothing on sale right now") {
		t.Error("the empty state does not say that nothing is on sale")
	}
	if !strings.Contains(body, "The Bijou has no tickets on sale at the moment.") {
		t.Error("the empty state does not name the organisation")
	}
	for _, overclaim := range []string{"check back", "coming soon", "new events are added", "more events"} {
		if strings.Contains(strings.ToLower(body), overclaim) {
			t.Errorf("the empty state promises something nobody has promised: %q", overclaim)
		}
	}
	if strings.Contains(body, "What&rsquo;s on") {
		t.Error("an empty programme still rendered a \"What's on\" heading")
	}
}

// The programme lists each event once, links it to its OWN host page, and
// carries a machine-readable time alongside the human one.
func TestRenderOrg_ListsEachEventLinkedToItsHostPage(t *testing.T) {
	a := sampleOrgEvent()
	b := sampleOrgEvent()
	b.Slug, b.Title = "sunday-social", "Sunday Social"
	body := renderOrg(t, orgFixture(a, b))

	for _, want := range []string{
		`href="/h/late-night-quartet"`, "Late Night Quartet",
		`href="/h/sunday-social"`, "Sunday Social",
		`datetime="2026-08-14T19:30:00Z"`, "2026-08-14 19:30",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the programme is missing %q", want)
		}
	}
	if n := strings.Count(body, "<li>"); n != 2 {
		t.Errorf("programme rows = %d, want 2", n)
	}
}

// The page claims nothing beyond this box: no directory, no search, nobody
// nearby, no other organisation. This is the same rule scripts/check-app.mjs
// holds the React app to, held here because a Go template is outside its glob.
func TestRenderOrg_ClaimsNoDiscoveryMechanism(t *testing.T) {
	body := strings.ToLower(renderOrg(t, orgFixture(sampleOrgEvent())))
	for _, claim := range []string{
		"near you", "nearby", "directory", "discover", "search", "browse organisers",
		"global feed", "central", "network of", "other organisers",
	} {
		if strings.Contains(body, claim) {
			t.Errorf("the organisation page implies a discovery mechanism that does not exist: %q", claim)
		}
	}
}

// ── contrast ────────────────────────────────────────────────────────────────

// The palette is MEASURED, not asserted in a comment. Every foreground/ground
// pair the page actually puts on screen, in both themes, is pulled out of the
// rendered stylesheet and run through the WCAG relative-luminance formula. A
// colour change that breaks a pair fails here; the fix is the colour, never the
// threshold.
func TestOrgPage_ContrastMeetsWCAG_AA(t *testing.T) {
	light, dark := orgPalettes(t, renderOrg(t, orgFixture(sampleOrgEvent())))

	pairs := []struct{ fg, bg string }{
		{"op-text", "op-bg"}, {"op-text", "op-surface"}, {"op-text", "op-sunken"},
		{"op-muted", "op-bg"}, {"op-muted", "op-surface"}, {"op-muted", "op-sunken"},
		{"op-link", "op-bg"}, {"op-link", "op-surface"}, {"op-link", "op-sunken"},
	}
	for _, theme := range []struct {
		name    string
		palette map[string]string
	}{{"light", light}, {"dark", dark}} {
		for _, p := range pairs {
			fg, okF := theme.palette[p.fg]
			bg, okB := theme.palette[p.bg]
			if !okF || !okB {
				t.Fatalf("%s: palette is missing --%s or --%s; the stylesheet no longer declares what this test measures", theme.name, p.fg, p.bg)
			}
			got := contrastRatio(t, fg, bg)
			// 4.5:1 — the AA bar for normal-size text. Applied to every pair
			// including the ones only used at larger sizes, because a size is
			// easier to change by accident than a colour.
			if got < 4.5 {
				t.Errorf("%s: --%s (%s) on --%s (%s) = %.2f:1, want >= 4.5:1", theme.name, p.fg, fg, p.bg, bg, got)
			}
		}
	}
}

var cssVarRe = regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-fA-F]{6})\s*;`)

// orgPalettes pulls the two themes out of the rendered stylesheet: everything
// before the dark-scheme media query is the light palette, and the declarations
// inside it are applied on top to produce the dark one — exactly how a browser
// resolves them. Parsing the OUTPUT rather than restating the colours here is
// what makes this a measurement of the shipped page.
func orgPalettes(t *testing.T, body string) (light, dark map[string]string) {
	t.Helper()
	const marker = "@media (prefers-color-scheme: dark)"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("the stylesheet declares no dark-scheme block; this page must be correct in both themes")
	}
	j := strings.Index(body, "</style>")
	if j < 0 || j < i {
		t.Fatal("could not find the end of the stylesheet")
	}

	light = map[string]string{}
	for _, m := range cssVarRe.FindAllStringSubmatch(body[:i], -1) {
		light[m[1]] = m[2]
	}
	if len(light) == 0 {
		t.Fatal("the light palette parsed as empty — this test would then measure nothing")
	}
	dark = map[string]string{}
	for k, v := range light {
		dark[k] = v
	}
	for _, m := range cssVarRe.FindAllStringSubmatch(body[i:j], -1) {
		dark[m[1]] = m[2]
	}
	return light, dark
}

func contrastRatio(t *testing.T, a, b string) float64 {
	t.Helper()
	la, lb := relativeLuminance(t, a), relativeLuminance(t, b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance is the WCAG 2.x definition, written out rather than
// imported so the numbers this test reports come from the formula and not from
// a library that might be measuring something else.
func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("not a six-digit hex colour: %q", hex)
	}
	chan8 := func(s string) float64 {
		n, err := strconv.ParseUint(s, 16, 8)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		c := float64(n) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := chan8(hex[1:3]), chan8(hex[3:5]), chan8(hex[5:7])
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// The measuring apparatus, tested against a pair whose answer is known
// independently: black on white is 21:1 exactly, and a colour against itself is
// 1:1. A contrast test that silently returned a large number for everything
// would pass every assertion above while proving nothing.
func TestOrgPage_ContrastHarnessIsCalibrated(t *testing.T) {
	if got := contrastRatio(t, "#000000", "#ffffff"); math.Abs(got-21) > 0.01 {
		t.Errorf("black on white = %.3f:1, want 21:1 — the harness is not measuring contrast", got)
	}
	if got := contrastRatio(t, "#7f7f7f", "#7f7f7f"); math.Abs(got-1) > 0.001 {
		t.Errorf("a colour against itself = %.3f:1, want 1:1", got)
	}
	// A pair that must FAIL the 4.5 bar, so a harness stuck at "pass" shows up.
	if got := contrastRatio(t, "#ff4848", "#ffffff"); got >= 4.5 {
		t.Errorf("brand red on white = %.2f:1; it is a fill, not an ink, and must not clear the AA bar for body text", got)
	}
}

// ── layout floors ───────────────────────────────────────────────────────────

// Two suite-wide floors, checked against the stylesheet the page actually
// serves: nothing smaller than 12px, and nothing that assumes a viewport wider
// than 390px. The page has one column, no table and no fixed width, so the
// second reduces to "no length is declared in a unit that cannot shrink".
func TestOrgPage_TypeFloorAndNarrowViewport(t *testing.T) {
	body := renderOrg(t, orgFixture(sampleOrgEvent()))
	css := body[strings.Index(body, "<style"):strings.Index(body, "</style>")]

	// Every font-size in rem, against the 17px base declared on <body>.
	const base = 17.0
	remRe := regexp.MustCompile(`font-size:\s*([0-9.]+)rem`)
	for _, m := range remRe.FindAllStringSubmatch(css, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", m[1], err)
		}
		if px := v * base; px < 12 {
			t.Errorf("font-size: %srem is %.1fpx at the 17px base — the floor is 12px", m[1], px)
		}
	}
	// `em` sizes are relative to their own parent; the only one is .zone
	// inside .meta (.9rem = 15.3px), so its floor is 12/15.3 = 0.784.
	emRe := regexp.MustCompile(`font-size:\s*([0-9.]+)em\b`)
	for _, m := range emRe.FindAllStringSubmatch(css, -1) {
		v, _ := strconv.ParseFloat(m[1], 64)
		if px := v * 0.9 * base; px < 12 {
			t.Errorf("font-size: %sem resolves to %.1fpx — the floor is 12px", m[1], px)
		}
	}

	// No fixed pixel width anywhere: a `width: NNNpx` is the one declaration
	// that cannot reflow into a 390px viewport.
	if m := regexp.MustCompile(`(?:^|[^-\w])width:\s*[0-9.]+px`).FindString(css); m != "" {
		t.Errorf("a fixed pixel width would overflow a 390px viewport: %q", strings.TrimSpace(m))
	}
	// The tap target: the programme rows are the only interactive elements,
	// and they are sized by padding on a block-level anchor. 1rem top + 1rem
	// bottom + a 1.15rem line of <h3> clears 44px with room to spare; a
	// declaration that replaced the padding with a fixed height would not.
	if !strings.Contains(css, "ul.programme a {") || !strings.Contains(css, "display: block;") {
		t.Error("the programme rows are no longer full-width block anchors; the 44px target rule depended on that")
	}
}
