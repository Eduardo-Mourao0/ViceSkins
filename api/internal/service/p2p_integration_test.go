package service_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/secrets"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// Regras da verificação P2P (trade_watch.go), com o provider no modo "real":
// Start não envia nada, e a plataforma só observa a Steam.

func setupP2P(t *testing.T, opts ...func(*service.PurchaseConfig)) *env {
	e := setup(t, opts...)
	e.provider.set(func(p *fakeProvider) { p.p2p, p.state = true, trade.OfferPending })
	return e
}

func (e *env) at(location trade.Location) {
	e.provider.set(func(p *fakeProvider) { p.location = location })
}

func (e *env) watch() {
	e.t.Helper()
	if err := e.svc.WatchTrades(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) release() {
	e.t.Helper()
	if err := e.svc.ReleaseDueHolds(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) owner(itemID int64) int64 {
	e.t.Helper()
	return e.scalar(`SELECT owner_id FROM inventory_items WHERE id = $1`, itemID)
}

// ---- Entrega ----

// Sem API key a oferta não é vista: a transação fica pending até o item
// aparecer no inventário do comprador, e então vai direto para on_hold.
func TestDeliveryDetectedByInventoryOnly(t *testing.T) {
	e := setupP2P(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, itemID := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	if tx.Status != domain.TxPending || tx.TradeOfferID != nil {
		t.Fatalf("after buy: %+v, want pending without offer (the seller sends it)", tx)
	}

	e.watch() // item ainda no vendedor
	if e.status(tx.ID) != domain.TxPending {
		t.Fatal("nothing happened on steam: must stay pending")
	}

	e.at(trade.ItemWithBuyer)
	e.watch()
	if e.status(tx.ID) != domain.TxOnHold || e.owner(itemID) != buyer {
		t.Fatalf("status %s, owner %d; want on_hold with the item at the buyer", e.status(tx.ID), e.owner(itemID))
	}
	if e.balance(seller) != 0 {
		t.Fatal("seller must not be paid during the trade protection")
	}
}

func TestItemThatLeftTheSellerIsNotDelivery(t *testing.T) {
	e := setupP2P(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

	// Sumiu do vendedor mas não apareceu no comprador: pode ter ido para
	// outra pessoa. Sem prova positiva, nada muda.
	e.at(trade.ItemElsewhere)
	e.watch()
	if e.status(tx.ID) != domain.TxPending {
		t.Fatalf("status = %s: an item seen nowhere is not a delivery", e.status(tx.ID))
	}
}

// A proteção de 7 dias conta do horário real da troca, não de quando o job viu.
func TestHoldStartsAtSteamTradeTime(t *testing.T) {
	const hold = 7 * 24 * time.Hour
	e := setupP2P(t, func(c *service.PurchaseConfig) { c.HoldDuration = hold })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

	tradedAt := time.Now()
	time.Sleep(50 * time.Millisecond) // o job percebe a troca um pouco depois
	e.provider.set(func(p *fakeProvider) { p.state, p.tradedAt = trade.OfferAccepted, &tradedAt })
	e.watch()

	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.HoldUntil == nil || got.HoldUntil.Sub(tradedAt.Add(hold)).Abs() > time.Millisecond {
		t.Fatalf("hold_until = %v, want trade time + 7d = %v", got.HoldUntil, tradedAt.Add(hold))
	}
}

func TestHoldIgnoresImplausibleTradeTime(t *testing.T) {
	const hold = time.Hour
	e := setupP2P(t, func(c *service.PurchaseConfig) { c.HoldDuration = hold })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

	future := time.Now().Add(24 * time.Hour) // encurtaria nada, alongaria a proteção
	e.provider.set(func(p *fakeProvider) { p.state, p.tradedAt = trade.OfferAccepted, &future })
	before := time.Now()
	e.watch()

	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.HoldUntil == nil || got.HoldUntil.After(before.Add(hold+time.Minute)) {
		t.Fatalf("hold_until = %v: a trade time in the future must be ignored", got.HoldUntil)
	}
}

// ---- Proteção e pagamento ----

func TestReversalDuringHoldDetected(t *testing.T) {
	e := setupP2P(t, func(c *service.PurchaseConfig) { c.HoldDuration = time.Hour })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, itemID := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.at(trade.ItemWithBuyer)
	e.watch() // on_hold

	e.at(trade.ItemWithSeller) // a Steam reverteu a troca
	e.watch()
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.Status != domain.TxFailed || got.FailureReason == nil {
		t.Fatalf("tx = %+v, want failed (reversed)", got)
	}
	if e.owner(itemID) != seller || e.balance(buyer) != 10_000 || e.balance(seller) != 0 {
		t.Fatalf("owner %d, buyer %d, seller %d; want item back, buyer refunded, seller unpaid",
			e.owner(itemID), e.balance(buyer), e.balance(seller))
	}
	e.assertLedger(buyer, seller)
}

func TestReleasePaysOnlyWithConfirmation(t *testing.T) {
	e := setupP2P(t) // HoldDuration 0: a proteção já venceu
	seller, buyer := e.user(0), e.user(10_000)

	newHold := func() int64 {
		listingID, _ := e.listing(seller, 5_000)
		tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
		e.at(trade.ItemWithBuyer)
		e.watch()
		if e.status(tx.ID) != domain.TxOnHold {
			t.Fatalf("setup: status %s, want on_hold", e.status(tx.ID))
		}
		return tx.ID
	}

	t.Run("steam unavailable: does not pay", func(t *testing.T) {
		id := newHold()
		e.provider.set(func(p *fakeProvider) { p.checkErr = errors.New("steam down") })
		e.release()
		e.provider.set(func(p *fakeProvider) { p.checkErr = nil })
		if e.status(id) != domain.TxOnHold || e.balance(seller) != 0 {
			t.Fatalf("status %s, seller %d; must wait for confirmation", e.status(id), e.balance(seller))
		}
		e.at(trade.ItemWithBuyer)
		e.release()
		if e.status(id) != domain.TxCompleted || e.balance(seller) != 4_900 {
			t.Fatalf("status %s, seller %d; want paid once confirmed", e.status(id), e.balance(seller))
		}
	})

	t.Run("item back at the seller: reversed, not paid", func(t *testing.T) {
		id := newHold()
		paid := e.balance(seller)
		e.at(trade.ItemWithSeller)
		e.release()
		if e.status(id) != domain.TxFailed || e.balance(seller) != paid {
			t.Fatalf("status %s; a reversed trade must not pay the seller", e.status(id))
		}
	})

	t.Run("buyer already moved the item on: paid", func(t *testing.T) {
		id := newHold()
		e.at(trade.ItemElsewhere)
		e.release()
		if e.status(id) != domain.TxCompleted {
			t.Fatalf("status %s, want completed (the item did not go back to the seller)", e.status(id))
		}
	})
	e.assertLedger(buyer, seller)
}

// ---- Cancelamento, prazos e entrega tardia ----

func TestSellerMarksSentAndBuyerCannotCancelWithoutKey(t *testing.T) {
	e := setupP2P(t)
	e.provider.set(func(p *fakeProvider) { p.cannotCancel = true })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

	if _, err := e.svc.MarkSentBySeller(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("buyer marking sent: err = %v, want ErrForbidden", err)
	}
	before := time.Now()
	got, err := e.svc.MarkSentBySeller(e.ctx, seller, tx.ID)
	if err != nil || got.Status != domain.TxSent || got.DeadlineAt.Before(before.Add(buyerTimeout)) {
		t.Fatalf("mark sent: %+v, %v; want sent with the buyer's deadline", got, err)
	}

	// A oferta existe e a plataforma não consegue cancelá-la: se o comprador
	// pudesse desistir, receberia o estorno e ainda poderia aceitar a troca.
	if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrCannotCancelOffer) {
		t.Fatalf("err = %v, want ErrCannotCancelOffer", err)
	}
	if e.status(tx.ID) != domain.TxSent || e.balance(buyer) != 5_000 {
		t.Fatal("refused cancel must change nothing")
	}
}

func TestBuyerCancelNeedsTheItemConfirmedAtTheSeller(t *testing.T) {
	e := setupP2P(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

	e.provider.set(func(p *fakeProvider) { p.checkErr = errors.New("steam down") })
	if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrTradeUnverified) {
		t.Fatalf("steam down: err = %v, want ErrTradeUnverified", err)
	}
	e.provider.set(func(p *fakeProvider) { p.checkErr = nil })

	e.at(trade.ItemElsewhere)
	if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrTradeUnverified) {
		t.Fatalf("item left the seller: err = %v, want ErrTradeUnverified", err)
	}
	if e.balance(buyer) != 5_000 {
		t.Fatal("no refund without the item confirmed at the seller")
	}

	e.at(trade.ItemWithSeller)
	if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); err != nil {
		t.Fatal(err)
	}
	if e.balance(buyer) != 10_000 {
		t.Fatal("buyer must be refunded once the item is confirmed at the seller")
	}
	e.assertLedger(buyer, seller)
}

func TestExpiryWaitsWhileTheItemIsUnaccounted(t *testing.T) {
	e := setupP2P(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.expireNow(tx.ID)

	// Saiu do vendedor e não aparece no comprador (ex.: item em proteção que
	// não aparece no inventário público). Estornar aqui seria arriscar pagar
	// duas vezes pelo mesmo item.
	e.at(trade.ItemElsewhere)
	e.expire()
	if e.status(tx.ID) != domain.TxPending || e.balance(buyer) != 5_000 {
		t.Fatalf("status %s, buyer %d; must wait", e.status(tx.ID), e.balance(buyer))
	}

	e.at(trade.ItemWithBuyer) // apareceu: vale a entrega, mesmo depois do prazo
	e.expire()
	if e.status(tx.ID) != domain.TxOnHold {
		t.Fatalf("status = %s, want on_hold", e.status(tx.ID))
	}
}

// Sem API key, a oferta pode continuar aberta depois do estorno por prazo.
// Se o comprador aceitar depois, a transação é marcada e uma disputa é aberta.
func TestLateDeliveryIsFlagged(t *testing.T) {
	e := setupP2P(t)
	e.provider.set(func(p *fakeProvider) { p.cannotCancel = true })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.expireNow(tx.ID)
	e.expire() // item no vendedor → estorno
	if e.status(tx.ID) != domain.TxFailed {
		t.Fatalf("setup: status %s, want failed", e.status(tx.ID))
	}

	if err := e.svc.WatchClosedTrades(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID); got.LateDeliveryAt != nil {
		t.Fatal("nothing happened yet: must not be flagged")
	}

	e.at(trade.ItemWithBuyer) // o comprador aceitou a oferta que ficou aberta
	if err := e.svc.WatchClosedTrades(e.ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.LateDeliveryAt == nil {
		t.Fatal("late delivery must be flagged")
	}
	// A transação continua encerrada; o acerto é feito pela disputa
	// (dispute_integration_test.go): o comprador é cobrado de novo.
	if got.Status != domain.TxFailed || e.balance(buyer) != 5_000 {
		t.Fatalf("status %s, buyer %d; want failed and the buyer charged again", got.Status, e.balance(buyer))
	}
}

// ---- API key do vendedor ----

func TestSellerAPIKeyReachesTheProviderDecrypted(t *testing.T) {
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	e := setupP2P(t, func(c *service.PurchaseConfig) { c.Keys = box })
	seller, other, buyer := e.user(0), e.user(0), e.user(20_000)

	const key = "0123456789ABCDEF0123456789ABCDEF"
	sealed, _ := box.Seal([]byte(key), []byte("steam-api-key:"+strconv.FormatInt(seller, 10)))
	e.exec(`UPDATE users SET steam_api_key_enc = $2 WHERE id = $1`, seller, sealed)

	listingID, _ := e.listing(seller, 5_000)
	e.svc.Buy(e.ctx, buyer, listingID)
	e.provider.mu.Lock()
	got := e.provider.lastTrade.SellerAPIKey
	e.provider.mu.Unlock()
	if got != key {
		t.Fatalf("provider got key %q, want the decrypted seller key", got)
	}

	// A key cifrada copiada para outro usuário não abre (AAD = id do dono).
	e.exec(`UPDATE users SET steam_api_key_enc = $2 WHERE id = $1`, other, sealed)
	listingID, _ = e.listing(other, 5_000)
	e.svc.Buy(e.ctx, buyer, listingID)
	e.provider.mu.Lock()
	got = e.provider.lastTrade.SellerAPIKey
	e.provider.mu.Unlock()
	if got != "" {
		t.Fatal("a key sealed for another user must not be usable")
	}
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatal(err)
	}
}
