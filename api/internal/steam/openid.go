package steam

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	openIDNS         = "http://specs.openid.net/auth/2.0"
	identifierSelect = "http://specs.openid.net/auth/2.0/identifier_select"
)

// ErrInvalidAssertion indica que o retorno do OpenID não é confiável
// (adulterado, para outro site, ou rejeitado pela Steam).
var ErrInvalidAssertion = errors.New("invalid steam openid assertion")

var claimedIDRe = regexp.MustCompile(`^https://steamcommunity\.com/openid/id/(7656119\d{10})$`)

// LoginURL monta a URL para onde o usuário é redirecionado para logar na Steam.
// state vai no return_to e é conferido no callback (proteção contra login CSRF).
func (c *Client) LoginURL(state string) string {
	v := url.Values{}
	v.Set("openid.ns", openIDNS)
	v.Set("openid.mode", "checkid_setup")
	v.Set("openid.return_to", c.callbackURL+"?state="+url.QueryEscape(state))
	v.Set("openid.realm", c.realm)
	v.Set("openid.identity", identifierSelect)
	v.Set("openid.claimed_id", identifierSelect)
	return c.openIDEndpoint + "?" + v.Encode()
}

// VerifyOpenID valida os parâmetros recebidos no callback e retorna o SteamID64.
// Checagens locais primeiro (baratas), depois confirma a assinatura com a própria
// Steam via check_authentication.
func (c *Client) VerifyOpenID(ctx context.Context, q url.Values) (string, error) {
	if mode := q.Get("openid.mode"); mode != "id_res" {
		return "", fmt.Errorf("%w: mode %q", ErrInvalidAssertion, mode)
	}
	if q.Get("openid.op_endpoint") != c.openIDEndpoint {
		return "", fmt.Errorf("%w: unexpected op_endpoint", ErrInvalidAssertion)
	}
	if !c.validReturnTo(q.Get("openid.return_to")) {
		return "", fmt.Errorf("%w: return_to mismatch", ErrInvalidAssertion)
	}
	claimed := q.Get("openid.claimed_id")
	m := claimedIDRe.FindStringSubmatch(claimed)
	if m == nil || q.Get("openid.identity") != claimed {
		return "", fmt.Errorf("%w: bad claimed_id", ErrInvalidAssertion)
	}

	form := url.Values{}
	for k, vs := range q {
		if strings.HasPrefix(k, "openid.") {
			form[k] = vs
		}
	}
	form.Set("openid.mode", "check_authentication")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.openIDEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("steam check_authentication: %w", stripURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("steam check_authentication: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", fmt.Errorf("steam check_authentication: read body: %w", err)
	}
	if !isValidResponse(string(body)) {
		return "", fmt.Errorf("%w: rejected by steam", ErrInvalidAssertion)
	}
	return m[1], nil
}

func (c *Client) validReturnTo(raw string) bool {
	got, err := url.Parse(raw)
	if err != nil {
		return false
	}
	want, err := url.Parse(c.callbackURL)
	if err != nil {
		return false
	}
	return got.Scheme == want.Scheme && got.Host == want.Host && got.Path == want.Path
}

// A resposta do check_authentication é no formato "chave:valor" por linha.
func isValidResponse(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "is_valid:true" {
			return true
		}
	}
	return false
}
