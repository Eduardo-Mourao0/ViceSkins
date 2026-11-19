package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	cs2AppID         = 730
	cs2ContextID     = 2
	inventoryPerPage = 2000
	maxInventoryPage = 10 // até 20 mil itens; acima disso desistimos

	imageBaseURL = "https://community.fastly.steamstatic.com/economy/image/"

	// IDs de asset_properties no inventário do CS2.
	propPaintSeed = 1 // "Pattern Template"
	propWear      = 2 // "Wear Rating" (float)
)

var (
	ErrInventoryPrivate = errors.New("steam inventory is private")
	ErrRateLimited      = errors.New("steam rate limited the request")
)

type Sticker struct {
	Kind     string // sticker, patch, charm...
	Name     string
	ImageURL string
}

// InventoryItem é um item do inventário já "achatado": a Steam manda asset,
// description e properties separados, e aqui juntamos tudo num struct só.
type InventoryItem struct {
	AssetID        string
	ClassID        string
	InstanceID     string
	MarketHashName string
	Name           string
	Type           string
	Weapon         string
	Rarity         string
	RarityColor    string
	Exterior       string
	Float          *float64 // nil quando o item não tem desgaste
	PaintSeed      *int32
	IconURL        string
	Stickers       []Sticker
	Tradable       bool
	Marketable     bool
}

// GetInventory busca o inventário CS2 completo, página por página.
func (c *Client) GetInventory(ctx context.Context, steamID string) ([]InventoryItem, error) {
	var items []InventoryItem
	start := ""
	for range maxInventoryPage {
		page, err := c.fetchInventoryPage(ctx, steamID, start)
		if err != nil {
			return nil, err
		}
		items = append(items, page.items()...)

		if !page.hasMore() || page.LastAssetID == "" {
			return items, nil
		}
		start = page.LastAssetID
	}
	return nil, fmt.Errorf("inventory has more than %d pages", maxInventoryPage)
}

func (c *Client) fetchInventoryPage(ctx context.Context, steamID, startAssetID string) (*inventoryPage, error) {
	v := url.Values{"l": {"english"}, "count": {strconv.Itoa(inventoryPerPage)}}
	if startAssetID != "" {
		v.Set("start_assetid", startAssetID)
	}
	u := fmt.Sprintf("%s/inventory/%s/%d/%d?%s", c.communityBase, url.PathEscape(steamID), cs2AppID, cs2ContextID, v.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get inventory: %w", stripURL(err))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, ErrInventoryPrivate
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		return nil, fmt.Errorf("get inventory: status %d", resp.StatusCode)
	}

	var page inventoryPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode inventory: %w", err)
	}
	if page.Success != 1 {
		return nil, fmt.Errorf("get inventory: success=%d", page.Success)
	}
	return &page, nil
}

// ---- Formato bruto da resposta da Steam ----

type inventoryPage struct {
	Assets []struct {
		AssetID    string `json:"assetid"`
		ClassID    string `json:"classid"`
		InstanceID string `json:"instanceid"`
	} `json:"assets"`
	Descriptions    []description `json:"descriptions"`
	AssetProperties []struct {
		AssetID    string          `json:"assetid"`
		Properties []assetProperty `json:"asset_properties"`
	} `json:"asset_properties"`
	// A Steam já mandou isso como número e como bool; RawMessage aceita os dois.
	MoreItems   json.RawMessage `json:"more_items"`
	LastAssetID string          `json:"last_assetid"`
	Success     int             `json:"success"`
}

type description struct {
	ClassID        string `json:"classid"`
	InstanceID     string `json:"instanceid"`
	IconURL        string `json:"icon_url"`
	Name           string `json:"name"`
	MarketHashName string `json:"market_hash_name"`
	Type           string `json:"type"`
	Tradable       int    `json:"tradable"`
	Marketable     int    `json:"marketable"`
	Tags           []struct {
		Category string `json:"category"`
		Name     string `json:"localized_tag_name"`
		Color    string `json:"color"`
	} `json:"tags"`
	Descriptions []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"descriptions"`
}

type assetProperty struct {
	PropertyID int    `json:"propertyid"`
	IntValue   string `json:"int_value"`
	FloatValue string `json:"float_value"`
}

func (p *inventoryPage) hasMore() bool {
	s := string(p.MoreItems)
	return s != "" && s != "0" && s != "false" && s != "null"
}

// items junta assets + descriptions (por classid/instanceid) + properties (por assetid).
func (p *inventoryPage) items() []InventoryItem {
	descs := make(map[string]description, len(p.Descriptions))
	for _, d := range p.Descriptions {
		descs[d.ClassID+"_"+d.InstanceID] = d
	}
	props := make(map[string][]assetProperty, len(p.AssetProperties))
	for _, ap := range p.AssetProperties {
		props[ap.AssetID] = ap.Properties
	}

	out := make([]InventoryItem, 0, len(p.Assets))
	for _, a := range p.Assets {
		d, ok := descs[a.ClassID+"_"+a.InstanceID]
		if !ok {
			continue
		}

		it := InventoryItem{
			AssetID:        a.AssetID,
			ClassID:        a.ClassID,
			InstanceID:     a.InstanceID,
			MarketHashName: d.MarketHashName,
			Name:           d.Name,
			Type:           d.Type,
			IconURL:        imageBaseURL + d.IconURL,
			Stickers:       parseStickers(d),
			Tradable:       d.Tradable == 1,
			Marketable:     d.Marketable == 1,
		}
		for _, t := range d.Tags {
			switch t.Category {
			case "Rarity":
				it.Rarity, it.RarityColor = t.Name, t.Color
			case "Exterior":
				it.Exterior = t.Name
			case "Weapon":
				it.Weapon = t.Name
			}
		}
		for _, pr := range props[a.AssetID] {
			switch pr.PropertyID {
			case propWear:
				if f, err := strconv.ParseFloat(pr.FloatValue, 64); err == nil {
					it.Float = &f
				}
			case propPaintSeed:
				if n, err := strconv.ParseInt(pr.IntValue, 10, 32); err == nil {
					seed := int32(n)
					it.PaintSeed = &seed
				}
			}
		}
		out = append(out, it)
	}
	return out
}

// Os stickers/chaveiros vêm como HTML: <img src="..." title="Sticker: Nome">.
var stickerImgRe = regexp.MustCompile(`<img[^>]*?src="([^"]+)"[^>]*?title="([^"]+)"`)

func parseStickers(d description) []Sticker {
	stickers := []Sticker{} // nunca nil: vira "[]" no JSON, não "null"
	for _, desc := range d.Descriptions {
		if desc.Name != "sticker_info" && desc.Name != "keychain_info" {
			continue
		}
		for _, m := range stickerImgRe.FindAllStringSubmatch(desc.Value, -1) {
			kind, name, found := strings.Cut(html.UnescapeString(m[2]), ": ")
			if !found {
				kind, name = "sticker", kind
			}
			stickers = append(stickers, Sticker{
				Kind:     strings.ToLower(kind),
				Name:     name,
				ImageURL: m[1],
			})
		}
	}
	return stickers
}
