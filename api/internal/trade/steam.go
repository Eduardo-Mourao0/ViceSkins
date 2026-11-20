package trade

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

// SteamAPI é o que o provider usa do cliente da Steam (*steam.Client).
type SteamAPI interface {
	GetInventory(ctx context.Context, steamID string) ([]steam.InventoryItem, error)
	GetSentOffers(ctx context.Context, apiKey string, since time.Time) ([]steam.TradeOffer, error)
	GetTradeOffer(ctx context.Context, apiKey, offerID string) (steam.TradeOffer, error)
	CancelTradeOffer(ctx context.Context, apiKey, offerID string) error
}

// Steam é o provider real, híbrido:
//
//   - Inventário (sempre): onde o item está. Com o vendedor, pelo AssetID
//     original (ou pela impressão digital, se voltou numa reversão); com o
//     comprador, pela impressão digital. Não precisa de credencial nenhuma,
//     mas os dois inventários têm que ser públicos.
//   - Oferta (se o vendedor cadastrou a API key): estado da oferta, horário
//     real da troca e cancelamento.
type Steam struct {
	api   SteamAPI
	pacer *pacer
}

// NewSteam: inventoryGap é o intervalo mínimo entre duas consultas de
// inventário. O endpoint tem rate limit agressivo e é compartilhado com o
// sync de inventário dos usuários.
func NewSteam(api SteamAPI, inventoryGap time.Duration) *Steam {
	return &Steam{api: api, pacer: &pacer{every: inventoryGap}}
}

// Start não faz nada: no P2P quem envia a oferta é o vendedor. Sem offerID,
// a transação fica pending até a oferta (ou o item) aparecer.
func (p *Steam) Start(context.Context, Trade) (string, error) { return "", nil }

func (p *Steam) Check(ctx context.Context, t Trade) (Observation, error) {
	var obs Observation
	var offerErr error
	if t.SellerAPIKey != "" {
		obs, offerErr = p.offer(ctx, t)
	}

	loc, invErr := p.locate(ctx, t)
	obs.Item = loc

	switch {
	case offerErr != nil && invErr != nil:
		return Observation{}, errors.Join(offerErr, invErr)
	case t.SellerAPIKey == "" && invErr != nil:
		return Observation{}, invErr
	}
	// Uma das fontes respondeu: devolve o que se sabe. Item ou oferta vazios
	// significam "não deu para saber", e o service age com cautela.
	return obs, nil
}

func (p *Steam) Cancel(ctx context.Context, t Trade) error {
	if t.SellerAPIKey == "" {
		return ErrCannotCancel
	}
	obs, err := p.offer(ctx, t)
	if err != nil {
		return err
	}
	if obs.Offer != OfferPending {
		return nil // nada aberto para cancelar
	}
	return p.api.CancelTradeOffer(ctx, t.SellerAPIKey, obs.OfferID)
}

// offer encontra a oferta desta venda: pela ID, se já for conhecida, ou entre
// as enviadas pelo vendedor desde a compra, com o item e para o comprador.
func (p *Steam) offer(ctx context.Context, t Trade) (Observation, error) {
	if t.OfferID != "" {
		o, err := p.api.GetTradeOffer(ctx, t.SellerAPIKey, t.OfferID)
		if err != nil {
			return Observation{}, err
		}
		return fromOffer(o), nil
	}

	buyer, err := steam.AccountID(t.BuyerSteamID)
	if err != nil {
		return Observation{}, err
	}
	offers, err := p.api.GetSentOffers(ctx, t.SellerAPIKey, t.Since)
	if err != nil {
		return Observation{}, err
	}

	// O vendedor pode ter enviado, cancelado e enviado de novo: uma oferta
	// aceita ou aberta vale mais que uma encerrada; entre iguais, a mais nova.
	var best *steam.TradeOffer
	rank := func(o steam.TradeOffer) int {
		switch fromOffer(o).Offer {
		case OfferAccepted:
			return 2
		case OfferPending:
			return 1
		}
		return 0
	}
	for i, o := range offers {
		if o.AccountIDOther != buyer || !slices.Contains(o.AssetIDsToGive, t.Item.AssetID) {
			continue
		}
		if best == nil || rank(o) > rank(*best) || (rank(o) == rank(*best) && o.TimeCreated.After(best.TimeCreated)) {
			best = &offers[i]
		}
	}
	if best == nil {
		return Observation{Offer: OfferNone}, nil
	}
	return fromOffer(*best), nil
}

func fromOffer(o steam.TradeOffer) Observation {
	obs := Observation{OfferID: o.ID}
	switch o.State {
	case steam.OfferActive, steam.OfferCreatedNeedsConfirmation, steam.OfferInEscrow:
		obs.Offer = OfferPending
	case steam.OfferAccepted:
		obs.Offer = OfferAccepted
		at := o.TimeUpdated
		obs.TradedAt = &at
	default:
		obs.Offer = OfferDeclined
	}
	return obs
}

// locate procura o item no vendedor e, se não estiver lá, no comprador.
func (p *Steam) locate(ctx context.Context, t Trade) (Location, error) {
	sellerItems, err := p.inventory(ctx, t.SellerSteamID)
	if err != nil {
		return ItemUnknown, fmt.Errorf("seller inventory: %w", err)
	}
	for _, it := range sellerItems {
		if it.AssetID == t.Item.AssetID || matches(t.Item, it) {
			return ItemWithSeller, nil
		}
	}
	if !t.Item.HasFingerprint() {
		return ItemElsewhere, nil // não dá para reconhecer no comprador
	}

	buyerItems, err := p.inventory(ctx, t.BuyerSteamID)
	if err != nil {
		return ItemUnknown, fmt.Errorf("buyer inventory: %w", err)
	}
	for _, it := range buyerItems {
		if matches(t.Item, it) {
			return ItemWithBuyer, nil
		}
	}
	return ItemElsewhere, nil
}

func (p *Steam) inventory(ctx context.Context, steamID string) ([]steam.InventoryItem, error) {
	if err := p.pacer.wait(ctx); err != nil {
		return nil, err
	}
	return p.api.GetInventory(ctx, steamID)
}

// matches compara pela impressão digital. O float vem do mesmo texto da
// Steam nos dois lados, então a comparação exata de float64 é segura aqui.
func matches(want Item, got steam.InventoryItem) bool {
	if !want.HasFingerprint() || got.Float == nil || got.MarketHashName != want.MarketHashName {
		return false
	}
	if *got.Float != *want.Float {
		return false
	}
	switch {
	case want.PaintSeed == nil && got.PaintSeed == nil:
		return true
	case want.PaintSeed != nil && got.PaintSeed != nil:
		return *want.PaintSeed == *got.PaintSeed
	}
	return false
}

// pacer espaça chamadas: cada uma espera pelo menos `every` depois da anterior.
type pacer struct {
	mu    sync.Mutex
	next  time.Time
	every time.Duration
}

func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	at := time.Now()
	if p.next.After(at) {
		at = p.next
	}
	p.next = at.Add(p.every)
	p.mu.Unlock()

	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
