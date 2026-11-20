package service_test

import (
	"errors"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// ---- Trade URL ----

func TestBuyRequiresTradeURL(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.userWithoutTradeURL(10_000)
	listingID, _ := e.listing(seller, 5_000)

	if _, err := e.svc.Buy(e.ctx, buyer, listingID); !errors.Is(err, domain.ErrTradeURLRequired) {
		t.Fatalf("err = %v, want ErrTradeURLRequired", err)
	}
	if e.balance(buyer) != 10_000 || e.listingStatus(listingID) != "active" {
		t.Fatal("rejected purchase must not charge the buyer or touch the listing")
	}
}

// A entrega usa a URL do momento da compra: trocar a URL depois (ex.: conta
// invadida) não desvia um item que já está a caminho.
func TestTradeURLIsSnapshottedAtPurchase(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	const original = "https://steamcommunity.com/tradeoffer/new/?partner=111&token=original"
	e.setTradeURL(buyer, original)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID) // fica pending: envio falhou
	e.setTradeURL(buyer, "https://steamcommunity.com/tradeoffer/new/?partner=222&token=attacker")

	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	e.retryDue(tx.ID)
	e.dispatch()

	e.provider.mu.Lock()
	got := e.provider.lastTrade.BuyerTradeURL
	e.provider.mu.Unlock()
	if got != original {
		t.Fatalf("offer went to %q, want the url at purchase time %q", got, original)
	}
}

// ---- Prazos ----

func (e *env) expireNow(txID int64) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, `UPDATE transactions SET deadline_at = $2 WHERE id = $1`,
		txID, time.Now().Add(-time.Second)); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) expire() {
	e.t.Helper()
	if err := e.svc.ExpireDeadlines(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func TestDeadlinesAreSetPerStep(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	before := time.Now()
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	if tx.DeadlineAt == nil || tx.DeadlineAt.Before(before.Add(sellerTimeout)) {
		t.Fatalf("pending deadline = %v, want ≈ now + %s", tx.DeadlineAt, sellerTimeout)
	}

	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	e.retryDue(tx.ID)
	before = time.Now()
	e.dispatch()
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.Status != domain.TxSent || got.DeadlineAt == nil || got.DeadlineAt.Before(before.Add(buyerTimeout)) {
		t.Fatalf("sent: status %s deadline %v, want ≈ now + %s", got.Status, got.DeadlineAt, buyerTimeout)
	}
}

func TestSellerMissesDeadline(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.expire() // prazo ainda não venceu
	if e.status(tx.ID) != domain.TxPending {
		t.Fatal("transaction expired before its deadline")
	}

	e.expireNow(tx.ID)
	e.expire()
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.Status != domain.TxFailed || got.FailureReason == nil {
		t.Fatalf("tx = %+v, want failed with reason", got)
	}
	if e.balance(buyer) != 10_000 || e.listingStatus(listingID) != "cancelled" {
		t.Fatalf("buyer %d, listing %s; want refunded and cancelled", e.balance(buyer), e.listingStatus(listingID))
	}
	// O outbox fechou junto: o job não envia mais nada.
	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	e.retryDue(tx.ID)
	e.dispatch()
	if e.provider.sendCount() != 1 {
		t.Fatal("offer sent after the transaction expired")
	}
	e.assertLedger(buyer, seller)
}

func TestBuyerMissesDeadline(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.state = trade.OfferPending })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID) // sent
	e.expireNow(tx.ID)
	e.expire()

	if e.status(tx.ID) != domain.TxFailed || e.balance(buyer) != 10_000 {
		t.Fatalf("status %s, buyer %d; want failed and refunded", e.status(tx.ID), e.balance(buyer))
	}
	if len(e.provider.cancelled) != 1 || e.provider.cancelled[0] != *tx.TradeOfferID {
		t.Fatalf("cancelled = %v, want the offer cancelled on steam", e.provider.cancelled)
	}
	e.assertLedger(buyer, seller)
}

// Se a Steam não confirma o cancelamento da oferta, não estorna: o comprador
// ainda poderia aceitar e ficar com item e dinheiro. Tenta no próximo tick.
func TestExpiryWaitsForOfferCancellation(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.state = trade.OfferPending })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.expireNow(tx.ID)

	e.provider.set(func(p *fakeProvider) { p.cancelErr = errors.New("steam is down") })
	e.expire()
	if e.status(tx.ID) != domain.TxSent || e.balance(buyer) != 5_000 {
		t.Fatal("must not refund while the offer is still open on steam")
	}

	e.provider.set(func(p *fakeProvider) { p.cancelErr = nil })
	e.expire()
	if e.status(tx.ID) != domain.TxFailed || e.balance(buyer) != 10_000 {
		t.Fatal("expiry must succeed once steam confirms the cancellation")
	}
	e.assertLedger(buyer, seller)
}

