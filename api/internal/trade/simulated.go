// Package trade contém as implementações de envio de ofertas de troca da Steam.
//
// No MVP só existe Simulated. A integração real (ex.: um microserviço Node com
// steam-tradeoffer-manager) entra como outra implementação da mesma interface,
// sem mudar o service de compra.
package trade

import (
	"context"
	"errors"
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

// ErrRejected é uma falha permanente: tentar de novo não adianta (ex.: trade
// URL inválida, inventário privado). Qualquer outro erro é tratado como
// temporário, e o envio é refeito com backoff.
var ErrRejected = errors.New("trade offer rejected")

// Offer é o que precisa ir para a Steam para montar a oferta de troca.
type Offer struct {
	TransactionID int64
	AssetID       string
	SellerID      int64
	BuyerID       int64
	BuyerTradeURL string // para onde a oferta vai (copiada na compra)
}

// Simulated finge enviar a oferta e a considera aceita depois de acceptAfter.
// Guarda as ofertas em memória: depois de reiniciar a API, ofertas desconhecidas
// são tratadas como aceitas.
type Simulated struct {
	acceptAfter time.Duration
	now         func() time.Time

	mu        sync.Mutex // map não é seguro para uso concorrente em Go
	sentAt    map[string]time.Time
	cancelled map[string]bool // canceladas não podem mais ser "aceitas"
}

func NewSimulated(acceptAfter time.Duration) *Simulated {
	return &Simulated{
		acceptAfter: acceptAfter,
		now:         time.Now,
		sentAt:      map[string]time.Time{},
		cancelled:   map[string]bool{},
	}
}

func (s *Simulated) SendOffer(_ context.Context, o Offer) (string, error) {
	id := fmt.Sprintf("sim-%d", o.TransactionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sentAt[id]; !ok { // idempotente: reenviar não reinicia o relógio
		s.sentAt[id] = s.now()
	}
	return id, nil
}

func (s *Simulated) OfferState(_ context.Context, offerID string) (OfferState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelled[offerID] {
		return OfferDeclined, nil
	}
	sent, ok := s.sentAt[offerID]
	if !ok || s.now().Sub(sent) >= s.acceptAfter {
		return OfferAccepted, nil
	}
	return OfferPending, nil
}

// CancelOffer falha se a oferta já foi aceita, como na Steam: troca feita
// não se cancela.
func (s *Simulated) CancelOffer(_ context.Context, offerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sent, ok := s.sentAt[offerID]
	if ok && s.now().Sub(sent) >= s.acceptAfter {
		return fmt.Errorf("offer %s already accepted", offerID)
	}
	s.cancelled[offerID] = true
	delete(s.sentAt, offerID)
	return nil
}
