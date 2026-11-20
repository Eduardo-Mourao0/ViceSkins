package trade

import (
	"context"
	"testing"
	"time"
)

func TestSimulated(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(0, 0)
	s := NewSimulated(time.Minute)
	s.now = func() time.Time { return now }

	id, _ := s.SendOffer(ctx, Offer{TransactionID: 1})
	if st, _ := s.OfferState(ctx, id); st != OfferPending {
		t.Fatalf("state = %s, want pending", st)
	}

	now = now.Add(30 * time.Second)
	if _, err := s.SendOffer(ctx, Offer{TransactionID: 1}); err != nil { // retry
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	if st, _ := s.OfferState(ctx, id); st != OfferAccepted {
		t.Fatalf("state = %s, want accepted (retry must not reset the clock)", st)
	}
	if err := s.CancelOffer(ctx, id); err == nil {
		t.Fatal("cancelling an accepted offer must fail")
	}

	id2, _ := s.SendOffer(ctx, Offer{TransactionID: 2})
	if err := s.CancelOffer(ctx, id2); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if st, _ := s.OfferState(ctx, id2); st != OfferDeclined {
		t.Fatalf("state = %s, want declined after cancel", st)
	}
}
