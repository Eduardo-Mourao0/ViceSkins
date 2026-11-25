package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/payment"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/repository"
)

// PaymentProvider é o gateway de Pix. Mesma ideia do TradeProvider: o service
// não sabe se o dinheiro é simulado ou de um gateway real.
type PaymentProvider interface {
	Name() string
	CreateCharge(ctx context.Context, req payment.ChargeRequest) (payment.Charge, error)
	ChargeStatus(ctx context.Context, chargeID string) (payment.Status, error)
}

// PaymentSimulator é opcional: só o provedor simulado o implementa. Com um
// gateway real, quem avisa do pagamento é o webhook dele.
type PaymentSimulator interface {
	SimulatePayment(ctx context.Context, chargeID string) error
}

type DepositStore interface {
	ExecTx(ctx context.Context, fn func(q *repository.Queries) error) error
	CreateDeposit(ctx context.Context, arg repository.CreateDepositParams) (repository.Deposit, error)
	SetDepositCharge(ctx context.Context, arg repository.SetDepositChargeParams) (repository.Deposit, error)
	DeleteDeposit(ctx context.Context, id int64) error
	GetDeposit(ctx context.Context, id int64) (repository.Deposit, error)
	GetOpenDeposit(ctx context.Context, arg repository.GetOpenDepositParams) (repository.Deposit, error)
	CountOpenDeposits(ctx context.Context, arg repository.CountOpenDepositsParams) (int64, error)
	ListDepositsToExpire(ctx context.Context, arg repository.ListDepositsToExpireParams) ([]int64, error)
	MarkDepositExpired(ctx context.Context, arg repository.MarkDepositExpiredParams) (int64, error)
}

const (
	defaultPixTTL   = 15 * time.Minute
	maxOpenDeposits = 3 // Pix em aberto por usuário: evita gerar cobranças sem fim
)

type DepositService struct {
	store    DepositStore
	provider PaymentProvider
	pixTTL   time.Duration
	now      func() time.Time
}

func NewDepositService(store DepositStore, provider PaymentProvider) *DepositService {
	return &DepositService{store: store, provider: provider, pixTTL: defaultPixTTL, now: time.Now}
}

// CanSimulate diz se o provedor aceita "simular pagamento" (só o simulado).
func (s *DepositService) CanSimulate() bool {
	_, ok := s.provider.(PaymentSimulator)
	return ok
}

// Create gera uma cobrança Pix. O saldo só entra quando ela for paga.
func (s *DepositService) Create(ctx context.Context, userID, amountCents int64) (domain.Deposit, error) {
	if amountCents < domain.MinDepositCents || amountCents > domain.MaxDepositCents {
		return domain.Deposit{}, fmt.Errorf("%w: deposit must be between %d and %d cents",
			domain.ErrInvalidInput, domain.MinDepositCents, domain.MaxDepositCents)
	}
	now := s.now()
	open, err := s.store.CountOpenDeposits(ctx, repository.CountOpenDepositsParams{UserID: userID, Now: now})
	if err != nil {
		return domain.Deposit{}, fmt.Errorf("count open deposits: %w", err)
	}
	if open >= maxOpenDeposits {
		return domain.Deposit{}, domain.ErrTooManyOpenDeposits
	}

	// Primeiro a linha no banco, para ter o nosso id como referência da
	// cobrança (o externalId de um gateway real). A chamada ao provedor fica
	// fora de transação: rede lenta não pode segurar conexão do banco.
	d, err := s.store.CreateDeposit(ctx, repository.CreateDepositParams{
		UserID: userID, AmountCents: amountCents, Provider: s.provider.Name(),
		ExpiresAt: now.Add(s.pixTTL), Now: now,
	})
	if err != nil {
		return domain.Deposit{}, fmt.Errorf("create deposit: %w", err)
	}

	charge, err := s.provider.CreateCharge(ctx, payment.ChargeRequest{
		Reference:   fmt.Sprintf("VSDEP%d", d.ID),
		AmountCents: amountCents,
		ExpiresAt:   d.ExpiresAt,
		Description: fmt.Sprintf("ViceSkins deposito %d", d.ID),
	})
	if err != nil {
		// Sem cobrança, o depósito nunca existiu. WithoutCancel: se o erro foi
		// o cliente desconectando, a limpeza ainda precisa rodar.
		if delErr := s.store.DeleteDeposit(context.WithoutCancel(ctx), d.ID); delErr != nil {
			slog.Error("delete deposit after charge failure", "deposit", d.ID, "err", delErr)
		}
		return domain.Deposit{}, fmt.Errorf("create pix charge: %w", err)
	}

	d, err = s.store.SetDepositCharge(ctx, repository.SetDepositChargeParams{
		ID: d.ID, ChargeID: &charge.ID, BRCode: charge.BRCode, Now: s.now(),
	})
	if err != nil {
		return domain.Deposit{}, fmt.Errorf("save pix charge: %w", err)
	}
	return toDomainDeposit(d), nil
}

// Get devolve um depósito do próprio usuário. O de outra pessoa é "não existe".
func (s *DepositService) Get(ctx context.Context, userID, id int64) (domain.Deposit, error) {
	d, err := s.store.GetDeposit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.UserID != userID) {
		return domain.Deposit{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Deposit{}, fmt.Errorf("get deposit: %w", err)
	}
	return toDomainDeposit(d), nil
}

