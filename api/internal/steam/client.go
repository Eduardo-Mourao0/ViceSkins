package steam

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultOpenIDEndpoint = "https://steamcommunity.com/openid/login"
	defaultWebAPIBase     = "https://api.steampowered.com"
	defaultCommunityBase  = "https://steamcommunity.com"
)

type Client struct {
	http           *http.Client
	apiKey         string
	realm          string // ex: http://localhost:8080
	callbackURL    string // ex: http://localhost:8080/auth/steam/callback
	openIDEndpoint string
	webAPIBase     string
	communityBase  string
}

func NewClient(apiKey, realm, callbackURL string) *Client {
	return &Client{
		http:           &http.Client{Timeout: 10 * time.Second},
		apiKey:         apiKey,
		realm:          realm,
		callbackURL:    callbackURL,
		openIDEndpoint: defaultOpenIDEndpoint,
		webAPIBase:     defaultWebAPIBase,
		communityBase:  defaultCommunityBase,
	}
}

// stripURL remove a URL de um *url.Error. A URL da Web API contém a API key
// na query string e não pode aparecer em log.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}
