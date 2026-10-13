package check

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountConsoleRoutesAreAuthenticated(t *testing.T) {
	s := fixture(t)
	h := s.Handler("control-only-secret")
	for _, path := range []string{"/", "/assets/account-dashboard.js", "/assets/accounts.css", "/api/account-state"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s status %d: missing session was allowed", path, w.Code)
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer control-only-secret")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, request)
		if w.Code != 200 {
			t.Errorf("%s status %d", path, w.Code)
		}
	}
}
func TestAccountTargetAPIRequiresAuthentication(t *testing.T) {
	s := fixture(t)
	h := s.Handler("secret")
	body := `{"account":"acct-a","model":"codex/gpt-6.1-sol","enabled":true}`
	unauthed := httptest.NewRequest(http.MethodPost, "/api/account-target", strings.NewReader(body))
	unauthed.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, unauthed)
	if w.Code != 401 {
		t.Fatalf("unauthorized: %d", w.Code)
	}
	if len(s.state.AccountTargets) != 0 {
		t.Fatal("mutation despite unauthenticated request")
	}
	req := httptest.NewRequest(http.MethodPost, "/api/account-target", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authorized: %d %s", w.Code, w.Body.String())
	}
	if len(s.state.AccountTargets) != 1 {
		t.Fatal("missing target")
	}
}

func TestLegacyAccountsRedirectsToMainTab(t *testing.T) {
	s := fixture(t)
	h := s.Handler("secret")
	request := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	request.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/#accounts" {
		t.Fatalf("legacy account page is not redirected: %d %s", w.Code, w.Header().Get("Location"))
	}
}
