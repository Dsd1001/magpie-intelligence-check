package check

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The full account console must remain one authenticated SPA, without a
// floating second site or a second design system.
func TestUnifiedAccountDashboardHasAllViewsAndDropdowns(t *testing.T) {
	contents, err := web.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(contents)
	for _, expected := range []string{
		`id="tab-models"`,
		`id="tab-tests"`,
		`id="tab-fingerprint"`,
		`id="tab-history"`,
		`id="tab-accounts"`,
		`id="tab-routing"`,
		`id="view-accounts"`,
		`id="view-routing"`,
		`<select id="targetModel"`,
		`<select id="accountSelect"`,
		`<select id="accountModel"`,
		`id="accountPrompt"`,
		`id="customPromptManager"`,
		`id="customPromptForm"`,
		`id="customPromptMode"`,
		`id="customPromptAutomaticFields"`,
		`id="customPromptAnswers"`,
		`id="customPromptAutoConfirm"`,
		`id="accountAnswerDialog"`,
		`id="routingProbeKey"`,
		`href="/assets/accounts.css"`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("missing dashboard element %s", expected)
		}
	}
	if strings.Contains(html, `href="/accounts"`) ||
		strings.Contains(html, `position:fixed;bottom:`) ||
		strings.Contains(html, `<input name="model" list=`) {
		t.Fatal("dashboard still uses floating or free-text account detection")
	}
	ids := regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(html, -1)
	seen := map[string]bool{}
	for _, match := range ids {
		if seen[match[1]] {
			t.Fatalf("duplicate HTML ID %s", match[1])
		}
		seen[match[1]] = true
	}
	module, err := web.ReadFile("web/assets/account-dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"account.user", "account.id", "email(t.account)", "email(r.account)", "quarantined.map(email)", "syncSelect($('#accountSelect')"} {
		if !strings.Contains(string(module), want) {
			t.Errorf("missing email mapping: %q", want)
		}
	}
	for _, path := range []string{"web/accounts.html", "web/account-panel.js"} {
		if _, err := web.ReadFile(path); err == nil {
			t.Fatalf("outdated standalone panel still embedded: %s", path)
		}
	}
}

func TestGatewayKeyNamesAPIExposesNoSecretsAndRequiresLogin(t *testing.T) {
	s, _ := routingFixture(t)
	h := s.Handler("control-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/routing/keys", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("API exposed without authentication: %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/routing/keys", nil)
	req.Header.Set("Authorization", "Bearer control-token")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("API failed: %d %s", w.Code, w.Body.String())
	}
	for _, needle := range []string{"PRODUCTION_KEY_SECRET", "UNIQUE_TESTING_GATEWAY_KEY_123456", "OTHER_PRODUCTION_SECRET", "LIMITED_KEY_SECRET"} {
		if strings.Contains(w.Body.String(), needle) {
			t.Fatal("private gateway secret leaked in labels API")
		}
	}
	var payload struct {
		Keys []GatewayKeyLabel `json:"keys"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Keys) != 4 {
		t.Fatalf("labels count: %d", len(payload.Keys))
	}
	if payload.Keys[0].Name != "sub2api" || payload.Keys[3].Name != "quality-probe" {
		t.Fatalf("wrong display names: %+v", payload.Keys)
	}
}
