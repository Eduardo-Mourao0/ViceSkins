package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/auth"
	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type fakeAuthService struct {
	user domain.User
	err  error
}

func (f *fakeAuthService) LoginURL(state string) string {
	return "https://steam.test/login?state=" + state
}

func (f *fakeAuthService) LoginWithSteam(context.Context, url.Values) (domain.User, error) {
	return f.user, f.err
}

func (f *fakeAuthService) GetUser(_ context.Context, id int64) (domain.User, error) {
	if id != f.user.ID {
		return domain.User{}, domain.ErrNotFound
	}
	return f.user, nil
}

func newTestRouter(svc AuthService) (http.Handler, *auth.Tokens) {
	tokens := auth.NewTokens("test-secret-0123456789abcdef0123456789", time.Hour)
	return NewRouter(Deps{
		DB:     fakePinger{},
		Auth:   NewAuthHandler(svc, tokens, false, "http://localhost:3000"),
		Tokens: tokens,
	}), tokens
}

func TestLoginSetsStateAndRedirects(t *testing.T) {
	router, _ := newTestRouter(&fakeAuthService{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/steam/login", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	var state string
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			state = c.Value
		}
	}
	if state == "" || !strings.HasSuffix(rec.Header().Get("Location"), "state="+state) {
		t.Fatalf("state cookie %q not in redirect %q", state, rec.Header().Get("Location"))
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	router, _ := newTestRouter(&fakeAuthService{user: domain.User{ID: 1}})
	req := httptest.NewRequest(http.MethodGet, "/auth/steam/callback?state=attacker", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "victim"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCallbackSetsSessionAndMeWorks(t *testing.T) {
	svc := &fakeAuthService{user: domain.User{ID: 7, SteamID: "76561197960287930", DisplayName: "gaben"}}
	router, _ := newTestRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/auth/steam/callback?state=s1", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "s1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302 (body: %s)", rec.Code, rec.Body)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			session = c
		}
	}
	if session == nil || !session.HttpOnly {
		t.Fatal("expected httpOnly session cookie")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	meReq.AddCookie(session)
	meRec := httptest.NewRecorder()
	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), `"steamId":"76561197960287930"`) {
		t.Fatalf("/me = %d %s", meRec.Code, meRec.Body)
	}
}

func TestMeRequiresAuth(t *testing.T) {
	router, _ := newTestRouter(&fakeAuthService{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func (f *fakeAuthService) SetTradeURL(context.Context, int64, string) (domain.User, error) {
	return domain.User{}, nil
}

func (f *fakeAuthService) SetSteamAPIKey(context.Context, int64, string) (domain.User, error) {
	return domain.User{}, nil
}

func (f *fakeAuthService) DeleteSteamAPIKey(context.Context, int64) (domain.User, error) {
	return domain.User{}, nil
}

func TestSteamLoginIsRateLimitedPerIP(t *testing.T) {
	router, _ := newTestRouter(&fakeAuthService{})
	hit := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/auth/steam/login", nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := range 20 {
		if code := hit("10.0.0.1"); code != http.StatusFound {
			t.Fatalf("request %d = %d, want 302", i+1, code)
		}
	}
	if code := hit("10.0.0.1"); code != http.StatusTooManyRequests {
		t.Fatalf("21st request = %d, want 429", code)
	}
	if code := hit("10.0.0.2"); code != http.StatusFound {
		t.Fatalf("other ip = %d, want 302 (limit is per ip)", code)
	}
}
