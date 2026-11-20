package service_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/testdb"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/trade"
)

// ---- Ambiente de teste ----

type fakeProvider struct {
	mu        sync.Mutex
	sendErr   error
	cancelErr error
	delay     time.Duration // latência simulada da Steam em SendOffer
	state     trade.OfferState
	sends     int         // chamadas a SendOffer, com sucesso ou não
	cancelled []string    // ofertas canceladas
	lastOffer trade.Offer // última oferta recebida
}

func (p *fakeProvider) SendOffer(_ context.Context, o trade.Offer) (string, error) {
	p.mu.Lock()
	p.sends++
	p.lastOffer = o
	delay, sendErr := p.delay, p.sendErr
	p.mu.Unlock()

	time.Sleep(delay) // fora do mutex: chamadas concorrentes se sobrepõem, como na Steam
	if sendErr != nil {
		return "", sendErr
	}
	return fmt.Sprintf("fake-%d", o.TransactionID), nil
}

func (p *fakeProvider) OfferState(context.Context, string) (trade.OfferState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, nil
}

func (p *fakeProvider) CancelOffer(_ context.Context, offerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancelErr != nil {
		return p.cancelErr
	}
	p.cancelled = append(p.cancelled, offerID)
	return nil
}

// set altera o fake sob o mutex (os jobs podem estar lendo em outra goroutine).
func (p *fakeProvider) set(fn func(p *fakeProvider)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn(p)
}

func (p *fakeProvider) sendCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sends
}

type env struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	store    *repository.Store
	provider *fakeProvider
	svc      *service.PurchaseService
}

const (
	feeBps          = 200 // 2%
	maxSendAttempts = 3
	sellerTimeout   = 2 * time.Hour // diferentes entre si: o teste confere qual prazo foi usado
	buyerTimeout    = 3 * time.Hour
)

func setup(t *testing.T) *env {
	pool := testdb.New(t)
	store := repository.NewStore(pool)
	provider := &fakeProvider{state: trade.OfferAccepted}
	// HoldDuration 0: a proteção "acaba" na hora, para testar o fluxo inteiro.
	svc := service.NewPurchaseService(store, provider, service.PurchaseConfig{
		FeeBps: feeBps, HoldDuration: 0, MaxSendAttempts: maxSendAttempts,
		SellerSendTimeout: sellerTimeout, BuyerAcceptTimeout: buyerTimeout,
	})
	return &env{t: t, ctx: context.Background(), pool: pool, store: store, provider: provider, svc: svc}
}

func (e *env) user(balance int64) int64 {
	e.t.Helper()
	id := e.userWithoutTradeURL(balance)
	e.setTradeURL(id, fmt.Sprintf("https://steamcommunity.com/tradeoffer/new/?partner=%d&token=testtokn", id))
	return id
}

func (e *env) setTradeURL(userID int64, url string) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, `UPDATE users SET trade_url = $2 WHERE id = $1`, userID, url); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) userWithoutTradeURL(balance int64) int64 {
	e.t.Helper()
	id := testdb.NewUser(e.t, e.pool)
	if balance > 0 {
		if _, err := e.store.Deposit(e.ctx, id, balance, "test"); err != nil {
			e.t.Fatal(err)
		}
	}
	return id
}

// listing cria um item negociável do vendedor e um anúncio ativo dele.
func (e *env) listing(sellerID, price int64) (listingID, itemID int64) {
	e.t.Helper()
	err := e.pool.QueryRow(e.ctx, `
		INSERT INTO inventory_items (owner_id, asset_id, class_id, instance_id, market_hash_name, name, item_type, icon_url, tradable, marketable)
		VALUES ($1, gen_random_uuid()::text, '1', '0', 'AK-47 | Test', 'AK-47 | Test', 'Rifle', 'x', true, true)
		RETURNING id`, sellerID).Scan(&itemID)
	if err != nil {
		e.t.Fatal(err)
	}
	err = e.pool.QueryRow(e.ctx,
		`INSERT INTO listings (item_id, seller_id, price_cents) VALUES ($1, $2, $3) RETURNING id`,
		itemID, sellerID, price).Scan(&listingID)
	if err != nil {
		e.t.Fatal(err)
	}
	return listingID, itemID
}

