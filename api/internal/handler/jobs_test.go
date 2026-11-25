package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testJobsSecret = "0123456789abcdef0123456789abcdef"

func TestJobsRunRequiresSecret(t *testing.T) {
	calls := 0
	h := NewJobsHandler(testJobsSecret, func(context.Context) map[string]error {
		calls++
		return map[string]error{"a": nil, "b": errors.New("db down: secret detail")}
	})

	for _, auth := range []string{"", "Bearer wrong", testJobsSecret, "Bearer " + testJobsSecret + "x"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/internal/jobs/run", nil)
		req.Header.Set("Authorization", auth)
		h.Run(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q: status %d, want 401", auth, rec.Code)
		}
	}
	if calls != 0 {
		t.Fatal("jobs ran without the secret")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs/run", nil)
	req.Header.Set("Authorization", "Bearer "+testJobsSecret)
	h.Run(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || calls != 1 || !strings.Contains(body, `"b":"error"`) {
		t.Fatalf("status %d, calls %d, body %s", rec.Code, calls, body)
	}
	if strings.Contains(body, "secret detail") {
		t.Fatal("error details must stay in the logs")
	}
}

func TestJobsRouteOnlyExistsWithSecret(t *testing.T) {
	router, _ := newTestRouter(&fakeAuthService{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/jobs/run", nil))
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 404/405 without a JobsHandler", rec.Code)
	}
}
