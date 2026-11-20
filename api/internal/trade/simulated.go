package trade

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Simulated finge a Steam: Start "envia" a oferta na hora e o comprador a
// aceita depois de acceptAfter. Guarda tudo em memória: depois de reiniciar a
// API, ofertas enviadas e desconhecidas são tratadas como aceitas.
type Simulated struct {
	acceptAfter time.Duration
	now         func() time.Time

	mu        sync.Mutex // map não é seguro para uso concorrente em Go
	sentAt    map[int64]time.Time
	cancelled map[int64]bool // canceladas não podem mais ser "aceitas"
}

func NewSimulated(acceptAfter time.Duration) *Simulated {
	return &Simulated{
		acceptAfter: acceptAfter,
		now:         time.Now,
		sentAt:      map[int64]time.Time{},
		cancelled:   map[int64]bool{},
	}
}

func offerID(txID int64) string { return fmt.Sprintf("sim-%d", txID) }

func (s *Simulated) Start(_ context.Context, t Trade) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sentAt[t.TransactionID]; !ok { // idempotente: reenviar não reinicia o relógio
		s.sentAt[t.TransactionID] = s.now()
	}
	return offerID(t.TransactionID), nil
}

func (s *Simulated) Check(_ context.Context, t Trade) (Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := offerID(t.TransactionID)
	if s.cancelled[t.TransactionID] {
		return Observation{Offer: OfferDeclined, OfferID: id, Item: ItemWithSeller}, nil
	}
	sent, ok := s.sentAt[t.TransactionID]
	if !ok {
		if t.OfferID == "" { // nunca foi enviada (Start falhou)
			return Observation{Item: ItemWithSeller}, nil
		}
		// Enviada antes de a API reiniciar: considera aceita.
		return Observation{Offer: OfferAccepted, OfferID: t.OfferID, Item: ItemWithBuyer}, nil
	}
	if at := sent.Add(s.acceptAfter); !s.now().Before(at) {
		return Observation{Offer: OfferAccepted, OfferID: id, TradedAt: &at, Item: ItemWithBuyer}, nil
	}
	return Observation{Offer: OfferPending, OfferID: id, Item: ItemWithSeller}, nil
}

// Cancel falha se a oferta já foi aceita, como na Steam: troca feita não se
// cancela.
func (s *Simulated) Cancel(_ context.Context, t Trade) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent, ok := s.sentAt[t.TransactionID]
	if ok && !s.now().Before(sent.Add(s.acceptAfter)) {
		return fmt.Errorf("offer for transaction %d already accepted", t.TransactionID)
	}
	s.cancelled[t.TransactionID] = true
	delete(s.sentAt, t.TransactionID)
	return nil
}