func (e *env) balance(userID int64) int64 {
	e.t.Helper()
	a, err := e.store.GetWalletAudit(e.ctx, userID)
	if err != nil {
		e.t.Fatal(err)
	}
	return a.BalanceCents
}

// assertLedger confere a invariante: saldo guardado == soma do ledger.
func (e *env) assertLedger(userIDs ...int64) {
	e.t.Helper()
	for _, id := range userIDs {
		a, err := e.store.GetWalletAudit(e.ctx, id)
		if err != nil {
			e.t.Fatal(err)
		}
		if a.BalanceCents != a.LedgerSumCents {
			e.t.Fatalf("user %d: balance %d != ledger sum %d", id, a.BalanceCents, a.LedgerSumCents)
		}
	}
}

func (e *env) scalar(query string, args ...any) (v int64) {
	e.t.Helper()
	if err := e.pool.QueryRow(e.ctx, query, args...).Scan(&v); err != nil {
		e.t.Fatal(err)
	}
	return v
}

// retryDue faz o retry agendado no outbox vencer agora (sem esperar o backoff).
func (e *env) retryDue(txID int64) {
	e.t.Helper()
	_, err := e.pool.Exec(e.ctx,
		`UPDATE trade_outbox SET next_attempt_at = $2 WHERE transaction_id = $1`, txID, time.Now().Add(-time.Second))
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) status(txID int64) domain.TxStatus {
	e.t.Helper()
	tx, err := e.store.GetTransaction(e.ctx, txID)
	if err != nil {
		e.t.Fatal(err)
	}
	return domain.TxStatus(tx.Status)
}

func (e *env) dispatch() {
	e.t.Helper()
	if err := e.svc.DispatchPendingOffers(e.ctx); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) listingStatus(id int64) (s string) {
	e.t.Helper()
	if err := e.pool.QueryRow(e.ctx, `SELECT status FROM listings WHERE id = $1`, id).Scan(&s); err != nil {
		e.t.Fatal(err)
	}
	return s
}

// ---- Fluxo completo ----

func TestPurchaseFullLifecycle(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, itemID := e.listing(seller, 5_000)

	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != domain.TxSent || tx.FeeCents != 100 || tx.TradeOfferID == nil {
		t.Fatalf("after buy: %+v", tx)
	}
	if e.balance(buyer) != 5_000 || e.listingStatus(listingID) != "sold" {
		t.Fatalf("buyer balance %d, listing %s", e.balance(buyer), e.listingStatus(listingID))
	}

	// Comprador "aceita" a troca → on_hold: item passa para ele, vendedor ainda não recebe.
	if err := e.svc.ProcessSentOffers(e.ctx); err != nil {
		t.Fatal(err)
	}
	tx, _ = e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if tx.Status != domain.TxOnHold || tx.HoldUntil == nil {
		t.Fatalf("after accept: %+v", tx)
	}
	if owner := e.scalar(`SELECT owner_id FROM inventory_items WHERE id = $1`, itemID); owner != buyer {
		t.Fatalf("item owner = %d, want buyer %d", owner, buyer)
	}
	if e.balance(seller) != 0 {
		t.Fatal("seller must not be paid during hold")
	}
	// Durante a proteção, o comprador não pode revender o item.
	listings := service.NewListingService(e.store)
	if _, err := listings.Create(e.ctx, buyer, itemID, 9_000); !errors.Is(err, domain.ErrItemNotTradable) {
		t.Fatalf("relist during hold: err = %v, want ErrItemNotTradable", err)
	}

	// Proteção acaba → completed: vendedor recebe preço − taxa.
	if err := e.svc.ReleaseDueHolds(e.ctx); err != nil {
		t.Fatal(err)
	}
	tx, _ = e.svc.GetForUser(e.ctx, seller, tx.ID)
	if tx.Status != domain.TxCompleted {
		t.Fatalf("after hold: %+v", tx)
	}
	if e.balance(seller) != 4_900 || e.balance(buyer) != 5_000 {
		t.Fatalf("seller %d (want 4900), buyer %d (want 5000)", e.balance(seller), e.balance(buyer))
	}
	// Proteção acabou: agora o comprador pode revender.
	if _, err := listings.Create(e.ctx, buyer, itemID, 9_000); err != nil {
		t.Fatalf("relist after completion: %v", err)
	}
	e.assertLedger(buyer, seller)
}

