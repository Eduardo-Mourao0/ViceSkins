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
	t1, t2 := Trade{TransactionID: 1}, Trade{TransactionID: 2}

	if _, err := s.Start(ctx, t1); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.Check(ctx, t1); o.Offer != OfferPending || o.Item != ItemWithSeller {
		t.Fatalf("observation = %+v, want pending offer with the item at the seller", o)
	}

	now = now.Add(30 * time.Second)
	if _, err := s.Start(ctx, t1); err != nil { // retry
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	o, _ := s.Check(ctx, t1)
	if o.Offer != OfferAccepted || o.Item != ItemWithBuyer || o.TradedAt == nil || !o.TradedAt.Equal(time.Unix(60, 0)) {
		t.Fatalf("observation = %+v, want accepted at t=60s (retry must not reset the clock)", o)
	}
	if err := s.Cancel(ctx, t1); err == nil {
		t.Fatal("cancelling an accepted offer must fail")
	}

	s.Start(ctx, t2)
	if err := s.Cancel(ctx, t2); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if o, _ := s.Check(ctx, t2); o.Offer != OfferDeclined || o.Item != ItemWithSeller {
		t.Fatalf("observation = %+v, want declined with the item at the seller", o)
	}
}
