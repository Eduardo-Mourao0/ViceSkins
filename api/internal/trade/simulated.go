// Package trade contém as implementações de envio de ofertas de troca da Steam.
//
// No MVP só existe Simulated. A integração real (ex.: um microserviço Node com
// steam-tradeoffer-manager) entra como outra implementação da mesma interface,
// sem mudar o service de compra.
package trade

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type OfferState string

const (
	OfferPending  OfferState = "pending"  // enviada, comprador ainda não respondeu
	OfferAccepted OfferState = "accepted" // comprador aceitou: item trocou de dono
	OfferDeclined OfferState = "declined" // recusada ou expirada
)

// Offer é o que precisa ir para a Steam para montar a oferta de troca.
type Offer struct {
	TransactionID int64
	AssetID       string
	SellerID      int64
	BuyerID       int64
}

// Simulated finge enviar a oferta e a considera aceita depois de acceptAfter.
// Guarda as ofertas em memória: depois de reiniciar a API, ofertas desconhecidas
// são tratadas como aceitas.
type Simulated struct {
	acceptAfter time.Duration
	now         func() time.Time

	mu     sync.Mutex // map não é seguro para uso concorrente em Go
	sentAt map[string]time.Time
}

func NewSimulated(acceptAfter time.Duration) *Simulated {
	return &Simulated{acceptAfter: acceptAfter, now: time.Now, sentAt: map[string]time.Time{}}
}

func (s *Simulated) SendOffer(_ context.Context, o Offer) (string, error) {
	id := fmt.Sprintf("sim-%d", o.TransactionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sentAt[id] = s.now()
	return id, nil
}

func (s *Simulated) OfferState(_ context.Context, offerID string) (OfferState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent, ok := s.sentAt[offerID]
	if !ok || s.now().Sub(sent) >= s.acceptAfter {
		delete(s.sentAt, offerID)
		return OfferAccepted, nil
	}
	return OfferPending, nil
}
