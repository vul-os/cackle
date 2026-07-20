package scan

import (
	"context"
	"sync"
	"testing"
	"time"
)

// seenSetFactories lets every SeenSet-generic test run against both
// backends without duplicating the test body.
func seenSetFactories(t *testing.T) map[string]func() SeenSet {
	return map[string]func() SeenSet{
		"memory": func() SeenSet { return NewMemorySeenSet() },
		"sqlite": func() SeenSet {
			s, err := OpenSQLiteSeenSet(":memory:")
			if err != nil {
				t.Fatalf("OpenSQLiteSeenSet: %v", err)
			}
			t.Cleanup(func() { s.Close() })
			return s
		},
	}
}

func TestSeenSet_FirstScanWins(t *testing.T) {
	for name, factory := range seenSetFactories(t) {
		t.Run(name, func(t *testing.T) {
			seen := factory()
			ctx := context.Background()
			now := time.Unix(1000, 0)

			first, err := seen.MarkSeen(ctx, "ticket-1", now)
			if err != nil {
				t.Fatalf("MarkSeen: %v", err)
			}
			if !first {
				t.Fatalf("expected first MarkSeen to report firstSeen=true")
			}

			second, err := seen.MarkSeen(ctx, "ticket-1", now.Add(time.Minute))
			if err != nil {
				t.Fatalf("MarkSeen: %v", err)
			}
			if second {
				t.Fatalf("expected second MarkSeen to report firstSeen=false")
			}
		})
	}
}

func TestSeenSet_IndependentTicketsIndependent(t *testing.T) {
	for name, factory := range seenSetFactories(t) {
		t.Run(name, func(t *testing.T) {
			seen := factory()
			ctx := context.Background()
			now := time.Unix(1000, 0)

			for _, id := range []string{"a", "b", "c"} {
				first, err := seen.MarkSeen(ctx, id, now)
				if err != nil {
					t.Fatalf("MarkSeen(%s): %v", id, err)
				}
				if !first {
					t.Fatalf("expected ticket %s to be a fresh first scan", id)
				}
			}
		})
	}
}

func TestSeenSet_Seen(t *testing.T) {
	for name, factory := range seenSetFactories(t) {
		t.Run(name, func(t *testing.T) {
			seen := factory()
			ctx := context.Background()

			ok, err := seen.Seen(ctx, "ticket-1")
			if err != nil {
				t.Fatalf("Seen: %v", err)
			}
			if ok {
				t.Fatalf("expected Seen=false before any MarkSeen")
			}

			if _, err := seen.MarkSeen(ctx, "ticket-1", time.Unix(1000, 0)); err != nil {
				t.Fatalf("MarkSeen: %v", err)
			}

			ok, err = seen.Seen(ctx, "ticket-1")
			if err != nil {
				t.Fatalf("Seen: %v", err)
			}
			if !ok {
				t.Fatalf("expected Seen=true after MarkSeen")
			}
		})
	}
}

func TestSeenSet_ConcurrentMarkSeen_ExactlyOneFirst(t *testing.T) {
	for name, factory := range seenSetFactories(t) {
		t.Run(name, func(t *testing.T) {
			seen := factory()
			ctx := context.Background()

			const n = 100
			var wg sync.WaitGroup
			var firstCount int32
			var mu sync.Mutex
			wg.Add(n)
			for i := 0; i < n; i++ {
				go func() {
					defer wg.Done()
					first, err := seen.MarkSeen(ctx, "shared-ticket", time.Unix(1000, 0))
					if err != nil {
						t.Errorf("MarkSeen: %v", err)
						return
					}
					if first {
						mu.Lock()
						firstCount++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if firstCount != 1 {
				t.Fatalf("expected exactly 1 winner out of %d concurrent MarkSeen calls, got %d", n, firstCount)
			}
		})
	}
}

func TestMemorySeenSet_FirstSeenAt(t *testing.T) {
	seen := NewMemorySeenSet()
	if _, ok := seen.FirstSeenAt("ticket-1"); ok {
		t.Fatalf("expected no first-seen time before MarkSeen")
	}
	at := time.Unix(1234, 0)
	if _, err := seen.MarkSeen(context.Background(), "ticket-1", at); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	got, ok := seen.FirstSeenAt("ticket-1")
	if !ok || !got.Equal(at) {
		t.Fatalf("expected first-seen time %v, got %v (ok=%v)", at, got, ok)
	}
}

func TestOpenSQLiteSeenSet_IsolatedInMemoryInstances(t *testing.T) {
	s1, err := OpenSQLiteSeenSet(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteSeenSet: %v", err)
	}
	defer s1.Close()
	s2, err := OpenSQLiteSeenSet(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteSeenSet: %v", err)
	}
	defer s2.Close()

	ctx := context.Background()
	if _, err := s1.MarkSeen(ctx, "ticket-1", time.Unix(1000, 0)); err != nil {
		t.Fatalf("MarkSeen on s1: %v", err)
	}
	seenOnS2, err := s2.Seen(ctx, "ticket-1")
	if err != nil {
		t.Fatalf("Seen on s2: %v", err)
	}
	if seenOnS2 {
		t.Fatalf("expected s2 to be an isolated database from s1, but it saw s1's write")
	}
}

func TestSQLiteSeenSet_PersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/seen.db"

	s1, err := OpenSQLiteSeenSet(path)
	if err != nil {
		t.Fatalf("OpenSQLiteSeenSet: %v", err)
	}
	if _, err := s1.MarkSeen(context.Background(), "ticket-1", time.Unix(1000, 0)); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := OpenSQLiteSeenSet(path)
	if err != nil {
		t.Fatalf("re-OpenSQLiteSeenSet: %v", err)
	}
	defer s2.Close()
	ok, err := s2.Seen(context.Background(), "ticket-1")
	if err != nil {
		t.Fatalf("Seen: %v", err)
	}
	if !ok {
		t.Fatalf("expected ticket-1 to still be seen after reopening the same file-backed db")
	}
}
