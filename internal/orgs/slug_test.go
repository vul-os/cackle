package orgs

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sharedSlugCases mirrors SHARED_CASES in
// web/src/pages/organizers/orgs/slug.test.js. The create-org form shows
// the organiser the web address their name will become BEFORE they
// submit; this function is what actually mints it. If the two rules drift,
// the form promises one address and the product publishes another — so
// both suites assert the same table and TestSlugifyTableIsInSyncWithTheFrontend
// below checks the frontend's copy of it still says the same thing.
var sharedSlugCases = [][2]string{
	{"The Old Biscuit Mill", "the-old-biscuit-mill"},
	{"Neon Nights", "neon-nights"},
	{"  Harbour   Live  ", "harbour-live"},
	{"HARBOUR-LIVE", "harbour-live"},
	{"harbour--live", "harbour-live"},
	{"Rocking the Daisies!", "rocking-the-daisies"},
	{"St Mary's Church Hall", "st-mary-s-church-hall"},
	{"Café del Mar", "caf-del-mar"},
	{"-leading-and-trailing-", "leading-and-trailing"},
	{"Venue 42", "venue-42"},
	{"2026", "2026"},
	{"!!! ???", ""},
	{"", ""},
	{"   ", ""},
	{"---", ""},
}

func TestSlugifyMatchesTheFrontendPreview(t *testing.T) {
	for _, c := range sharedSlugCases {
		if got := slugify(c[0]); got != c[1] {
			t.Errorf("slugify(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

func TestSlugifyIsIdempotent(t *testing.T) {
	for _, c := range sharedSlugCases {
		once := slugify(c[0])
		if twice := slugify(once); twice != once {
			t.Errorf("slugify(slugify(%q)) = %q, want %q — normalising twice must be a no-op", c[0], twice, once)
		}
	}
}

var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestSlugifyOutputShapeAndLengthCap(t *testing.T) {
	inputs := []string{
		"Ünïcödé Vénüé",
		"tabs\tand\nnewlines",
		"<script>alert(1)</script>",
		`a/b\c?d#e&f=g`,
		"100% Pure",
		strings.Repeat("a", 80),
		strings.Repeat("a", maxOrgSlugLen-1) + " bbbb", // cut lands on a hyphen
		strings.Repeat("ab ", 40),
	}
	for _, c := range sharedSlugCases {
		inputs = append(inputs, c[0])
	}

	for _, in := range inputs {
		got := slugify(in)
		if len(got) > maxOrgSlugLen {
			t.Errorf("slugify(%q) is %d chars, over the %d cap", in, len(got), maxOrgSlugLen)
		}
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("slugify(%q) = %q — a slug must never lead or trail with a hyphen", in, got)
		}
		if got != "" && !slugShape.MatchString(got) {
			t.Errorf("slugify(%q) = %q, which is not a valid slug shape", in, got)
		}
	}
}

// TestSlugifyTableIsInSyncWithTheFrontend reads the JS test's own
// SHARED_CASES array and checks it still lists exactly the same
// input/output pairs as the Go table above.
//
// Without this, "both suites assert the same table" is a comment rather
// than a fact: someone could add a case to one side only, both suites stay
// green, and the preview quietly starts lying again. Parsing the JS is
// crude but it is the whole point — the check has to fail when the FILE
// changes, not when a duplicated Go copy of it changes.
func TestSlugifyTableIsInSyncWithTheFrontend(t *testing.T) {
	path := filepath.Join("..", "..", "web", "src", "pages", "organizers", "orgs", "slug.test.js")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v — if the frontend preview moved, update this test rather than deleting it", path, err)
	}

	const marker = "export const SHARED_CASES = ["
	start := strings.Index(string(src), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares SHARED_CASES — the two slug rules can no longer be cross-checked", path)
	}
	rest := string(src)[start+len(marker)-1:]
	end := strings.Index(rest, "\n];")
	if end < 0 {
		t.Fatalf("could not find the end of SHARED_CASES in %s", path)
	}

	literals := jsStringLiterals(rest[:end+2])
	if len(literals)%2 != 0 {
		t.Fatalf("frontend SHARED_CASES has %d strings, which is not an even number of [input, want] pairs", len(literals))
	}
	if len(literals)/2 != len(sharedSlugCases) {
		t.Fatalf("frontend SHARED_CASES has %d cases, Go sharedSlugCases has %d — they must be the same table",
			len(literals)/2, len(sharedSlugCases))
	}
	for i, want := range sharedSlugCases {
		gotIn, gotWant := literals[i*2], literals[i*2+1]
		if gotIn != want[0] || gotWant != want[1] {
			t.Errorf("case %d: frontend has [%q %q], Go has [%q %q]", i, gotIn, gotWant, want[0], want[1])
		}
	}
}

// jsStringLiterals extracts every single- or double-quoted string from a
// JavaScript literal, in source order, decoding the escapes the shared
// table can actually contain. Deliberately small: the input is one array
// of string pairs in a file this repo owns, not arbitrary JS.
func jsStringLiterals(src string) []string {
	var out []string
	for i := 0; i < len(src); i++ {
		q := src[i]
		if q != '\'' && q != '"' {
			continue
		}
		var b strings.Builder
		i++
		for i < len(src) && src[i] != q {
			if src[i] == '\\' && i+1 < len(src) {
				i++
				switch src[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(src[i]) // \' \" \\ and friends
				}
			} else {
				b.WriteByte(src[i])
			}
			i++
		}
		out = append(out, b.String())
	}
	return out
}

// TestCreate_UsesTheDerivedSlugEndToEnd ties the rule to the real
// behaviour: an org created with no explicit slug ends up in the database
// under exactly what slugify said it would.
func TestCreate_UsesTheDerivedSlugEndToEnd(t *testing.T) {
	st := openTestStore(t)
	svc := New(st, nil)
	ctx := context.Background()
	ownerID := mustUser(t, st, "slug-owner@example.com")

	org, err := svc.Create(ctx, CreateInput{Name: "  The Old Biscuit Mill  "}, ownerID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if org.Slug != "the-old-biscuit-mill" {
		t.Fatalf("slug = %q, want %q", org.Slug, "the-old-biscuit-mill")
	}
	if org.Name != "The Old Biscuit Mill" {
		t.Fatalf("name = %q — the name should be trimmed but otherwise untouched", org.Name)
	}

	found, err := st.GetOrgBySlug(ctx, "the-old-biscuit-mill")
	if err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	if found.ID != org.ID {
		t.Fatalf("stored org id = %q, want %q", found.ID, org.ID)
	}

	// An explicitly-supplied messy slug is normalised, not rejected.
	other, err := svc.Create(ctx, CreateInput{Name: "Second Venue", Slug: "  Second   VENUE!! "}, ownerID)
	if err != nil {
		t.Fatalf("Create with explicit slug: %v", err)
	}
	if other.Slug != "second-venue" {
		t.Fatalf("explicit slug normalised to %q, want %q", other.Slug, "second-venue")
	}
}

// TestCreate_RejectsANameThatProducesNoSlug — "!!!" is a name a person
// could plausibly type, and it reduces to nothing. Refuse it rather than
// invent a placeholder handle the organiser never chose and would be stuck
// with in public.
func TestCreate_RejectsANameThatProducesNoSlug(t *testing.T) {
	st := openTestStore(t)
	svc := New(st, nil)
	ownerID := mustUser(t, st, "no-slug@example.com")

	for _, name := range []string{"!!! ???", "---", "..."} {
		if _, err := svc.Create(context.Background(), CreateInput{Name: name}, ownerID); err == nil {
			t.Fatalf("Create(%q): expected an error, got nil", name)
		}
	}
}
