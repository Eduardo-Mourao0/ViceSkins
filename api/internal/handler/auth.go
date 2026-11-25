package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

const (
	sessionCookie = "vs_session"
	stateCookie   = "vs_login_state"
)

type AuthService interface {
	LoginURL(state string) string
	LoginWithSteam(ctx context.Context, q url.Values) (domain.User, error)
	GetUser(ctx context.Context, id int64) (domain.User, error)
	SetTradeURL(ctx context.Context, userID int64, raw string) (domain.User, error)
	SetSteamAPIKey(ctx context.Context, userID int64, raw string) (domain.User, error)
	DeleteSteamAPIKey(ctx context.Context, userID int64) (domain.User, error)
}

type AuthHandler struct {
	svc          AuthService
	tokens       TokenIssuer
	cookieSecure bool
	redirectURL  string // para onde o usuário vai depois do login
}

func NewAuthHandler(svc AuthService, tokens TokenIssuer, cookieSecure bool, redirectURL string) *AuthHandler {
	return &AuthHandler{svc: svc, tokens: tokens, cookieSecure: cookieSecure, redirectURL: redirectURL}
}

// GET /auth/steam/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:  stateCookie,
		Value: state,
		// Path "/": em produção o callback chega pelo proxy do Next
		// (/api/auth/steam/callback), e um Path "/auth/steam" não casaria.
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.svc.LoginURL(state), http.StatusFound)
}

// GET /auth/steam/callback
//
// Erros não aparecem como JSON: o usuário chegou aqui navegando, vindo da
// Steam. Ele volta para o site com um código curto na URL (?login=erro&
// motivo=...), e o front mostra a mensagem. O detalhe técnico fica no log.
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	c, err := r.Cookie(stateCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		// Sem o cookie (expirou, outra aba, outro endereço do site) ou com
		// outro valor (login forjado): nos dois casos, não entra.
		slog.Warn("steam login: invalid state", "has_cookie", err == nil)
		h.loginFailed(w, r, loginExpired)
		return
	}
	h.clearCookie(w, stateCookie, "/")

	user, err := h.svc.LoginWithSteam(r.Context(), q)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthenticated) {
			slog.Warn("steam login rejected", "err", err)
			h.loginFailed(w, r, loginRejected)
			return
		}
		slog.Error("steam login", "err", err)
		h.loginFailed(w, r, loginUnavailable)
		return
	}

	if !h.startSession(w, user.ID) {
		return
	}
	http.Redirect(w, r, h.redirectURL, http.StatusFound)
}

// Motivos de falha no login, como aparecem na URL. Lista fechada: nada da
// requisição ou do erro vai para a URL.
const (
	loginExpired     = "expirou"
	loginRejected    = "recusado"
	loginUnavailable = "indisponivel"
)

// loginFailed manda o usuário para a página inicial do site com o motivo.
// O destino vem da configuração (origem do AUTH_REDIRECT_URL), nunca da
// requisição: não dá para usar o callback como redirecionamento aberto.
func (h *AuthHandler) loginFailed(w http.ResponseWriter, r *http.Request, reason string) {
	home := "/"
	if u, err := url.Parse(h.redirectURL); err == nil && u.Scheme != "" && u.Host != "" {
		home = u.Scheme + "://" + u.Host + "/"
	}
	http.Redirect(w, r, home+"?login=erro&motivo="+reason, http.StatusFound)
}

// startSession emite o JWT e grava o cookie de sessão. Em caso de erro, já
// responde 500 e devolve false.
func (h *AuthHandler) startSession(w http.ResponseWriter, userID int64) bool {
	token, err := h.tokens.Issue(userID)
	if err != nil {
		slog.Error("issue token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.tokens.TTL().Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	return true
}

// POST /auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearCookie(w, sessionCookie, "/")
	w.WriteHeader(http.StatusNoContent)
}

// GET /me (requer autenticação)
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	user, err := h.svc.GetUser(r.Context(), userID)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if err != nil {
		slog.Error("get user", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

// PUT /me/trade-url {"tradeUrl": "https://steamcommunity.com/tradeoffer/new/?..."}
func (h *AuthHandler) SetTradeURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TradeURL string `json:"tradeUrl"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, _ := userIDFrom(r.Context())
	user, err := h.svc.SetTradeURL(r.Context(), userID, req.TradeURL)
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return
	case err != nil:
		slog.Error("set trade url", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *AuthHandler) clearCookie(w http.ResponseWriter, name, path string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// DTO de saída. steamId vai como string: SteamID64 passa do limite de inteiro seguro do JS.
type userResponse struct {
	ID           int64   `json:"id"`
	SteamID      string  `json:"steamId"`
	DisplayName  string  `json:"displayName"`
	AvatarURL    string  `json:"avatarUrl"`
	BalanceCents int64   `json:"balanceCents"`
	TradeURL     *string `json:"tradeUrl"`
	// A key em si nunca sai da API: só se existe e os 4 últimos caracteres.
	HasSteamAPIKey   bool    `json:"hasSteamApiKey"`
	SteamAPIKeyLast4 *string `json:"steamApiKeyLast4"`
	Role             string  `json:"role"`
	DebtCents        int64   `json:"debtCents"` // deve à plataforma: não compra até pagar
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:               u.ID,
		SteamID:          u.SteamID,
		DisplayName:      u.DisplayName,
		AvatarURL:        u.AvatarURL,
		BalanceCents:     u.BalanceCents,
		TradeURL:         u.TradeURL,
		HasSteamAPIKey:   u.HasSteamAPIKey,
		SteamAPIKeyLast4: u.SteamAPIKeyLast4,
		Role:             u.Role,
		DebtCents:        u.DebtCents,
	}
}

// PUT /me/steam-api-key {"apiKey": "..."}
func (h *AuthHandler) SetSteamAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"apiKey"`
	}
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, _ := userIDFrom(r.Context())
	user, err := h.svc.SetSteamAPIKey(r.Context(), userID, req.APIKey)
	h.writeUserResult(w, user, err)
}

// DELETE /me/steam-api-key
func (h *AuthHandler) DeleteSteamAPIKey(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFrom(r.Context())
	user, err := h.svc.DeleteSteamAPIKey(r.Context(), userID)
	h.writeUserResult(w, user, err)
}

func (h *AuthHandler) writeUserResult(w http.ResponseWriter, user domain.User, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, toUserResponse(user))
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvalidAPIKey):
		writeError(w, http.StatusBadRequest, "steam rejected this api key")
	case errors.Is(err, domain.ErrFeatureDisabled):
		writeError(w, http.StatusServiceUnavailable, "steam api keys are not enabled on this server")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "unauthenticated")
	default:
		// Nunca loga a requisição: ela carrega a key.
		slog.Error("update user", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
