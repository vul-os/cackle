package store

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
)

// The demo ID source exists so `cackle --demo` can be seeded reproducibly and
// docs/screenshots can therefore be diffed. Three properties make that work,
// and all three are easy to break by accident:
//
//	reproducible — two sources with the same seed emit the same sequence
//	ordered      — IDs are sortable primary keys; demo rows that sort
//	               differently per run defeat the whole exercise
//	well-formed  — still a parseable ULID, because everything downstream
//	               treats these as ULIDs
//
// Tested through DeterministicIDSource rather than SetDeterministicIDs on
// purpose: the setter is single-shot and irreversible, so calling it here
// would leave every other test in this package running on sequential keys.

func TestDeterministicIDsAreReproducible(t *testing.T) {
	a, b := DeterministicIDSource(1), DeterministicIDSource(1)
	for i := range 100 {
		x, y := a(), b()
		if x != y {
			t.Fatalf("run %d diverged: %q vs %q — a reseeded demo would not "+
				"reproduce, and the screenshots stop being diffable", i, x, y)
		}
	}
}

func TestDeterministicIDsAreUniqueAndOrdered(t *testing.T) {
	src := DeterministicIDSource(1)
	seen := make(map[string]bool, 1000)
	prev := ""
	for i := range 1000 {
		id := src()
		if seen[id] {
			t.Fatalf("id %d (%q) repeated — these are primary keys", i, id)
		}
		seen[id] = true
		if prev != "" && id <= prev {
			t.Fatalf("id %d (%q) does not sort after %q", i, id, prev)
		}
		prev = id
	}
}

func TestDeterministicIDsAreValidULIDs(t *testing.T) {
	src := DeterministicIDSource(7)
	for range 50 {
		id := src()
		if len(id) != 26 {
			t.Fatalf("%q is %d chars, want 26", id, len(id))
		}
		parsed, err := ulid.Parse(id)
		if err != nil {
			t.Fatalf("%q does not parse as a ULID: %v", id, err)
		}
		// Pin the timestamp field, not just parseability. Zeroing it still
		// yields a parseable 26-char ULID, so a test that stopped at Parse
		// would stay green with the time prefix gone — verified by deleting
		// the copy() that writes it and watching this go red.
		if got := parsed.Time(); got != demoULIDMillis {
			t.Fatalf("%q carries timestamp %d, want the fixed demo instant %d",
				id, got, demoULIDMillis)
		}
	}
}

func TestDifferentSeedsGiveDifferentIDs(t *testing.T) {
	// Otherwise the seed parameter is decoration, and a caller who changed it
	// expecting different data would get the same database.
	a, b := DeterministicIDSource(1), DeterministicIDSource(2)
	if x, y := a(), b(); x == y {
		t.Fatalf("seeds 1 and 2 both produced %q", x)
	}
}

// The real generator must be untouched unless someone deliberately opts in.
// This is the property that keeps sequential, guessable IDs out of a real
// deployment, so it is asserted rather than assumed.
func TestNewIDIsRandomByDefault(t *testing.T) {
	if deterministicIDsSet {
		t.Skip("a prior test in this binary opted into deterministic IDs")
	}
	seen := make(map[string]bool, 200)
	for range 200 {
		id := NewID()
		if seen[id] {
			t.Fatalf("NewID repeated %q — the default source is not random", id)
		}
		seen[id] = true
	}
	// Uniqueness is NOT evidence of randomness, and assuming it was made this
	// test useless: a counter is perfectly unique, so swapping the default
	// source for DeterministicIDSource left the uniqueness check green. Found
	// by making exactly that swap and watching nothing fail.
	//
	// The timestamp is the property that actually separates them. A real ULID
	// carries the wall clock; the demo source carries a fixed instant, so a
	// default that had been silently made deterministic is caught here.
	id, err := ulid.Parse(NewID())
	if err != nil {
		t.Fatalf("NewID did not produce a parseable ULID: %v", err)
	}
	if id.Time() == demoULIDMillis {
		t.Fatal("NewID is carrying the fixed demo timestamp — the DEFAULT id " +
			"source has been made deterministic, so every primary key in a real " +
			"deployment is sequential and guessable")
	}
	skew := time.Since(ulid.Time(id.Time()))
	if skew < -time.Minute || skew > time.Minute {
		t.Errorf("NewID's timestamp is %v from now — it is not reading the clock", skew)
	}
}