// ---- Concorrência ----

func TestConcurrentBuyersOnlyOneWins(t *testing.T) {
	e := setup(t)
	seller := e.user(0)

	// A corrida acontece numa janela de microssegundos; uma rodada só pode
	// passar por sorte mesmo com bug. Várias rodadas tornam a falha certa.
	const rounds, buyersPerRound = 15, 20
	for round := range rounds {
		listingID, _ := e.listing(seller, 5_000)
		buyers := make([]int64, buyersPerRound)
		for i := range buyers {
			buyers[i] = e.user(10_000)
		}

		var wg sync.WaitGroup
		start := make(chan struct{}) // largada: todas as goroutines começam juntas
		results := make([]error, buyersPerRound)
		for i, b := range buyers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, results[i] = e.svc.Buy(e.ctx, b, listingID)
			}()
		}
		close(start)
		wg.Wait()

		wins := 0
		for i, err := range results {
			switch {
			case err == nil:
				wins++
				if got := e.balance(buyers[i]); got != 5_000 {
					t.Errorf("round %d: winner balance = %d, want 5000", round, got)
				}
			case errors.Is(err, domain.ErrListingNotActive):
				if got := e.balance(buyers[i]); got != 10_000 {
					t.Errorf("round %d: loser was charged: balance %d", round, got)
				}
			default:
				t.Errorf("round %d: unexpected error: %v", round, err)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: %d buyers won the same listing, want exactly 1", round, wins)
		}
		if c := e.scalar(`SELECT count(*) FROM transactions WHERE listing_id = $1`, listingID); c != 1 {
			t.Fatalf("round %d: %d transactions for the listing, want 1", round, c)
		}
		e.assertLedger(append(buyers, seller)...)
	}
}

func TestSameBuyerCannotOverspendConcurrently(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(1_000)

	// 5 anúncios de R$ 4,00 ao mesmo tempo com R$ 10,00 de saldo: só 2 cabem.
	listings := make([]int64, 5)
	for i := range listings {
		listings[i], _ = e.listing(seller, 400)
	}

	var wg sync.WaitGroup
	results := make([]error, len(listings))
	for i, l := range listings {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = e.svc.Buy(e.ctx, buyer, l)
		}()
	}
	wg.Wait()

	ok, broke := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrInsufficientBalance):
			broke++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 2 || broke != 3 {
		t.Fatalf("ok=%d insufficient=%d, want 2 and 3", ok, broke)
	}
	if b := e.balance(buyer); b != 200 {
		t.Fatalf("buyer balance = %d, want 200", b)
	}
	e.assertLedger(buyer)
}

func TestConcurrentCompletionPaysSellerOnce(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Transition(e.ctx, tx.ID, domain.TxOnHold); err != nil {
		t.Fatal(err)
	}

	// 10 workers tentando concluir a mesma transação ao mesmo tempo.
	const n = 10
	var wg sync.WaitGroup
	results := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = e.svc.Transition(e.ctx, tx.ID, domain.TxCompleted)
		}()
	}
	wg.Wait()

	ok := 0
	for _, err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d completions succeeded, want 1", ok)
	}
	if b := e.balance(seller); b != 4_900 {
		t.Fatalf("seller balance = %d, want 4900 (paid exactly once)", b)
	}
	e.assertLedger(seller, buyer)
}

