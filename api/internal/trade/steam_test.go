package trade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/steam"
)

const (
	sellerID = "76561198000000001"
	buyerID  = "76561198000000002" // account id 39734274
)

type fakeSteamAPI struct {
	inventories map[string][]steam.InventoryItem
	privateInv  map[string]bool
	offers      []steam.TradeOffer
	offersErr   error
	cancelled   []string
	invCalls    int
}

func (f *fakeSteamAPI) GetInventory(_ context.Context, id string) ([]steam.InventoryItem, error) {
	f.invCalls++
	if f.privateInv[id] {
		return nil, steam.ErrInventoryPrivate
	}
	return f.inventories[id], nil
}

func (f *fakeSteamAPI) GetSentOffers(context.Context, string, time.Time) ([]steam.TradeOffer, error) {
	return f.offers, f.offersErr
}

func (f *fakeSteamAPI) GetTradeOffer(_ context.Context, _, id string) (steam.TradeOffer, error) {
	for _, o := range f.offers {
		if o.ID == id {
			return o, nil
		}
	}
	return steam.TradeOffer{}, errors.New("not found")
}

func (f *fakeSteamAPI) CancelTradeOffer(_ context.Context, _, id string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func ptr[T any](v T) *T { return &v }

// AK com float: a impressão digital vale nos dois lados.
var ak = Item{AssetID: "100", MarketHashName: "AK-47 | Redline (Field-Tested)", Float: ptr(0.2735034227371216), PaintSeed: ptr(int32(310))}

func invItem(asset string, it Item) steam.InventoryItem {
	return steam.InventoryItem{AssetID: asset, MarketHashName: it.MarketHashName, Float: it.Float, PaintSeed: it.PaintSeed}
}

func newTrade(item Item, key string) Trade {
	return Trade{TransactionID: 1, Item: item, SellerSteamID: sellerID, BuyerSteamID: buyerID, SellerAPIKey: key}
}

func TestLocate(t *testing.T) {
	sameNameOtherFloat := ak
	sameNameOtherFloat.Float = ptr(0.30)
	sameFloatOtherSeed := ak
	sameFloatOtherSeed.PaintSeed = ptr(int32(311))

	tests := []struct {
		name   string
		seller []steam.InventoryItem
		buyer  []steam.InventoryItem
		want   Location
	}{
		{"still with seller (same asset id)", []steam.InventoryItem{invItem("100", ak)}, nil, ItemWithSeller},
		{"delivered: new asset id at the buyer", nil, []steam.InventoryItem{invItem("900", ak)}, ItemWithBuyer},
		{"reversed: back at the seller with a new asset id", []steam.InventoryItem{invItem("901", ak)}, nil, ItemWithSeller},
		{"same skin, other float is not ours", nil, []steam.InventoryItem{invItem("900", sameNameOtherFloat)}, ItemElsewhere},
		{"same float, other pattern is not ours", nil, []steam.InventoryItem{invItem("900", sameFloatOtherSeed)}, ItemElsewhere},
		{"in neither inventory", nil, nil, ItemElsewhere},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeSteamAPI{inventories: map[string][]steam.InventoryItem{sellerID: tt.seller, buyerID: tt.buyer}}
			obs, err := NewSteam(api, 0).Check(context.Background(), newTrade(ak, ""))
			if err != nil {
				t.Fatal(err)
			}
			if obs.Item != tt.want {
				t.Fatalf("item = %q, want %q", obs.Item, tt.want)
			}
		})
	}
}

func TestLocateWithoutFingerprint(t *testing.T) {
	caseItem := Item{AssetID: "200", MarketHashName: "Kilowatt Case"}
	api := &fakeSteamAPI{inventories: map[string][]steam.InventoryItem{
		buyerID: {{AssetID: "999", MarketHashName: "Kilowatt Case"}}, // pode ser outra caixa igual
	}}
	obs, err := NewSteam(api, 0).Check(context.Background(), newTrade(caseItem, ""))
	if err != nil {
		t.Fatal(err)
	}
	if obs.Item != ItemElsewhere {
		t.Fatalf("item = %q: an item without float must never be 'found' at the buyer", obs.Item)
	}
	if api.invCalls != 1 {
		t.Fatalf("%d inventory calls, want 1 (the buyer's is useless here)", api.invCalls)
	}
}

