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

func (f *fakeAuthService) DemoUsers(context.Context) ([]domain.User, error) {
	if f.user.IsDemo() {
		return []domain.User{f.user}, nil
	}
	return nil, nil
}

func (f *fakeAuthService) DemoLogin(ctx context.Context, id int64) (domain.User, error) {
	u, err := f.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	if !u.IsDemo() {
		return domain.User{}, domain.ErrForbidden
	}
	return u, nil
}

func newTestRouter(svc AuthService) (http.Handler, *auth.Tokens) {
	return newTestRouterWith(svc, false)
}

func newTestRouterWith(svc AuthService, devLogin bool) (http.Handler, *auth.Tokens) {
	tokens := auth.NewTokens("test-secret-0123456789abcdef0123456789", time.Hour)
	return NewRouter(Deps{
		DB:       fakePinger{},
		Auth:     NewAuthHandler(svc, tokens, false, "http://localhost:3000"),
		Tokens:   tokens,
		DevLogin: devLogin,
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

func TestDevLoginRoutesDoNotExistWhenDisabled(t *testing.T) {
	router, _ := newTestRouterWith(&fakeAuthService{user: domain.User{ID: 1, SteamID: "demo-1"}}, false)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/auth/dev/users", nil),
		httptest.NewRequest(http.MethodPost, "/auth/dev/login", strings.NewReader(`{"userId":1}`)),
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, want 404/405 when DEV_LOGIN is off", req.Method, req.URL.Path, rec.Code)
		}
	}
}

func TestDevLogin(t *testing.T) {
	tests := []struct {
		name    string
		steamID string
		want    int
	}{
		{"demo account gets a session", "demo-1", http.StatusOK},
		{"real steam account is refused", "76561197960287930", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestRouterWith(&fakeAuthService{user: domain.User{ID: 7, SteamID: tt.steamID}}, true)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/dev/login", strings.NewReader(`{"userId":7}`)))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}
			gotCookie := false
			for _, c := range rec.Result().Cookies() {
				gotCookie = gotCookie || (c.Name == sessionCookie && c.Value != "")
			}
			if gotCookie != (tt.want == http.StatusOK) {
				t.Fatalf("session cookie set = %v, want %v", gotCookie, tt.want == http.StatusOK)
			}
		})
	}
}