// ---- Regras da compra ----

func TestBuyRejections(t *testing.T) {
	e := setup(t)
	seller, rich, poor := e.user(0), e.user(100_000), e.user(100)

	active, _ := e.listing(seller, 5_000)
	cancelled, _ := e.listing(seller, 5_000)
	if _, err := e.pool.Exec(e.ctx, `UPDATE listings SET status = 'cancelled' WHERE id = $1`, cancelled); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		buyer     int64
		listingID int64
		want      error
	}{
		{"own listing", seller, active, domain.ErrCannotBuyOwnListing},
		{"insufficient balance", poor, active, domain.ErrInsufficientBalance},
		{"listing not active", rich, cancelled, domain.ErrListingNotActive},
		{"listing does not exist", rich, 999_999_999, domain.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := e.balance(tt.buyer)
			_, err := e.svc.Buy(e.ctx, tt.buyer, tt.listingID)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if after := e.balance(tt.buyer); after != before {
				t.Fatalf("balance changed on rejected purchase: %d → %d", before, after)
			}
		})
	}
	if e.listingStatus(active) != "active" {
		t.Fatal("rejected purchases must not touch the listing")
	}
}

// ---- Falhas e estornos ----

// ---- Outbox: envio da oferta com retry ----

func TestTransientSendErrorIsRetried(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		t.Fatal(err)
	}
	// Falha temporária: a compra continua de pé, esperando o retry.
	if tx.Status != domain.TxPending || e.balance(buyer) != 5_000 {
		t.Fatalf("status %s, buyer %d; want pending and charged", tx.Status, e.balance(buyer))
	}
	if errMsg := e.scalar(`SELECT count(*) FROM trade_outbox WHERE transaction_id = $1 AND last_error IS NOT NULL AND done_at IS NULL`, tx.ID); errMsg != 1 {
		t.Fatal("outbox must keep the row open with the last error")
	}

	// Antes do backoff vencer, o job não tenta de novo.
	e.dispatch()
	if e.provider.sendCount() != 1 {
		t.Fatalf("sends = %d, want 1 (retry is not due yet)", e.provider.sendCount())
	}

	// A Steam volta e o retry vence: a oferta sai.
	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	e.retryDue(tx.ID)
	e.dispatch()
	if s := e.status(tx.ID); s != domain.TxSent {
		t.Fatalf("status = %s, want sent", s)
	}
	if open := e.scalar(`SELECT count(*) FROM trade_outbox WHERE done_at IS NULL`); open != 0 {
		t.Fatalf("%d outbox rows still open after send", open)
	}
	e.assertLedger(buyer, seller)
}

func TestSendGivesUpAfterMaxAttempts(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID) // tentativa 1
	for range maxSendAttempts - 1 {             // tentativas 2..max
		e.retryDue(tx.ID)
		e.dispatch()
	}
	if e.provider.sendCount() != maxSendAttempts {
		t.Fatalf("sends = %d, want %d", e.provider.sendCount(), maxSendAttempts)
	}
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.Status != domain.TxFailed || got.FailureReason == nil {
		t.Fatalf("tx = %+v, want failed with reason", got)
	}
	if e.balance(buyer) != 10_000 {
		t.Fatalf("buyer not refunded: %d", e.balance(buyer))
	}
	// Desistiu de vez: nenhum envio a mais.
	e.retryDue(tx.ID)
	e.dispatch()
	if e.provider.sendCount() != maxSendAttempts {
		t.Fatal("job kept sending after giving up")
	}
	e.assertLedger(buyer, seller)
}