// Open devolve o Pix em aberto mais recente do usuário, se houver.
func (s *DepositService) Open(ctx context.Context, userID int64) (*domain.Deposit, error) {
	d, err := s.store.GetOpenDeposit(ctx, repository.GetOpenDepositParams{UserID: userID, Now: s.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get open deposit: %w", err)
	}
	out := toDomainDeposit(d)
	return &out, nil
}

// SimulatePayment paga o Pix no provedor simulado e confirma o depósito, que
// é o que o webhook de um gateway real faria.
func (s *DepositService) SimulatePayment(ctx context.Context, userID, id int64) (domain.Deposit, error) {
	sim, ok := s.provider.(PaymentSimulator)
	if !ok {
		return domain.Deposit{}, domain.ErrFeatureDisabled
	}
	raw, err := s.store.GetDeposit(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && raw.UserID != userID) || (err == nil && raw.ProviderChargeID == nil) {
		return domain.Deposit{}, domain.ErrNotFound // sem charge: a cobrança ainda está sendo criada
	}
	if err != nil {
		return domain.Deposit{}, fmt.Errorf("get deposit: %w", err)
	}
	switch d := toDomainDeposit(raw); {
	case d.Status == domain.DepositPaid:
		return d, nil
	case d.Status == domain.DepositExpired || !s.now().Before(d.ExpiresAt):
		return domain.Deposit{}, domain.ErrDepositExpired
	}

	switch err := sim.SimulatePayment(ctx, *raw.ProviderChargeID); {
	case errors.Is(err, payment.ErrChargeExpired), errors.Is(err, payment.ErrChargeNotFound):
		// Not found: o gateway não conhece a cobrança; tratar como vencida.
		return domain.Deposit{}, domain.ErrDepositExpired
	case err != nil:
		return domain.Deposit{}, fmt.Errorf("simulate payment: %w", err)
	}
	return s.confirm(ctx, id)
}

// confirm credita um depósito pago. Idempotente: o webhook de um gateway real
// chega repetido, e ele e o job de expiração podem chegar juntos. O FOR UPDATE
// na linha do depósito põe as chamadas em fila, e só a primeira credita.
func (s *DepositService) confirm(ctx context.Context, id int64) (domain.Deposit, error) {
	var out repository.Deposit
	err := s.store.ExecTx(ctx, func(q *repository.Queries) error {
		d, err := q.GetDepositForUpdate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock deposit: %w", err)
		}
		if d.Status == string(domain.DepositPaid) {
			out = d
			return nil
		}

		entry, err := repository.ApplyWalletEntry(ctx, q, d.UserID, d.AmountCents, "deposit", nil,
			fmt.Sprintf("Depósito via Pix #%d", d.ID))
		if err != nil {
			return err
		}
		// Dinheiro novo paga primeiro o que o usuário deve à plataforma.
		if err := repository.PayDebt(ctx, q, d.UserID); err != nil {
			return err
		}
		now := s.now()
		if _, err := q.MarkDepositPaid(ctx, repository.MarkDepositPaidParams{
			ID: d.ID, Now: &now, WalletEntryID: &entry.ID,
		}); err != nil {
			return fmt.Errorf("mark deposit paid: %w", err)
		}
		out, err = q.GetDeposit(ctx, id)
		return err
	})
	if err != nil {
		return domain.Deposit{}, err
	}
	return toDomainDeposit(out), nil
}

// ExpireDeposits é um job: Pix vencidos viram "expired". Antes, pergunta ao
// provedor: um Pix pago no último segundo (ou cujo aviso se perdeu) é
// creditado, nunca expirado.
func (s *DepositService) ExpireDeposits(ctx context.Context) error {
	ids, err := s.store.ListDepositsToExpire(ctx, repository.ListDepositsToExpireParams{Now: s.now(), MaxRows: 50})
	if err != nil {
		return fmt.Errorf("list deposits to expire: %w", err)
	}
	for _, id := range ids {
		if err := s.settleExpired(ctx, id); err != nil {
			slog.Error("expire deposit", "deposit", id, "err", err)
		}
	}
	return nil
}

func (s *DepositService) settleExpired(ctx context.Context, id int64) error {
	d, err := s.store.GetDeposit(ctx, id)
	if err != nil {
		return fmt.Errorf("get deposit: %w", err)
	}
	if d.ProviderChargeID != nil {
		st, err := s.provider.ChargeStatus(ctx, *d.ProviderChargeID)
		if err != nil {
			return fmt.Errorf("charge status: %w", err) // sem resposta, sem decisão: tenta na próxima rodada
		}
		switch st {
		case payment.StatusPaid:
			_, err := s.confirm(ctx, id)
			return err
		case payment.StatusPending:
			return nil // o relógio do provedor ainda não venceu
		}
	}
	_, err = s.store.MarkDepositExpired(ctx, repository.MarkDepositExpiredParams{ID: id, Now: s.now()})
	return err
}

func toDomainDeposit(d repository.Deposit) domain.Deposit {
	return domain.Deposit{
		ID:          d.ID,
		UserID:      d.UserID,
		AmountCents: d.AmountCents,
		Status:      domain.DepositStatus(d.Status),
		BRCode:      d.BRCode,
		ExpiresAt:   d.ExpiresAt,
		PaidAt:      d.PaidAt,
		CreatedAt:   d.CreatedAt,
	}
}
