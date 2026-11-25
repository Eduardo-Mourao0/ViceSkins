package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/payment"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/service"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/testdb"
)

// fakeGateway imita um gateway Pix real: lembra das cobranças, tem relógio
// próprio e pode falhar ao criar. O provedor simulado de verdade não tem
// estado (confirma na hora); este cobre o caminho de um gateway, em que o
// pagamento chega por fora (webhook) e o job de expiração consulta o status.
type fakeGateway struct {
	mu        sync.Mutex
	charges   map[string]*fakeCharge
	seq       int
	createErr error
	ahead     atomic.Int64 // quanto o relógio do gateway está adiantado
}

type fakeCharge struct {
	expiresAt time.Time
	paid      bool
}

func (g *fakeGateway) Name() string { return "fake" }

func (g *fakeGateway) now() time.Time { return time.Now().Add(time.Duration(g.ahead.Load())) }

func (g *fakeGateway) CreateCharge(_ context.Context, req payment.ChargeRequest) (payment.Charge, error) {
	if g.createErr != nil {
		return payment.Charge{}, g.createErr
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	id := fmt.Sprintf("fake_%d", g.seq)
	g.charges[id] = &fakeCharge{expiresAt: req.ExpiresAt}
	code := payment.BRCode{Key: "k@x.invalid", MerchantName: "T", MerchantCity: "T", AmountCents: req.AmountCents, TxID: req.Reference}
	return payment.Charge{ID: id, BRCode: code.String()}, nil
}

func (g *fakeGateway) ChargeStatus(_ context.Context, id string) (payment.Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.charges[id]
	switch {
	case !ok:
		return payment.StatusUnknown, nil
	case c.paid:
		return payment.StatusPaid, nil
	case !g.now().Before(c.expiresAt):
		return payment.StatusExpired, nil
	}
	return payment.StatusPending, nil
}

func (g *fakeGateway) SimulatePayment(_ context.Context, id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, ok := g.charges[id]
	switch {
	case !ok:
		return payment.ErrChargeNotFound
	case !g.now().Before(c.expiresAt):
		return payment.ErrChargeExpired
	}
	c.paid = true
	return nil
}

type depositEnv struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	pix  *fakeGateway
	svc  *service.DepositService
}

func setupDeposits(t *testing.T) *depositEnv {
	pool := testdb.New(t)
	e := &depositEnv{t: t, ctx: context.Background(), pool: pool, pix: &fakeGateway{charges: map[string]*fakeCharge{}}}
	e.svc = service.NewDepositService(repository.NewStore(pool), e.pix)
	return e
}

func (e *depositEnv) create(userID, amount int64) domain.Deposit {
	e.t.Helper()
	d, err := e.svc.Create(e.ctx, userID, amount)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func (e *depositEnv) money(userID int64) (balance, ledger, entries int64) {
	e.t.Helper()
	err := e.pool.QueryRow(e.ctx, `
		SELECT balance_cents,
		       COALESCE((SELECT SUM(amount_cents) FROM wallet_entries WHERE user_id = users.id), 0),
		       (SELECT COUNT(*) FROM wallet_entries WHERE user_id = users.id)
		FROM users WHERE id = $1`, userID).Scan(&balance, &ledger, &entries)
	if err != nil {
		e.t.Fatal(err)
	}
	return
}

// expireInDB faz o depósito vencer no nosso relógio, sem mexer no do provedor.
// Hora da API, não now() do banco: o relógio do container pode estar adiantado.
func (e *depositEnv) expireInDB(id int64) {
	e.t.Helper()
	if _, err := e.pool.Exec(e.ctx, `UPDATE deposits SET expires_at = $2 WHERE id = $1`, id, time.Now().Add(-time.Minute)); err != nil {
		e.t.Fatal(err)
	}
}

// expireAtProvider faz todas as cobranças vencerem também no provedor.
func (e *depositEnv) expireAtProvider() { e.pix.ahead.Store(int64(time.Hour)) }

func (e *depositEnv) chargeID(id int64) string {
	e.t.Helper()
	var charge string
	if err := e.pool.QueryRow(e.ctx, `SELECT provider_charge_id FROM deposits WHERE id = $1`, id).Scan(&charge); err != nil {
		e.t.Fatal(err)
	}
	return charge
}

func TestPixDepositFlow(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)

	d := e.create(user, 15_050)
	if d.Status != domain.DepositPending || !strings.Contains(d.BRCode, "5406150.50") {
		t.Fatalf("new deposit = %+v", d)
	}
	if b, _, _ := e.money(user); b != 0 {
		t.Fatalf("balance before paying = %d, want 0", b)
	}
	if open, err := e.svc.Open(e.ctx, user); err != nil || open == nil || open.ID != d.ID {
		t.Fatalf("open = %+v, %v", open, err)
	}

	// Outro usuário não vê nem paga o Pix alheio.
	other := testdb.NewUser(t, e.pool)
	if _, err := e.svc.Get(e.ctx, other, d.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other user get: %v", err)
	}
	if _, err := e.svc.SimulatePayment(e.ctx, other, d.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other user simulate: %v", err)
	}

	paid, err := e.svc.SimulatePayment(e.ctx, user, d.ID)
	if err != nil || paid.Status != domain.DepositPaid || paid.PaidAt == nil {
		t.Fatalf("paid = %+v, %v", paid, err)
	}
	// Repetir (webhook duplicado, clique duplo) não credita de novo.
	if _, err := e.svc.SimulatePayment(e.ctx, user, d.ID); err != nil {
		t.Fatal(err)
	}
	if b, l, n := e.money(user); b != 15_050 || l != b || n != 1 {
		t.Fatalf("balance %d, ledger %d, entries %d; want 15050 credited once", b, l, n)
	}
	if open, _ := e.svc.Open(e.ctx, user); open != nil {
		t.Fatalf("a paid deposit is not open: %+v", open)
	}
}

