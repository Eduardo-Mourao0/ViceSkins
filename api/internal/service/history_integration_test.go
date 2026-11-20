package service_test

import (
	"errors"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
)

func TestHistoryVisibilityAndRoles(t *testing.T) {
	e := setup(t)
	history := service.NewHistoryService(e.store)
	seller, buyer, stranger := e.user(0), e.user(10_000), e.user(10_000)

	l1, _ := e.listing(seller, 1_000)
	l2, _ := e.listing(seller, 2_000)
	tx1, _ := e.svc.Buy(e.ctx, buyer, l1)
	if _, err := e.svc.Buy(e.ctx, buyer, l2); err != nil {
		t.Fatal(err)
	}

	page, err := history.List(e.ctx, buyer, domain.TxHistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transactions) != 2 || page.NextCursor != nil {
		t.Fatalf("buyer sees %d transactions (next %v), want 2 and no next page", len(page.Transactions), page.NextCursor)
	}
	if got := page.Transactions[0]; got.Seller.ID != seller || got.Buyer.ID != buyer || got.Item.Name == "" {
		t.Fatalf("detail not filled: %+v", got)
	}

	asSeller, _ := history.List(e.ctx, seller, domain.TxHistoryFilter{Role: domain.RoleSeller})
	asBuyer, _ := history.List(e.ctx, seller, domain.TxHistoryFilter{Role: domain.RoleBuyer})
	if len(asSeller.Transactions) != 2 || len(asBuyer.Transactions) != 0 {
		t.Fatalf("seller: %d as seller, %d as buyer; want 2 and 0", len(asSeller.Transactions), len(asBuyer.Transactions))
	}

	if p, _ := history.List(e.ctx, stranger, domain.TxHistoryFilter{}); len(p.Transactions) != 0 {
		t.Fatalf("stranger sees %d transactions", len(p.Transactions))
	}
	if _, err := history.Get(e.ctx, stranger, tx1.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger detail: err = %v, want ErrNotFound", err)
	}
	if _, err := history.Get(e.ctx, seller, tx1.ID); err != nil {
		t.Fatalf("seller detail: %v", err)
	}
}

func TestHistoryCursorPaginationIsStable(t *testing.T) {
	e := setup(t)
	history := service.NewHistoryService(e.store)
	seller, buyer := e.user(0), e.user(100_000)

	var want []int64
	for range 5 {
		l, _ := e.listing(seller, 1_000)
		tx, err := e.svc.Buy(e.ctx, buyer, l)
		if err != nil {
			t.Fatal(err)
		}
		want = append([]int64{tx.ID}, want...) // mais nova primeiro
	}

	var got []int64
	var cursor int64
	for page := 0; ; page++ {
		p, err := history.List(e.ctx, buyer, domain.TxHistoryFilter{Before: cursor, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range p.Transactions {
			got = append(got, tx.ID)
		}

		// Uma compra nova chega entre a 1ª e a 2ª página. Com paginação por
		// offset, ela empurraria tudo e um item apareceria repetido.
		if page == 0 {
			l, _ := e.listing(seller, 1_000)
			if _, err := e.svc.Buy(e.ctx, buyer, l); err != nil {
				t.Fatal(err)
			}
		}

		if p.NextCursor == nil {
			break
		}
		cursor = *p.NextCursor
	}

	if len(got) != len(want) {
		t.Fatalf("got ids %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got ids %v, want %v (no repeats, no gaps)", got, want)
		}
	}
}

func TestHistoryStatusFilterAndSummary(t *testing.T) {
	e := setup(t)
	history := service.NewHistoryService(e.store)
	seller, buyer := e.user(0), e.user(100_000)

	// Uma concluída (10.000), uma cancelada (5.000) e uma em andamento (2.000).
	l1, _ := e.listing(seller, 10_000)
	done, _ := e.svc.Buy(e.ctx, buyer, l1)
	e.svc.Transition(e.ctx, done.ID, domain.TxOnHold)
	e.svc.Transition(e.ctx, done.ID, domain.TxCompleted)

	l2, _ := e.listing(seller, 5_000)
	cancelled, _ := e.svc.Buy(e.ctx, buyer, l2)
	e.svc.CancelByBuyer(e.ctx, buyer, cancelled.ID)

	l3, _ := e.listing(seller, 2_000)
	open, _ := e.svc.Buy(e.ctx, buyer, l3)

	tests := []struct {
		status string
		want   []int64
	}{
		{domain.TxFilterOpen, []int64{open.ID}},
		{domain.TxFilterClosed, []int64{cancelled.ID, done.ID}},
		{string(domain.TxCancelled), []int64{cancelled.ID}},
	}
	for _, tt := range tests {
		p, err := history.List(e.ctx, buyer, domain.TxHistoryFilter{Status: tt.status})
		if err != nil {
			t.Fatal(err)
		}
		var ids []int64
		for _, tx := range p.Transactions {
			ids = append(ids, tx.ID)
		}
		if len(ids) != len(tt.want) || (len(ids) > 0 && ids[0] != tt.want[0]) {
			t.Errorf("status %q: got %v, want %v", tt.status, ids, tt.want)
		}
	}

	if _, err := history.List(e.ctx, buyer, domain.TxHistoryFilter{Status: "lost"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unknown status: err = %v, want ErrInvalidInput", err)
	}
	if _, err := history.List(e.ctx, buyer, domain.TxHistoryFilter{Role: "admin"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unknown role: err = %v, want ErrInvalidInput", err)
	}

	// Comprador: gastou 10.000 + 2.000 (a cancelada foi estornada, não conta).
	b, _ := history.Summary(e.ctx, buyer)
	if b.Purchases != 2 || b.SpentCents != 12_000 {
		t.Errorf("buyer summary = %+v, want 2 purchases / 12000 spent", b)
	}
	// Vendedor: recebeu 10.000 − 2% = 9.800; tem 2.000 − 2% = 1.960 a receber.
	s, _ := history.Summary(e.ctx, seller)
	if s.Sales != 1 || s.EarnedCents != 9_800 || s.FeesPaidCents != 200 || s.PendingEarningsCents != 1_960 {
		t.Errorf("seller summary = %+v", s)
	}
	if s.EarnedCents != e.balance(seller) {
		t.Errorf("earned %d != seller balance %d", s.EarnedCents, e.balance(seller))
	}
}
