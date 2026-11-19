package steam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newInventoryTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("key", "http://localhost:8080", "http://localhost:8080/auth/steam/callback")
	c.communityBase = srv.URL
	return c
}

func TestGetInventoryParsesItems(t *testing.T) {
	fixture, err := os.ReadFile("testdata/inventory_page.json")
	if err != nil {
		t.Fatal(err)
	}
	c := newInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inventory/"+testSteamID+"/730/2" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Write(fixture)
	})

	items, err := c.GetInventory(context.Background(), testSteamID)
	if err != nil {
		t.Fatal(err)
	}
	// O asset 1003 não tem description correspondente e deve ser ignorado.
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	ak := items[0]
	if ak.AssetID != "1001" || ak.Weapon != "AK-47" || ak.Rarity != "Classified" || ak.RarityColor != "d32ce6" || ak.Exterior != "Field-Tested" {
		t.Errorf("unexpected AK fields: %+v", ak)
	}
	if ak.Float == nil || *ak.Float != 0.254781 {
		t.Errorf("float = %v, want 0.254781", ak.Float)
	}
	if ak.PaintSeed == nil || *ak.PaintSeed != 661 {
		t.Errorf("paint seed = %v, want 661", ak.PaintSeed)
	}
	if ak.IconURL != imageBaseURL+"ak_icon" {
		t.Errorf("icon = %q", ak.IconURL)
	}
	if !ak.Tradable || !ak.Marketable {
		t.Error("AK should be tradable and marketable")
	}
	wantStickers := []Sticker{
		{Kind: "sticker", Name: "Falcons | Budapest 2025", ImageURL: "https://cdn.test/s1.png"},
		{Kind: "sticker", Name: "Crown & Glory", ImageURL: "https://cdn.test/s2.png"},
	}
	if fmt.Sprint(ak.Stickers) != fmt.Sprint(wantStickers) {
		t.Errorf("stickers = %+v, want %+v", ak.Stickers, wantStickers)
	}

	box := items[1]
	if box.Float != nil || box.PaintSeed != nil {
		t.Error("case should have no float/paint seed")
	}
	if box.Tradable {
		t.Error("case should not be tradable")
	}
	if box.Stickers == nil || len(box.Stickers) != 0 {
		t.Errorf("stickers should be empty non-nil slice, got %#v", box.Stickers)
	}
}

func TestGetInventoryFollowsPagination(t *testing.T) {
	page := func(assetID string, more bool) string {
		extra := ""
		if more {
			extra = fmt.Sprintf(`,"more_items":1,"last_assetid":%q`, assetID)
		}
		return fmt.Sprintf(`{"assets":[{"assetid":%q,"classid":"1","instanceid":"0"}],
			"descriptions":[{"classid":"1","instanceid":"0","name":"X","market_hash_name":"X","icon_url":"i","tags":[],"descriptions":[]}],
			"success":1%s}`, assetID, extra)
	}

	var starts []string
	c := newInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start_assetid")
		starts = append(starts, start)
		switch start {
		case "":
			fmt.Fprint(w, page("1", true))
		case "1":
			fmt.Fprint(w, page("2", false))
		default:
			t.Errorf("unexpected start_assetid %q", start)
		}
	})

	items, err := c.GetInventory(context.Background(), testSteamID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].AssetID != "1" || items[1].AssetID != "2" {
		t.Fatalf("items = %+v", items)
	}
	if len(starts) != 2 {
		t.Fatalf("expected 2 requests, got %v", starts)
	}
}

func TestGetInventoryErrors(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusForbidden, ErrInventoryPrivate},
		{http.StatusTooManyRequests, ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			c := newInventoryTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, "null")
			})
			_, err := c.GetInventory(context.Background(), testSteamID)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