func TestExpiryIgnoresTransactionsThatMovedOn(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	if err := e.svc.WatchTrades(e.ctx); err != nil { // → on_hold
		t.Fatal(err)
	}
	e.expireNow(tx.ID) // prazo antigo vencido, mas a troca já foi aceita
	e.expire()

	if e.status(tx.ID) != domain.TxOnHold || e.balance(buyer) != 5_000 {
		t.Fatalf("status %s, buyer %d; an accepted trade must not expire", e.status(tx.ID), e.balance(buyer))
	}
}

// O comprador aceita a oferta bem quando o prazo vence. O job de prazos olha
// a Steam (item ainda no vendedor), e no meio da decisão a troca acontece e o
// job de verificação move a transação para on_hold. Sem a guarda From(status),
// o estorno decidido com a observação velha seria aplicado em cima de on_hold
// (on_hold → failed é válido): comprador com item e dinheiro.
//
// Roda nos dois modos: com API key (a Steam recusa cancelar oferta aceita) e
// sem (não há como cancelar; só a guarda protege).
func TestExpiryRacesWithAcceptance(t *testing.T) {
	for _, mode := range []struct {
		name         string
		cannotCancel bool
	}{{"with api key", false}, {"without api key", true}} {
		t.Run(mode.name, func(t *testing.T) {
			e := setup(t)
			seller := e.user(0)
			e.provider.set(func(p *fakeProvider) { p.cannotCancel = mode.cannotCancel })

			const rounds = 30
			outcomes := map[domain.TxStatus]int{}
			for range rounds {
				buyer := e.user(10_000)
				listingID, itemID := e.listing(seller, 5_000)

				e.provider.set(func(p *fakeProvider) { p.state = trade.OfferPending })
				tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
				e.expireNow(tx.ID)

				start := make(chan struct{})
				var wg sync.WaitGroup
				for _, job := range []func() error{
					func() error {
						// O comprador aceita em algum momento aleatório e o job vê.
						time.Sleep(time.Duration(rand.IntN(8000)) * time.Microsecond)
						e.provider.set(func(p *fakeProvider) { p.state = trade.OfferAccepted })
						return e.svc.WatchTrades(e.ctx)
					},
					func() error { return e.svc.ExpireDeadlines(e.ctx) },
				} {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						if err := job(); err != nil {
							t.Error(err)
						}
					}()
				}
				close(start)
				wg.Wait()

				status := e.status(tx.ID)
				outcomes[status]++
				owner := e.scalar(`SELECT owner_id FROM inventory_items WHERE id = $1`, itemID)
				entries := e.scalar(`SELECT count(*) FROM wallet_entries WHERE transaction_id = $1`, tx.ID)
				switch status {
				case domain.TxOnHold: // aceita: item com o comprador, sem estorno
					if owner != buyer || e.balance(buyer) != 5_000 || entries != 1 {
						t.Fatalf("accepted: owner %d, buyer balance %d, entries %d", owner, e.balance(buyer), entries)
					}
				case domain.TxFailed: // expirou antes do aceite: estorno, item com o vendedor
					// Passou por on_hold e depois falhou: o estorno da observação velha
					// foi aplicado em cima de uma troca aceita (o bug da guarda From).
					if held := e.scalar(`SELECT (hold_until IS NOT NULL)::int FROM transactions WHERE id = $1`, tx.ID); held == 1 {
						t.Fatal("expiry refunded a trade that had already been accepted (on_hold → failed)")
					}
					if owner != seller || e.balance(buyer) != 10_000 || entries != 2 {
						t.Fatalf("expired: owner %d, buyer balance %d, entries %d", owner, e.balance(buyer), entries)
					}
				default:
					t.Fatalf("status = %s, want on_hold or failed", status)
				}
				e.assertLedger(buyer, seller)
			}
			t.Logf("outcomes over %d rounds: %v", rounds, outcomes)
		})
	}
}

// Versão determinística da corrida acima: o comprador aceita exatamente entre
// a consulta do job de prazos (item no vendedor) e a ação dele (estorno).
func TestExpiryDoesNotRefundTradeAcceptedMidDecision(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.state, p.cannotCancel = trade.OfferPending, true })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, itemID := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.expireNow(tx.ID)

	e.provider.set(func(p *fakeProvider) {
		p.afterCheck = func() {
			e.provider.set(func(p *fakeProvider) { p.state = trade.OfferAccepted })
			if err := e.svc.WatchTrades(e.ctx); err != nil { // → on_hold
				t.Error(err)
			}
		}
	})
	e.expire()

	if s := e.status(tx.ID); s != domain.TxOnHold {
		t.Fatalf("status = %s, want on_hold: a stale observation must not refund an accepted trade", s)
	}
	if owner := e.scalar(`SELECT owner_id FROM inventory_items WHERE id = $1`, itemID); owner != buyer || e.balance(buyer) != 5_000 {
		t.Fatalf("owner %d, buyer balance %d; want item with buyer and no refund", owner, e.balance(buyer))
	}
	e.assertLedger(buyer, seller)
}
