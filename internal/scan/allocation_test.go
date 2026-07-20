package scan

import (
	"errors"
	"testing"
	"time"
)

func sampleAllocation() Allocation {
	return Allocation{
		ID:           "alloc-1",
		EventID:      "event-1",
		DeviceID:     "gate-remote-1",
		TicketTypeID: "tt-ga",
		Count:        50,
		IssuedAt:     time.Unix(1000, 0),
		ExpiresAt:    time.Unix(5000, 0),
		KID:          "k_test",
	}
}

func TestAllocation_SignVerify_RoundTrip(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()

	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}

	if err := VerifyAllocation(a, sig, pub, time.Unix(2000, 0)); err != nil {
		t.Fatalf("VerifyAllocation: %v", err)
	}
}

func TestAllocation_Verify_TamperedField(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}

	tampered := a
	tampered.Count = 999999 // attacker tries to grant themselves more capacity
	if err := VerifyAllocation(tampered, sig, pub, time.Unix(2000, 0)); !errors.Is(err, ErrAllocationBadSignature) {
		t.Fatalf("expected ErrAllocationBadSignature, got %v", err)
	}
}

func TestAllocation_Verify_WrongKey(t *testing.T) {
	pub, priv := mustKeyPair(t)
	otherPub, _ := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	_ = pub
	if err := VerifyAllocation(a, sig, otherPub, time.Unix(2000, 0)); !errors.Is(err, ErrAllocationBadSignature) {
		t.Fatalf("expected ErrAllocationBadSignature for wrong key, got %v", err)
	}
}

func TestAllocation_Verify_Expired(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	if err := VerifyAllocation(a, sig, pub, time.Unix(9000, 0)); !errors.Is(err, ErrAllocationExpired) {
		t.Fatalf("expected ErrAllocationExpired, got %v", err)
	}
}

func TestAllocation_Verify_ExactExpiryBoundary(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	// exp is an exclusive upper bound, same convention as tickets.Verify.
	if err := VerifyAllocation(a, sig, pub, a.ExpiresAt); !errors.Is(err, ErrAllocationExpired) {
		t.Fatalf("expected expired exactly at ExpiresAt, got %v", err)
	}
	if err := VerifyAllocation(a, sig, pub, a.ExpiresAt.Add(-time.Second)); err != nil {
		t.Fatalf("expected valid one second before ExpiresAt, got %v", err)
	}
}

func TestAllocation_Verify_NoExpiryMeansUnbounded(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	a.ExpiresAt = time.Time{}
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	if err := VerifyAllocation(a, sig, pub, time.Unix(999999999, 0)); err != nil {
		t.Fatalf("expected unbounded allocation to remain valid far in the future, got %v", err)
	}
}

func TestAllocation_Verify_BadSignatureEncoding(t *testing.T) {
	pub, _ := mustKeyPair(t)
	a := sampleAllocation()
	if err := VerifyAllocation(a, "not-valid-base64!!!", pub, time.Unix(2000, 0)); err == nil {
		t.Fatalf("expected error for invalid signature encoding")
	}
}

func TestAllocation_Verify_TruncatedSignature(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	if err := VerifyAllocation(a, sig[:len(sig)-10], pub, time.Unix(2000, 0)); err == nil {
		t.Fatalf("expected error for truncated signature")
	}
}

func TestAllocation_Verify_BadKeySizes(t *testing.T) {
	pub, priv := mustKeyPair(t)
	a := sampleAllocation()
	sig, err := SignAllocation(a, priv)
	if err != nil {
		t.Fatalf("SignAllocation: %v", err)
	}
	if err := VerifyAllocation(a, sig, pub[:16], time.Unix(2000, 0)); err == nil {
		t.Fatalf("expected error for undersized public key")
	}
	if _, err := SignAllocation(a, priv[:16]); err == nil {
		t.Fatalf("expected error for undersized private key")
	}
}

func TestAllocationCounter_EnforcesCapacity(t *testing.T) {
	a := sampleAllocation()
	a.Count = 3
	c := NewAllocationCounter(a)

	if !c.TryConsume(1) {
		t.Fatalf("expected first consume to succeed")
	}
	if !c.TryConsume(1) {
		t.Fatalf("expected second consume to succeed")
	}
	if !c.TryConsume(1) {
		t.Fatalf("expected third consume to succeed (hits cap exactly)")
	}
	if c.TryConsume(1) {
		t.Fatalf("expected fourth consume to fail — cap exceeded")
	}
	if got := c.Remaining(); got != 0 {
		t.Fatalf("expected 0 remaining, got %d", got)
	}
}

func TestAllocationCounter_ConsumeMoreThanRemainingInOneCall(t *testing.T) {
	a := sampleAllocation()
	a.Count = 5
	c := NewAllocationCounter(a)

	if !c.TryConsume(3) {
		t.Fatalf("expected consuming 3 of 5 to succeed")
	}
	if c.TryConsume(3) {
		t.Fatalf("expected consuming another 3 (would total 6 > 5) to fail")
	}
	if got := c.Remaining(); got != 2 {
		t.Fatalf("expected 2 remaining after the failed over-consume left state unchanged, got %d", got)
	}
	if !c.TryConsume(2) {
		t.Fatalf("expected consuming the exact remainder to succeed")
	}
	if got := c.Remaining(); got != 0 {
		t.Fatalf("expected 0 remaining, got %d", got)
	}
}

func TestAllocationCounter_ZeroCapacity(t *testing.T) {
	a := sampleAllocation()
	a.Count = 0
	c := NewAllocationCounter(a)
	if c.TryConsume(1) {
		t.Fatalf("expected TryConsume(1) to fail against a zero-capacity allocation")
	}
	if !c.TryConsume(0) {
		t.Fatalf("expected TryConsume(0) to always succeed as a no-op")
	}
}

func TestAllocationCounter_ConcurrentConsume_NeverExceedsCap(t *testing.T) {
	a := sampleAllocation()
	a.Count = 20
	c := NewAllocationCounter(a)

	const workers = 100
	results := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		go func() { results <- c.TryConsume(1) }()
	}
	succeeded := 0
	for i := 0; i < workers; i++ {
		if <-results {
			succeeded++
		}
	}
	if succeeded != 20 {
		t.Fatalf("expected exactly 20 successful consumes out of %d concurrent attempts against cap 20, got %d", workers, succeeded)
	}
	if got := c.Remaining(); got != 0 {
		t.Fatalf("expected 0 remaining, got %d", got)
	}
}