func TestPrivateInventoryIsUnknownNotMissing(t *testing.T) {
	api := &fakeSteamAPI{privateInv: map[string]bool{buyerID: true}}
	_, err := NewSteam(api, 0).Check(context.Background(), newTrade(ak, ""))
	if !errors.Is(err, steam.ErrInventoryPrivate) {
		t.Fatalf("err = %v, want ErrInventoryPrivate (never 'elsewhere')", err)
	}
}

func TestOfferWithAPIKey(t *testing.T) {
	traded := time.Unix(1_700_000_500, 0)
	api := &fakeSteamAPI{
		inventories: map[string][]steam.InventoryItem{buyerID: {invItem("900", ak)}},
		offers: []steam.TradeOffer{
			{ID: "1", AccountIDOther: 39734274, State: steam.OfferCanceled, AssetIDsToGive: []string{"100"}, TimeCreated: time.Unix(10, 0)},
			{ID: "2", AccountIDOther: 39734274, State: steam.OfferAccepted, AssetIDsToGive: []string{"100"}, TimeCreated: time.Unix(5, 0), TimeUpdated: traded},
			{ID: "3", AccountIDOther: 11111111, State: steam.OfferActive, AssetIDsToGive: []string{"100"}, TimeCreated: time.Unix(20, 0)}, // outro destinatário
			{ID: "4", AccountIDOther: 39734274, State: steam.OfferActive, AssetIDsToGive: []string{"777"}, TimeCreated: time.Unix(30, 0)}, // outro item
		},
	}
	obs, err := NewSteam(api, 0).Check(context.Background(), newTrade(ak, "KEY"))
	if err != nil {
		t.Fatal(err)
	}
	if obs.Offer != OfferAccepted || obs.OfferID != "2" || obs.TradedAt == nil || !obs.TradedAt.Equal(traded) {
		t.Fatalf("obs = %+v, want offer 2 accepted at the steam trade time", obs)
	}
	if obs.Item != ItemWithBuyer {
		t.Fatalf("item = %q, want buyer", obs.Item)
	}
}

func TestCheckSurvivesOneSourceFailing(t *testing.T) {
	api := &fakeSteamAPI{
		privateInv: map[string]bool{sellerID: true},
		offers:     []steam.TradeOffer{{ID: "9", AccountIDOther: 39734274, State: steam.OfferActive, AssetIDsToGive: []string{"100"}}},
	}
	obs, err := NewSteam(api, 0).Check(context.Background(), newTrade(ak, "KEY"))
	if err != nil {
		t.Fatalf("offer info is available, Check must not fail: %v", err)
	}
	if obs.Offer != OfferPending || obs.Item != ItemUnknown {
		t.Fatalf("obs = %+v, want pending offer and unknown location", obs)
	}

	api.offersErr = errors.New("steam down")
	if _, err := NewSteam(api, 0).Check(context.Background(), newTrade(ak, "KEY")); err == nil {
		t.Fatal("both sources failed: Check must return an error")
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	if err := NewSteam(&fakeSteamAPI{}, 0).Cancel(ctx, newTrade(ak, "")); !errors.Is(err, ErrCannotCancel) {
		t.Fatalf("without api key: err = %v, want ErrCannotCancel", err)
	}

	api := &fakeSteamAPI{offers: []steam.TradeOffer{
		{ID: "5", AccountIDOther: 39734274, State: steam.OfferActive, AssetIDsToGive: []string{"100"}},
	}}
	if err := NewSteam(api, 0).Cancel(ctx, newTrade(ak, "KEY")); err != nil {
		t.Fatal(err)
	}
	if len(api.cancelled) != 1 || api.cancelled[0] != "5" {
		t.Fatalf("cancelled = %v, want the open offer 5", api.cancelled)
	}

	api = &fakeSteamAPI{} // nada aberto: nada a cancelar
	if err := NewSteam(api, 0).Cancel(ctx, newTrade(ak, "KEY")); err != nil || len(api.cancelled) != 0 {
		t.Fatalf("err = %v, cancelled = %v", err, api.cancelled)
	}
}

func TestPacerSpacesCalls(t *testing.T) {
	p := &pacer{every: 20 * time.Millisecond}
	start := time.Now()
	for range 3 {
		if err := p.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("3 calls took %s, want >= 40ms", elapsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.next = time.Now().Add(time.Hour)
	if err := p.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
