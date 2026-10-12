package check

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func controllerRequest(t *testing.T, h http.Handler, method, path, payload string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(payload))
	r.Header.Set("Authorization", "Bearer CONTROL_TOKEN")
	if payload != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRoutingControllerHTTPProtectionAndBulkEnrollment(t *testing.T) {
	s, d := routingFixture(t)
	if err := s.ConfigureRouting("probe", false, ""); err != nil {
		t.Fatal(err)
	}
	h := s.Handler("CONTROL_TOKEN")
	for _, url := range []string{"/api/codex-accounts", "/api/routing/preview", "/api/routing/settings"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 401 {
			t.Fatalf("unauthenticated read %s: %d", url, w.Code)
		}
	}
	w := controllerRequest(t, h, "GET", "/api/codex-accounts", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "SECRET_TOKEN_A") || !strings.Contains(w.Body.String(), "\"id\":\"A\"") {
		t.Fatalf("safe account discovery: %d %s", w.Code, w.Body.String())
	}
	// Malformed confirmation must be rejected without enabling routing.
	w = controllerRequest(t, h, "POST", "/api/routing/config", `{"probe_key_id":"probe","auto_apply":true,"expected_codex_count":4}`)
	if w.Code == 200 || s.state.Route.AutoApply {
		t.Fatal("enabled without explicit confirmation")
	}
	w = controllerRequest(t, h, "POST", "/api/routing/config", `{"probe_key_id":"probe","auto_apply":true,"expected_codex_count":4,"schedule_enabled":false,"confirm":"ENABLE_ALL_PRODUCTION_KEYS"}`)
	if w.Code != 200 {
		t.Fatalf("enable route: %d %s", w.Code, w.Body.String())
	}
	w = controllerRequest(t, h, "POST", "/api/account-targets/discover", `{"model":"codex/gpt-6.1-sol","confirm":"ADD_ALL_CODEX_ACCOUNTS"}`)
	if w.Code != 200 || len(s.state.AccountTargets) != 4 {
		t.Fatalf("bulk enrollment: %d %s", w.Code, w.Body.String())
	}
	w = controllerRequest(t, h, "POST", "/api/routing/apply", `{"confirm":"WRONG"}`)
	if w.Code == 200 {
		t.Fatal("applied routes without operation confirmation")
	}
	s.mu.Lock()
	s.state.AccountTargets[accountTargetID("C", "codex/gpt-6.1-sol")].ConsecutiveFails = 3
	s.mu.Unlock()
	w = controllerRequest(t, h, "GET", "/api/routing/preview", "")
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	w = controllerRequest(t, h, "POST", "/api/routing/apply", `{"confirm":"APPLY_QUARANTINE"}`)
	if w.Code != 200 {
		t.Fatalf("apply: %d %s", w.Code, w.Body.String())
	}
	_, keys, err := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if err != nil || len(keys[0].Accounts) != 3 {
		t.Fatal("production key not quarantined")
	}
	w = controllerRequest(t, h, "POST", "/api/routing/restore", `{"confirm":"RESTORE_ALL_PRODUCTION_KEYS"}`)
	if w.Code != 200 {
		t.Fatalf("restore: %d %s", w.Code, w.Body.String())
	}
	if s.state.Route.AutoApply {
		t.Fatal("emergency restore did not disarm")
	}
}

func TestRoutingPreviewResponseOmitsKeySecrets(t *testing.T) {
	s, _ := routingFixture(t)
	h := s.Handler("CONTROL_TOKEN")
	w := controllerRequest(t, h, "GET", "/api/routing/preview", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"PRODUCTION_KEY_SECRET", "UNIQUE_TESTING_GATEWAY_KEY_123456", "SECRET_TOKEN_A"} {
		if strings.Contains(w.Body.String(), word) {
			t.Fatalf("secret %q leaked to dashboard", word)
		}
	}
}
func TestRoutingConfigRejectsIdentityCountMismatch(t *testing.T) {
	s, _ := routingFixture(t)
	h := s.Handler("CONTROL_TOKEN")
	w := controllerRequest(t, h, "POST", "/api/routing/config", `{"probe_key_id":"probe","auto_apply":true,"expected_codex_count":6,"confirm":"ENABLE_ALL_PRODUCTION_KEYS"}`)
	if w.Code == 200 || s.state.Route.AutoApply {
		t.Fatalf("route active despite mismatch: %s", w.Body.String())
	}
	// The test gateway-key file remains readable and unchanged.
	_, err := os.Stat(filepath.Join(os.Getenv("MAGPIE_ACCOUNT_QUALITY_MAGPIE_DIR"), "caller-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
}
