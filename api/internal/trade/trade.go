// Package trade acompanha a entrega dos itens vendidos.
//
// O modelo é P2P: quem envia a oferta de troca é o próprio vendedor, pela
// Steam. A plataforma não opera bots; ela só OBSERVA a troca e informa ao
// service o que viu (Observation). Quem decide o que fazer com isso (avançar,
// estornar, pagar) é o service, com regras conservadoras.
//
// Implementações:
//   - Simulated: tudo em memória, para dev e demonstração.
//   - Steam: híbrida. Sempre confere os inventários públicos; se o vendedor
//     cadastrou a API key, também acompanha a oferta pela Web API.
package trade

import (
	"errors"
	"time"
)

var (
	// ErrRejected é uma falha permanente ao iniciar a troca: tentar de novo
	// não adianta. Qualquer outro erro é tratado como temporário.
	ErrRejected = errors.New("trade rejected")

	// ErrCannotCancel: o provider não tem como cancelar ofertas (vendedor
	// sem API key). Quem chama decide se pode seguir mesmo assim.
	ErrCannotCancel = errors.New("trade offer cannot be cancelled by the platform")
)

// Item identifica a skin vendida.
//
// O AssetID só vale enquanto o item está no inventário do vendedor: a Steam
// gera um novo a cada troca. Do outro lado, o item é reconhecido pela
// impressão digital (nome + float + pattern), que não muda.
type Item struct {
	AssetID        string
	MarketHashName string
	Float          *float64
	PaintSeed      *int32
}

// HasFingerprint diz se dá para reconhecer o item em outro inventário. Itens
// sem float (caixas, stickers, agentes) não têm impressão digital única.
func (i Item) HasFingerprint() bool { return i.Float != nil }

// Trade é tudo que um provider precisa para acompanhar uma venda.
type Trade struct {
	TransactionID int64
	Item          Item
	SellerSteamID string
	BuyerSteamID  string
	BuyerTradeURL string
	SellerAPIKey  string    // vazio: só a verificação pelo inventário
	OfferID       string    // já conhecida, se houver
	Since         time.Time // momento da compra: ofertas anteriores não contam
}

// OfferState é o que se sabe da oferta de troca.
type OfferState string

const (
	OfferNone     OfferState = ""         // nenhuma oferta conhecida (ou sem como saber)
	OfferPending  OfferState = "pending"  // enviada, aguardando o comprador
	OfferAccepted OfferState = "accepted" // aceita: a troca aconteceu
	OfferDeclined OfferState = "declined" // recusada, cancelada ou expirada
)

// Location é onde o item foi visto.
type Location string

const (
	ItemUnknown    Location = ""          // não deu para verificar (inventário privado, Steam fora)
	ItemWithSeller Location = "seller"    // ainda (ou de novo) com o vendedor
	ItemWithBuyer  Location = "buyer"     // reconhecido no inventário do comprador
	ItemElsewhere  Location = "elsewhere" // saiu do vendedor, mas não foi visto no comprador
)

// Observation é o retrato da troca num momento.
type Observation struct {
	Offer    OfferState
	OfferID  string
	TradedAt *time.Time // horário real da troca, quando a Steam informa
	Item     Location
}