func TestConcurrentConfirmationsCreditOnce(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)
	d := e.create(user, 5_000)

	// Simulação e job de expiração ao mesmo tempo, como webhook e job num
	// gateway real: o FOR UPDATE no depósito precisa deixar só um creditar.
	e.expireInDB(d.ID)
	if err := e.pix.SimulatePayment(e.ctx, e.chargeID(d.ID)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.svc.ExpireDeposits(e.ctx)
		}()
	}
	wg.Wait()

	if b, l, n := e.money(user); b != 5_000 || l != b || n != 1 {
		t.Fatalf("balance %d, ledger %d, entries %d; want 5000 credited once", b, l, n)
	}
}

func TestExpireDeposits(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)

	unpaid := e.create(user, 1_000)
	paidLate := e.create(user, 2_000)
	// Pago no provedor, mas o aviso nunca chegou e venceu aqui: credita.
	if err := e.pix.SimulatePayment(e.ctx, e.chargeID(paidLate.ID)); err != nil {
		t.Fatal(err)
	}
	e.expireInDB(unpaid.ID)
	e.expireInDB(paidLate.ID)
	e.expireAtProvider()

	if err := e.svc.ExpireDeposits(e.ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.svc.Get(e.ctx, user, unpaid.ID); d.Status != domain.DepositExpired {
		t.Fatalf("unpaid status = %s, want expired", d.Status)
	}
	if d, _ := e.svc.Get(e.ctx, user, paidLate.ID); d.Status != domain.DepositPaid {
		t.Fatalf("paid-late status = %s, want paid", d.Status)
	}
	if _, err := e.svc.SimulatePayment(e.ctx, user, unpaid.ID); !errors.Is(err, domain.ErrDepositExpired) {
		t.Fatalf("paying an expired pix: %v", err)
	}
	if b, _, n := e.money(user); b != 2_000 || n != 1 {
		t.Fatalf("balance %d, entries %d; want only the paid deposit", b, n)
	}
}

func TestDepositValidationAndLimits(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)

	for _, amount := range []int64{0, -500, domain.MinDepositCents - 1, domain.MaxDepositCents + 1} {
		if _, err := e.svc.Create(e.ctx, user, amount); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("amount %d: %v, want ErrInvalidInput", amount, err)
		}
	}
	for range 3 {
		e.create(user, 1_000)
	}
	if _, err := e.svc.Create(e.ctx, user, 1_000); !errors.Is(err, domain.ErrTooManyOpenDeposits) {
		t.Fatalf("4th open pix: %v, want ErrTooManyOpenDeposits", err)
	}
}

func TestChargeFailureLeavesNoDeposit(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)
	e.pix.createErr = errors.New("gateway down")

	if _, err := e.svc.Create(e.ctx, user, 1_000); err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := e.pool.QueryRow(e.ctx, `SELECT COUNT(*) FROM deposits`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("deposits = %d, %v; want none", n, err)
	}
}

func TestPixDepositPaysDebtFirst(t *testing.T) {
	e := setupDeposits(t)
	user := testdb.NewUser(t, e.pool)
	if _, err := e.pool.Exec(e.ctx, `UPDATE users SET debt_cents = 300 WHERE id = $1`, user); err != nil {
		t.Fatal(err)
	}
	d := e.create(user, 1_000)
	if _, err := e.svc.SimulatePayment(e.ctx, user, d.ID); err != nil {
		t.Fatal(err)
	}
	var balance, debt int64
	if err := e.pool.QueryRow(e.ctx, `SELECT balance_cents, debt_cents FROM users WHERE id = $1`, user).Scan(&balance, &debt); err != nil {
		t.Fatal(err)
	}
	if balance != 700 || debt != 0 {
		t.Fatalf("balance %d, debt %d; want 700 and 0", balance, debt)
	}
}

// Na Vercel, criar o Pix e simular o pagamento podem cair em instâncias
// diferentes da API. O provedor simulado não guarda estado, então funciona.
func TestSimulatedPixWorksAcrossInstances(t *testing.T) {
	e := setupDeposits(t)
	store := repository.NewStore(e.pool)
	instanceA := service.NewDepositService(store, payment.NewSimulated())
	instanceB := service.NewDepositService(store, payment.NewSimulated())
	user := testdb.NewUser(t, e.pool)

	paid, err := instanceA.Create(e.ctx, user, 2_500)
	if err != nil {
		t.Fatal(err)
	}
	if d, err := instanceB.SimulatePayment(e.ctx, user, paid.ID); err != nil || d.Status != domain.DepositPaid {
		t.Fatalf("simulate on another instance: %+v, %v", d, err)
	}

	unpaid, _ := instanceA.Create(e.ctx, user, 1_000)
	e.expireInDB(unpaid.ID)
	if err := instanceB.ExpireDeposits(e.ctx); err != nil {
		t.Fatal(err)
	}
	if d, _ := instanceA.Get(e.ctx, user, unpaid.ID); d.Status != domain.DepositExpired {
		t.Fatalf("unpaid simulated pix = %s, want expired", d.Status)
	}
	if b, _, n := e.money(user); b != 2_500 || n != 1 {
		t.Fatalf("balance %d, entries %d; want only the paid one", b, n)
	}
}
