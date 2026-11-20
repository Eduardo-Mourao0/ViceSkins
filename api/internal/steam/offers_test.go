package steam

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeWebAPI(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("platform-key", "http://x", "http://x/cb")
	c.webAPIBase = srv.URL
	return c
}

func TestGetSentOffers(t *testing.T) {
	c := fakeWebAPI(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("key") != "seller-key" || q.Get("get_sent_offers") != "1" || q.Get("time_historical_cutoff") != "1700000000" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"response":{"trade_offers_sent":[
			{"tradeofferid":"111","accountid_other":39734395,"trade_offer_state":3,"time_created":1700000100,"time_updated":1700000200,
			 "items_to_give":[{"appid":730,"contextid":"2","assetid":"555"},{"appid":"440","contextid":"2","assetid":"999"}]}
		]}}`))
	})
	offers, err := c.GetSentOffers(context.Background(), "seller-key", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(offers) != 1 {
		t.Fatalf("%d offers, want 1", len(offers))
	}
	o := offers[0]
	if o.ID != "111" || o.AccountIDOther != 39734395 || o.State != OfferAccepted || !o.TimeUpdated.Equal(time.Unix(1_700_000_200, 0)) {
		t.Fatalf("offer = %+v", o)
	}
	if len(o.AssetIDsToGive) != 1 || o.AssetIDsToGive[0] != "555" {
		t.Fatalf("assets = %v, want only the CS2 asset 555", o.AssetIDsToGive)
	}
}

func TestCancelTradeOfferPostsForm(t *testing.T) {
	c := fakeWebAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/IEconService/CancelTradeOffer/v1/" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if strings.Contains(r.URL.RawQuery, "seller-key") {
			t.Error("POST must not put the key in the url")
		}
		r.ParseForm()
		if r.PostForm.Get("tradeofferid") != "111" || r.PostForm.Get("key") != "seller-key" {
			t.Errorf("form = %v", r.PostForm)
		}
		w.Write([]byte(`{"response":{}}`))
	})
	if err := c.CancelTradeOffer(context.Background(), "seller-key", "111"); err != nil {
		t.Fatal(err)
	}
}

func TestWebAPIErrorsNeverLeakTheKey(t *testing.T) {
	c := fakeWebAPI(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	if err := c.ValidateAPIKey(context.Background(), "SECRET-KEY-123"); !errors.Is(err, ErrInvalidAPIKey) {
		t.Fatalf("err = %v, want ErrInvalidAPIKey", err)
	}

	c.webAPIBase = "http://127.0.0.1:1" // conexão recusada: o erro de rede traria a URL com a key
	_, err := c.GetSentOffers(context.Background(), "SECRET-KEY-123", time.Now())
	if err == nil || strings.Contains(err.Error(), "SECRET-KEY-123") {
		t.Fatalf("err = %v; must fail without leaking the key", err)
	}
}

func TestAccountID(t *testing.T) {
	if id, err := AccountID("76561198000000123"); err != nil || id != 39734395 {
		t.Fatalf("AccountID = %d, %v", id, err)
	}
	for _, bad := range []string{"", "demo-1", "123"} {
		if _, err := AccountID(bad); err == nil {
			t.Errorf("AccountID(%q) accepted", bad)
		}
	}
}