// Simula a API morrendo no meio do envio: a linha fica reservada (lease) e
// ninguém conclui. Quando o lease vence, o job retoma sozinho.
func TestExpiredLeaseIsPickedUpAgain(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	// O 1º envio falha; depois um "worker" reserva a linha e morre sem concluir.
	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("crash") })
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	_, err := e.pool.Exec(e.ctx, `UPDATE trade_outbox
		SET next_attempt_at = $2, locked_until = $3
		WHERE transaction_id = $1`, tx.ID, time.Now().Add(-time.Second), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	e.dispatch()
	if e.status(tx.ID) != domain.TxPending {
		t.Fatal("job must not touch a row with a valid lease")
	}

	if _, err := e.pool.Exec(e.ctx, `UPDATE trade_outbox SET locked_until = $2 WHERE transaction_id = $1`, tx.ID, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	e.dispatch()
	if s := e.status(tx.ID); s != domain.TxSent {
		t.Fatalf("status = %s, want sent after lease expired", s)
	}
}

// 10 workers rodando o job ao mesmo tempo na mesma fila: a oferta sai uma vez.
func TestConcurrentDispatchSendsOnce(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	// Latência realista: abre a janela em que outro worker poderia pegar a
	// mesma linha se o lease não fosse respeitado.
	e.provider.set(func(p *fakeProvider) { p.sendErr, p.delay = nil, 200*time.Millisecond })
	e.retryDue(tx.ID)

	const workers = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			// Cada worker roda várias vezes durante o envio, como um ticker.
			for range 5 {
				if err := e.svc.DispatchPendingOffers(e.ctx); err != nil {
					t.Error(err)
				}
				time.Sleep(30 * time.Millisecond)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n := e.provider.sendCount(); n != 2 { // 1 falha no Buy + 1 envio
		t.Fatalf("SendOffer called %d times, want 2", n)
	}
	if e.status(tx.ID) != domain.TxSent {
		t.Fatalf("status = %s, want sent", e.status(tx.ID))
	}
}

func TestBuyerCancelWhilePendingNeverSends(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	e.provider.set(func(p *fakeProvider) { p.sendErr = errors.New("steam is down") })
	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); err != nil {
		t.Fatal(err)
	}

	e.provider.set(func(p *fakeProvider) { p.sendErr = nil })
	e.retryDue(tx.ID)
	e.dispatch()
	if n := e.provider.sendCount(); n != 1 {
		t.Fatalf("SendOffer called %d times, want 1 (no send after cancel)", n)
	}
	if e.balance(buyer) != 10_000 {
		t.Fatalf("buyer not refunded: %d", e.balance(buyer))
	}
	e.assertLedger(buyer, seller)
}

func TestRejectedOfferFailsImmediately(t *testing.T) {
	e := setup(t)
	e.provider.set(func(p *fakeProvider) { p.sendErr = fmt.Errorf("invalid trade url: %w", trade.ErrRejected) })
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Status != domain.TxFailed || tx.FailureReason == nil {
		t.Fatalf("tx = %+v, want failed with reason", tx)
	}
	if e.balance(buyer) != 10_000 {
		t.Fatalf("buyer not refunded: %d", e.balance(buyer))
	}
	if e.listingStatus(listingID) != "cancelled" {
		t.Fatalf("listing = %s, want cancelled", e.listingStatus(listingID))
	}
	// Ledger: débito da compra + crédito do estorno, nada some.
	if c := e.scalar(`SELECT count(*) FROM wallet_entries WHERE transaction_id = $1`, tx.ID); c != 2 {
		t.Fatalf("%d ledger entries for tx, want 2 (purchase + refund)", c)
	}
	e.assertLedger(buyer, seller)
}

func TestDeclinedOfferRefundsBuyer(t *testing.T) {
	e := setup(t)
	e.provider.state = trade.OfferDeclined
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, err := e.svc.Buy(e.ctx, buyer, listingID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ProcessSentOffers(e.ctx); err != nil {
		t.Fatal(err)
	}
	tx, _ = e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if tx.Status != domain.TxFailed || e.balance(buyer) != 10_000 {
		t.Fatalf("status %s, buyer balance %d; want failed and refunded", tx.Status, e.balance(buyer))
	}
	e.assertLedger(buyer)
}

func TestReversalDuringHoldReturnsItemAndRefunds(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, itemID := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
	if _, err := e.svc.Transition(e.ctx, tx.ID, domain.TxOnHold); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Transition(e.ctx, tx.ID, domain.TxFailed, service.Reason("trade reversed")); err != nil {
		t.Fatal(err)
	}

	if owner := e.scalar(`SELECT owner_id FROM inventory_items WHERE id = $1`, itemID); owner != seller {
		t.Fatalf("item owner = %d, want seller %d", owner, seller)
	}
	if tradable := e.scalar(`SELECT tradable::int FROM inventory_items WHERE id = $1`, itemID); tradable != 1 {
		t.Fatal("item returned to seller must be tradable again")
	}
	if e.balance(buyer) != 10_000 || e.balance(seller) != 0 {
		t.Fatalf("buyer %d seller %d; want 10000 and 0", e.balance(buyer), e.balance(seller))
	}
	e.assertLedger(buyer, seller)
}

// ---- Cancelamento e transições inválidas ----

func TestCancelByBuyer(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)

	t.Run("buyer cancels before trade is accepted", func(t *testing.T) {
		listingID, _ := e.listing(seller, 5_000)
		tx, _ := e.svc.Buy(e.ctx, buyer, listingID)

		if _, err := e.svc.CancelByBuyer(e.ctx, seller, tx.ID); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("seller cancel: err = %v, want ErrForbidden", err)
		}
		got, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID)
		if err != nil || got.Status != domain.TxCancelled {
			t.Fatalf("cancel: %+v, %v", got, err)
		}
		if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("second cancel: err = %v, want ErrInvalidTransition", err)
		}
		if e.balance(buyer) != 10_000 {
			t.Fatalf("buyer not refunded: %d", e.balance(buyer))
		}
		if !slices.Contains(e.provider.cancelled, *tx.TradeOfferID) {
			t.Fatal("trade offer must be cancelled on Steam")
		}
	})

	t.Run("no refund if the offer cannot be cancelled", func(t *testing.T) {
		listingID, _ := e.listing(seller, 5_000)
		tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
		before := e.balance(buyer)

		e.provider.set(func(p *fakeProvider) { p.cancelErr = errors.New("steam is down") })
		defer e.provider.set(func(p *fakeProvider) { p.cancelErr = nil })

		if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); err == nil {
			t.Fatal("cancel must fail when the offer cannot be cancelled")
		}
		if e.status(tx.ID) != domain.TxSent || e.balance(buyer) != before {
			t.Fatal("failed cancel must not change status or refund")
		}
	})

	t.Run("cannot cancel after trade is accepted", func(t *testing.T) {
		listingID, _ := e.listing(seller, 5_000)
		tx, _ := e.svc.Buy(e.ctx, buyer, listingID)
		if _, err := e.svc.Transition(e.ctx, tx.ID, domain.TxOnHold); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.CancelByBuyer(e.ctx, buyer, tx.ID); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Fatalf("err = %v, want ErrInvalidTransition", err)
		}
	})

	e.assertLedger(buyer, seller)
}

func TestInvalidTransitionChangesNothing(t *testing.T) {
	e := setup(t)
	seller, buyer := e.user(0), e.user(10_000)
	listingID, _ := e.listing(seller, 5_000)

	tx, _ := e.svc.Buy(e.ctx, buyer, listingID) // sent
	if _, err := e.svc.Transition(e.ctx, tx.ID, domain.TxCompleted); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("sent → completed: err = %v, want ErrInvalidTransition", err)
	}
	if e.balance(seller) != 0 {
		t.Fatal("invalid transition must not pay the seller")
	}
	got, _ := e.svc.GetForUser(e.ctx, buyer, tx.ID)
	if got.Status != domain.TxSent {
		t.Fatalf("status = %s, want sent", got.Status)
	}
}
