package steam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrPlayerNotFound = errors.New("steam player not found")

type PlayerSummary struct {
	SteamID     string `json:"steamid"`
	PersonaName string `json:"personaname"`
	AvatarFull  string `json:"avatarfull"`
}

func (c *Client) GetPlayerSummary(ctx context.Context, steamID string) (PlayerSummary, error) {
	if c.apiKey == "" {
		return PlayerSummary{}, errors.New("steam api key not configured")
	}

	v := url.Values{"key": {c.apiKey}, "steamids": {steamID}}
	u := c.webAPIBase + "/ISteamUser/GetPlayerSummaries/v2/?" + v.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return PlayerSummary{}, stripURL(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return PlayerSummary{}, fmt.Errorf("get player summaries: %w", stripURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PlayerSummary{}, fmt.Errorf("get player summaries: status %d", resp.StatusCode)
	}

	var body struct {
		Response struct {
			Players []PlayerSummary `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return PlayerSummary{}, fmt.Errorf("decode player summaries: %w", err)
	}
	if len(body.Response.Players) == 0 {
		return PlayerSummary{}, ErrPlayerNotFound
	}
	return body.Response.Players[0], nil
}
