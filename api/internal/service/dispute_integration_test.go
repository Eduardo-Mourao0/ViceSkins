package service_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// ---- Invariantes de dinheiro ----

func (e *env) platform() int64 {
	e.t.Helper()
	return e.scalar(`SELECT id FROM users WHERE is_platform`)
}

func (e *env) debt(userID int64) int64 {
	e.t.Helper()
	return e.scalar(`SELECT debt_cents FROM users WHERE id = $1`, userID)
}

// assertMoneyConserved: nenhum fluxo cria ou some com dinheiro. Tudo que
// entrou por depósito está em algum saldo (inclusive o da plataforma) ou
// retido em compras ainda em andamento.
func (e *env) assertMoneyConserved() {
	e.t.Helper()
	deposits := e.scalar(`SELECT COALESCE(SUM(amount_cents), 0)::bigint FROM wallet_entries WHERE kind = 'deposit'`)
	balances := e.scalar(`SELECT COALESCE(SUM(balance_cents), 0)::bigint FROM users`)
	escrow := e.scalar(`SELECT COALESCE(SUM(price_cents), 0)::bigint FROM transactions WHERE status IN ('pending', 'sent', 'on_hold')`)
	if balances+escrow != deposits {
		e.t.Fatalf("money not conserved: balances %d + escrow %d != deposits %d", balances, escrow, deposits)
	}
	e.assertLedger(e.platform())
}

// lateDelivery monta o cenário da disputa: vendedor sem API key, prazo vence,
// comprador é estornado, e depois aceita a oferta que ficou aberta.
func (e *env) lateDelivery(seller, buyer, price int64) (txID, itemID int64) {
	e.t.Helper()
	listingID, itemID := e.listing(seller, price)
	e.at("")
	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		e.t.Fatal(err)
	}
	e.expireNow(tx.ID)
	e.expire()
	if e.status(tx.ID) != domain.TxFailed {
		e.t.Fatalf("setup: status %s, want failed (refunded)", e.status(tx.ID))
	}
	return tx.ID, itemID
}

