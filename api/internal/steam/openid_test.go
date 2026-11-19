package steam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

const testSteamID = "76561197960287930"

// fakeSteam simula o endpoint OpenID da Steam respondendo ao check_authentication.
func fakeSteam(t *testing.T, valid bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("openid.mode") != "check_authentication" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, "ns:http://specs.openid.net/auth/2.0\nis_valid:%t\n", valid)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(endpoint string) *Client {
	c := NewClient("key", "http://localhost:8080", "http://localhost:8080/auth/steam/callback")
	c.openIDEndpoint = endpoint
	return c
}

func validParams(endpoint string) url.Values {
	claimed := "https://steamcommunity.com/openid/id/" + testSteamID
	return url.Values{
		"state":              {"abc"},
		"openid.ns":          {openIDNS},
		"openid.mode":        {"id_res"},
		"openid.op_endpoint": {endpoint},
		"openid.claimed_id":  {claimed},
		"openid.identity":    {claimed},
		"openid.return_to":   {"http://localhost:8080/auth/steam/callback?state=abc"},
		"openid.sig":         {"sig"},
	}
}

func TestVerifyOpenID(t *testing.T) {
	tests := []struct {
		name       string
		steamValid bool
		mutate     func(q url.Values)
		wantErr    bool
	}{
		{name: "valid", steamValid: true, mutate: func(url.Values) {}},
		{name: "steam rejects", steamValid: false, mutate: func(url.Values) {}, wantErr: true},
		{name: "wrong mode", steamValid: true, mutate: func(q url.Values) { q.Set("openid.mode", "cancel") }, wantErr: true},
		{name: "wrong op_endpoint", steamValid: true, mutate: func(q url.Values) { q.Set("openid.op_endpoint", "https://evil.com/openid") }, wantErr: true},
		{name: "return_to other host", steamValid: true, mutate: func(q url.Values) {
			q.Set("openid.return_to", "http://evil.com/auth/steam/callback?state=abc")
		}, wantErr: true},
		{name: "claimed_id not steam", steamValid: true, mutate: func(q url.Values) {
			q.Set("openid.claimed_id", "https://evil.com/openid/id/"+testSteamID)
			q.Set("openid.identity", "https://evil.com/openid/id/"+testSteamID)
		}, wantErr: true},
		{name: "identity differs from claimed_id", steamValid: true, mutate: func(q url.Values) {
			q.Set("openid.identity", "https://steamcommunity.com/openid/id/76561197960287931")
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeSteam(t, tt.steamValid)
			c := newTestClient(srv.URL)
			q := validParams(srv.URL)
			tt.mutate(q)

			steamID, err := c.VerifyOpenID(context.Background(), q)

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidAssertion) {
					t.Fatalf("err = %v, want ErrInvalidAssertion", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if steamID != testSteamID {
				t.Fatalf("steamID = %q, want %q", steamID, testSteamID)
			}
		})
	}
}

func TestLoginURLCarriesState(t *testing.T) {
	c := newTestClient(defaultOpenIDEndpoint)
	u, err := url.Parse(c.LoginURL("xyz"))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.Query().Get("openid.return_to"); got != "http://localhost:8080/auth/steam/callback?state=xyz" {
		t.Fatalf("return_to = %q", got)
	}
}
