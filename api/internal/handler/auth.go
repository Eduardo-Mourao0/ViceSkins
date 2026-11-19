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
		Name:     stateCookie,
		Value:    state,
		Path:     "/auth/steam",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.svc.LoginURL(state), http.StatusFound)
}

// GET /auth/steam/callback
func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	c, err := r.Cookie(stateCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		writeError(w, http.StatusBadRequest, "invalid login state")
		return
	}
	h.clearCookie(w, stateCookie, "/auth/steam")

	user, err := h.svc.LoginWithSteam(r.Context(), q)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthenticated) {
			slog.Warn("steam login rejected", "err", err)
			writeError(w, http.StatusUnauthorized, "steam login failed")
			return
		}
		slog.Error("steam login", "err", err)
		writeError(w, http.StatusBadGateway, "could not complete steam login")
		return
	}

	token, err := h.tokens.Issue(user.ID)
	if err != nil {
		slog.Error("issue token", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
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
	http.Redirect(w, r, h.redirectURL, http.StatusFound)
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
	ID           int64  `json:"id"`
	SteamID      string `json:"steamId"`
	DisplayName  string `json:"displayName"`
	AvatarURL    string `json:"avatarUrl"`
	BalanceCents int64  `json:"balanceCents"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{
		ID:           u.ID,
		SteamID:      u.SteamID,
		DisplayName:  u.DisplayName,
		AvatarURL:    u.AvatarURL,
		BalanceCents: u.BalanceCents,
	}
}
