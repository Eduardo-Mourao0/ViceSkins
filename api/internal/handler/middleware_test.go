package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Eduardo-Mourao0/ViceSkins/api/internal/domain"
)

type fakeUsers map[int64]domain.User

func (f fakeUsers) GetUser(_ context.Context, id int64) (domain.User, error) {
	u, ok := f[id]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

func TestRequireAdmin(t *testing.T) {
	users := fakeUsers{1: {ID: 1, Role: "admin"}, 2: {ID: 2, Role: "user"}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := RequireAdmin(users)(ok)

	for _, tt := range []struct {
		userID int64
		want   int
	}{
		{1, http.StatusTeapot},    // admin passa
		{2, http.StatusForbidden}, // usuário comum
		{9, http.StatusForbidden}, // usuário que não existe mais
	} {
		req := httptest.NewRequest(http.MethodGet, "/admin/disputes", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxKeyUserID{}, tt.userID))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("user %d: status %d, want %d", tt.userID, rec.Code, tt.want)
		}
	}
}