func (e *env) watchClosed() {
	e.t.Helper()
	if err := e.svc.WatchClosedTrades(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) releaseDisputes() {
	e.t.Helper()
	if err := e.svc.ReleaseDueDisputes(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) disputeID(txID int64) int64 {
	e.t.Helper()
	return e.scalar(`SELECT id FROM disputes WHERE transaction_id = $1`, txID)
}

func (e *env) disputeStatus(txID int64) string {
	e.t.Helper()
	var s string
	if err := e.pool.QueryRow(e.ctx, `SELECT status FROM disputes WHERE transaction_id = $1`, txID).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

func setupDisputes(t *testing.T) *env {
	e := setupP2P(t)
	e.provider.set(func(p *fakeProvider) { p.cannotCancel = true })
	return e
}

// ---- Cenários ----

func TestNormalSaleCreditsFeeToPlatform(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)
	before := e.balance(e.platform())

	e.svc.Buy(e.ctx, buyer, listingID)
	e.assertMoneyConserved() // 5.000 retidos na compra
	e.watch()
	e.release()

	if got := e.balance(e.platform()) - before; got != 100 {
		t.Fatalf("platform got %d, want the 2%% fee (100)", got)
	}
	e.assertMoneyConserved()
}

func TestLateDeliveryDisputeSettles(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer := e.user(0), e.user(10_000)
	txID, itemID := e.lateDelivery(seller, buyer, 5_000)
	platformBefore := e.balance(e.platform())

	e.watchClosed() // nada no comprador ainda
	if n := e.scalar(`SELECT count(*) FROM disputes`); n != 0 {
		t.Fatal("no late delivery yet: no dispute")
	}

	e.at(trade.ItemWithBuyer) // aceitou a oferta que ficou aberta
	e.watchClosed()
	if e.disputeStatus(txID) != string(domain.DisputeHolding) {
		t.Fatalf("dispute = %s, want holding", e.disputeStatus(txID))
	}
	if e.balance(buyer) != 5_000 || e.debt(buyer) != 0 {
		t.Fatalf("buyer %d debt %d; want charged again (5000 left), no debt", e.balance(buyer), e.debt(buyer))
	}
	if e.balance(e.platform())-platformBefore != 5_000 {
		t.Fatal("platform must hold the charge until the protection ends")
	}
	if e.owner(itemID) != buyer {
		t.Fatal("the item is with the buyer now")
	}
	if got, _ := e.svc.GetForUser(e.ctx, buyer, txID); got.LateDeliveryAt == nil {
		t.Fatal("transaction must be flagged")
	}
	e.assertMoneyConserved()

	e.releaseDisputes() // HoldDuration 0: protecão já venceu; item segue com o comprador
	if e.disputeStatus(txID) != string(domain.DisputeSettled) {
		t.Fatalf("dispute = %s, want settled", e.disputeStatus(txID))
	}
	if e.balance(seller) != 4_900 || e.balance(e.platform())-platformBefore != 100 {
		t.Fatalf("seller %d, platform +%d; want 4900 and the fee", e.balance(seller), e.balance(e.platform())-platformBefore)
	}
	e.assertLedger(buyer, seller)
	e.assertMoneyConserved()
}

// O comprador gastou o estorno: a cobrança vira dívida, a plataforma garante
// o vendedor, e os depósitos seguintes quitam a dívida antes de virar saldo.
func TestDisputeShortfallIsCoveredByPlatformAndRecoveredFromDeposits(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer := e.user(0), e.user(5_000)
	txID, _ := e.lateDelivery(seller, buyer, 5_000) // estornado: saldo volta a 5.000

	// Gasta o estorno em outra compra (fica retida, aguardando a troca).
	other, _ := e.listing(seller, 5_000)
	e.at("")
	if _, err := e.svc.Buy(e.ctx, buyer, other); err != nil {
		t.Fatal(err)
	}

	e.at(trade.ItemWithBuyer)
	e.watchClosed()
	if e.balance(buyer) != 0 || e.debt(buyer) != 5_000 {
		t.Fatalf("buyer %d debt %d; want nothing to charge and 5000 of debt", e.balance(buyer), e.debt(buyer))
	}
	e.assertMoneyConserved()

	// Com dívida, não compra.
	third, _ := e.listing(seller, 100)
	if _, err := e.svc.Buy(e.ctx, buyer, third); !errors.Is(err, domain.ErrBuyerHasDebt) {
		t.Fatalf("err = %v, want ErrBuyerHasDebt", err)
	}

	platformBefore := e.balance(e.platform())
	e.releaseDisputes()
	if e.disputeStatus(txID) != string(domain.DisputeSettled) || e.balance(seller) != 4_900 {
		t.Fatalf("dispute %s, seller %d; the seller is paid in full even with the shortfall", e.disputeStatus(txID), e.balance(seller))
	}
	if e.balance(e.platform())-platformBefore != -4_900 {
		t.Fatal("the platform covers the payout")
	}
	e.assertMoneyConserved()

	// Depósitos pagam a dívida primeiro.
	e.store.Deposit(e.ctx, buyer, 3_000, "test")
	if e.balance(buyer) != 0 || e.debt(buyer) != 2_000 {
		t.Fatalf("after 3000: balance %d debt %d; want 0 and 2000", e.balance(buyer), e.debt(buyer))
	}
	e.store.Deposit(e.ctx, buyer, 3_000, "test")
	if e.balance(buyer) != 1_000 || e.debt(buyer) != 0 {
		t.Fatalf("after 3000 more: balance %d debt %d; want 1000 and 0", e.balance(buyer), e.debt(buyer))
	}
	if e.balance(e.platform())-platformBefore != 100 {
		t.Fatalf("platform net %d, want the fee (100) once the debt is recovered", e.balance(e.platform())-platformBefore)
	}
	if _, err := e.svc.Buy(e.ctx, buyer, third); err != nil {
		t.Fatalf("debt paid: buying must work again: %v", err)
	}
	e.assertLedger(buyer, seller)
	e.assertMoneyConserved()
}

func TestDisputeReversedDuringProtection(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer := e.user(0), e.user(10_000)
	txID, itemID := e.lateDelivery(seller, buyer, 5_000)
	e.at(trade.ItemWithBuyer)
	e.watchClosed()

	e.at(trade.ItemWithSeller) // a Steam reverteu a troca tardia
	e.releaseDisputes()
	if e.disputeStatus(txID) != string(domain.DisputeReversed) {
		t.Fatalf("dispute = %s, want reversed", e.disputeStatus(txID))
	}
	if e.balance(buyer) != 10_000 || e.balance(seller) != 0 || e.owner(itemID) != seller {
		t.Fatalf("buyer %d seller %d owner %d; want charge undone, seller unpaid, item back",
			e.balance(buyer), e.balance(seller), e.owner(itemID))
	}
	e.assertMoneyConserved()
}

func TestDisputeWaitsForSteamConfirmation(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer := e.user(0), e.user(10_000)
	txID, _ := e.lateDelivery(seller, buyer, 5_000)
	e.at(trade.ItemWithBuyer)
	e.watchClosed()

	e.provider.set(func(p *fakeProvider) { p.checkErr = errors.New("steam down") })
	e.releaseDisputes()
	if e.disputeStatus(txID) != string(domain.DisputeHolding) || e.balance(seller) != 0 {
		t.Fatal("without confirmation, nobody is paid")
	}
}

// O admin arquiva: a cobrança é desfeita. Se parte da dívida já foi paga com
// depósitos, esse valor volta; o restante é perdoado.
func TestAdminDismissUndoesChargeAndPaidDebt(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer, admin := e.user(0), e.user(5_000), e.user(0)
	txID, itemID := e.lateDelivery(seller, buyer, 5_000)
	other, _ := e.listing(seller, 3_000)
	e.at("")
	e.svc.Buy(e.ctx, buyer, other) // sobra 2.000

	e.at(trade.ItemWithBuyer)
	e.watchClosed() // cobra 2.000, 3.000 viram dívida
	e.store.Deposit(e.ctx, buyer, 1_000, "test")
	if e.balance(buyer) != 0 || e.debt(buyer) != 2_000 {
		t.Fatalf("setup: balance %d debt %d", e.balance(buyer), e.debt(buyer))
	}

	id := e.disputeID(txID)
	if err := e.svc.DismissDispute(e.ctx, admin, id, ""); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("no note: err = %v, want ErrInvalidInput", err)
	}
	if err := e.svc.DismissDispute(e.ctx, admin, id, "detecção errada: item igual de outra troca"); err != nil {
		t.Fatal(err)
	}
	// Pagou 2.000 na cobrança + 1.000 de dívida: recebe 3.000; os 2.000 restantes são perdoados.
	if e.balance(buyer) != 3_000 || e.debt(buyer) != 0 || e.owner(itemID) != seller {
		t.Fatalf("buyer %d debt %d owner %d; want 3000 back, no debt, item with seller",
			e.balance(buyer), e.debt(buyer), e.owner(itemID))
	}
	var by int64
	e.pool.QueryRow(e.ctx, `SELECT resolved_by FROM disputes WHERE id = $1`, id).Scan(&by)
	if e.disputeStatus(txID) != string(domain.DisputeDismissed) || by != admin {
		t.Fatalf("dispute %s by %d, want dismissed by the admin", e.disputeStatus(txID), by)
	}
	if err := e.svc.DismissDispute(e.ctx, admin, id, "de novo"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second dismiss: err = %v, want ErrInvalidTransition", err)
	}
	e.assertLedger(buyer, seller)
	e.assertMoneyConserved()
}

// Vários workers veem a mesma entrega tardia: uma disputa só, uma cobrança só.
func TestConcurrentLateDeliveryOpensOneDispute(t *testing.T) {
	e := setupDisputes(t)
	seller, buyer := e.user(0), e.user(20_000)
	txID, _ := e.lateDelivery(seller, buyer, 5_000)
	e.at(trade.ItemWithBuyer)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := e.svc.WatchClosedTrades(e.ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n := e.scalar(`SELECT count(*) FROM disputes WHERE transaction_id = $1`, txID); n != 1 {
		t.Fatalf("%d disputes, want 1", n)
	}
	if n := e.scalar(`SELECT count(*) FROM wallet_entries WHERE transaction_id = $1 AND kind = 'dispute_charge'`, txID); n != 1 {
		t.Fatalf("buyer charged %d times, want once", n)
	}
	if e.balance(buyer) != 15_000 {
		t.Fatalf("buyer = %d, want 15000", e.balance(buyer))
	}
	e.assertMoneyConserved()
}
