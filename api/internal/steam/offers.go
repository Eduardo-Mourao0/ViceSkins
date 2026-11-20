package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Ofertas de troca pela Web API oficial (IEconService). Todas as chamadas
// usam a API key do DONO das ofertas (o vendedor), passada por parâmetro:
// cada vendedor tem a sua, e ela nunca é a key da plataforma.

var ErrInvalidAPIKey = errors.New("steam api key is invalid")

// OfferState segue o ETradeOfferState da Steam.
type OfferState int

const (
	OfferInvalid                  OfferState = 1
	OfferActive                   OfferState = 2 // enviada, aguardando o outro lado
	OfferAccepted                 OfferState = 3
	OfferCountered                OfferState = 4
	OfferExpired                  OfferState = 5
	OfferCanceled                 OfferState = 6
	OfferDeclined                 OfferState = 7
	OfferInvalidItems             OfferState = 8
	OfferCreatedNeedsConfirmation OfferState = 9 // criada, falta confirmar no app da Steam
	OfferCanceledBySecondFactor   OfferState = 10
	OfferInEscrow                 OfferState = 11
)

type TradeOffer struct {
	ID             string
	AccountIDOther uint32 // SteamID32 de quem recebe
	State          OfferState
	AssetIDsToGive []string // itens CS2 que o dono da key está dando
	TimeCreated    time.Time
	TimeUpdated    time.Time // para ofertas aceitas, o momento da troca
}

// GetSentOffers lista as ofertas enviadas pelo dono da key desde `since`,
// incluindo as já encerradas (aceitas, recusadas...).
func (c *Client) GetSentOffers(ctx context.Context, apiKey string, since time.Time) ([]TradeOffer, error) {
	v := url.Values{
		"key":                    {apiKey},
		"get_sent_offers":        {"1"},
		"get_received_offers":    {"0"},
		"active_only":            {"0"},
		"historical_only":        {"0"},
		"get_descriptions":       {"0"},
		"time_historical_cutoff": {strconv.FormatInt(since.Unix(), 10)},
	}
	var body struct {
		Response struct {
			Sent []rawOffer `json:"trade_offers_sent"`
		} `json:"response"`
	}
	if err := c.webAPI(ctx, http.MethodGet, "/IEconService/GetTradeOffers/v1/", v, &body); err != nil {
		return nil, fmt.Errorf("get trade offers: %w", err)
	}
	out := make([]TradeOffer, 0, len(body.Response.Sent))
	for _, o := range body.Response.Sent {
		out = append(out, o.parse())
	}
	return out, nil
}

func (c *Client) GetTradeOffer(ctx context.Context, apiKey, offerID string) (TradeOffer, error) {
	v := url.Values{"key": {apiKey}, "tradeofferid": {offerID}, "get_descriptions": {"0"}}
	var body struct {
		Response struct {
			Offer *rawOffer `json:"offer"`
		} `json:"response"`
	}
	if err := c.webAPI(ctx, http.MethodGet, "/IEconService/GetTradeOffer/v1/", v, &body); err != nil {
		return TradeOffer{}, fmt.Errorf("get trade offer: %w", err)
	}
	if body.Response.Offer == nil {
		return TradeOffer{}, fmt.Errorf("get trade offer: offer %s not found", offerID)
	}
	return body.Response.Offer.parse(), nil
}

func (c *Client) CancelTradeOffer(ctx context.Context, apiKey, offerID string) error {
	v := url.Values{"key": {apiKey}, "tradeofferid": {offerID}}
	if err := c.webAPI(ctx, http.MethodPost, "/IEconService/CancelTradeOffer/v1/", v, nil); err != nil {
		return fmt.Errorf("cancel trade offer: %w", err)
	}
	return nil
}

// ValidateAPIKey confere se a key funciona, com a chamada mais barata de ofertas.
func (c *Client) ValidateAPIKey(ctx context.Context, apiKey string) error {
	v := url.Values{"key": {apiKey}, "time_last_visit": {"0"}}
	return c.webAPI(ctx, http.MethodGet, "/IEconService/GetTradeOffersSummary/v1/", v, nil)
}

// webAPI faz a chamada e decodifica o JSON em out (se não for nil).
// Em GET a key vai na URL (exigência da Steam): erros passam por stripURL
// para ela nunca aparecer em log. Em POST ela vai no corpo.
func (c *Client) webAPI(ctx context.Context, method, path string, v url.Values, out any) error {
	var req *http.Request
	var err error
	if method == http.MethodPost {
		req, err = http.NewRequestWithContext(ctx, method, c.webAPIBase+path, strings.NewReader(v.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.webAPIBase+path+"?"+v.Encode(), nil)
	}
	if err != nil {
		return stripURL(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return stripURL(err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrInvalidAPIKey
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

type rawOffer struct {
	ID             string `json:"tradeofferid"`
	AccountIDOther uint32 `json:"accountid_other"`
	State          int    `json:"trade_offer_state"`
	TimeCreated    int64  `json:"time_created"`
	TimeUpdated    int64  `json:"time_updated"`
	ItemsToGive    []struct {
		AppID   json.Number `json:"appid"` // a Steam manda número ou string, depende do endpoint
		AssetID string      `json:"assetid"`
	} `json:"items_to_give"`
}

func (o rawOffer) parse() TradeOffer {
	t := TradeOffer{
		ID:             o.ID,
		AccountIDOther: o.AccountIDOther,
		State:          OfferState(o.State),
		TimeCreated:    time.Unix(o.TimeCreated, 0),
		TimeUpdated:    time.Unix(o.TimeUpdated, 0),
	}
	for _, it := range o.ItemsToGive {
		if it.AppID.String() == strconv.Itoa(cs2AppID) {
			t.AssetIDsToGive = append(t.AssetIDsToGive, it.AssetID)
		}
	}
	return t
}

// AccountID converte SteamID64 em SteamID32 (o accountid das ofertas).
func AccountID(steamID64 string) (uint32, error) {
	const base = 76561197960265728
	id, err := strconv.ParseUint(steamID64, 10, 64)
	if err != nil || id < base || id-base > 1<<32-1 {
		return 0, fmt.Errorf("invalid steamid64 %q", steamID64)
	}
	return uint32(id - base), nil
}
