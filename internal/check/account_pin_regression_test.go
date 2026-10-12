package check

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Magpie 0.1.1207 uses the user/login name (or a trace ID) to pin
// X-Magpie-Account. The 16-character stable login ID belongs ONLY in
// a Gateway Key account allowlist. Testing the wrong value returns 404.
func TestProbeConvertsStableLoginIDToCorrectHeader(t *testing.T) {
	const stable = "dfbcea9e0035a885"
	const login = "codex-current@example.com"
	useProbeAccountDirectory(t, map[string]string{stable: login})
	var mu sync.Mutex
	var seen []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pin := r.Header.Get("X-Magpie-Account")
		mu.Lock()
		seen = append(seen, pin)
		mu.Unlock()
		if pin != login {
			http.Error(w, "X-Magpie-Account: no account serves this model", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	answer, _, err := CallAccount(context.Background(), Config{GatewayURL: up.URL, GatewayKey: "test-probe"}, "codex/gpt-6.1-sol", stable, "milk")
	if err != nil || answer != "46.24 inches" {
		t.Fatalf("stable id probe failed: %q / %v", answer, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != login {
		t.Fatalf("wrong pinned account: %+v", seen)
	}
}

// A user may rename an account in Magpie. Saved targets must still probe
// the renamed account, using the unchanged stable ID to look it up.
func TestProbeResolvesRenamedAccountOnEveryRound(t *testing.T) {
	const stable = "dfbcea9e0035a885"
	dir := writeCodexFixture(t, `[{"agent":"codex","id":"dfbcea9e0035a885","user":"before@example.com"}]`)
	t.Setenv("MAGPIE_ACCOUNT_QUALITY_MAGPIE_DIR", dir)
	var mu sync.Mutex
	seen := []string{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-Magpie-Account"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	cfg := Config{GatewayURL: up.URL, GatewayKey: "test-probe"}
	if _, _, err := CallAccount(context.Background(), cfg, "codex/gpt-6.1-sol", stable, "milk"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logins.json"), []byte(`[{"agent":"codex","id":"dfbcea9e0035a885","user":"after@example.com"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CallAccount(context.Background(), cfg, "codex/gpt-6.1-sol", stable, "milk"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[0] != "before@example.com" || seen[1] != "after@example.com" {
		t.Fatalf("lost account identity after rename: %+v", seen)
	}
}

func TestProbeUnknownAccountDoesNotFallback(t *testing.T) {
	useProbeAccountDirectory(t, map[string]string{"aaaabbbbccccdddd": "valid@example.com"})
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "should never send", 500)
	}))
	defer up.Close()
	_, _, err := CallAccount(context.Background(), Config{GatewayURL: up.URL, GatewayKey: "probe-key"}, "codex/gpt-6.1-sol", "deadbeefdeadbeef", "milk")
	if err == nil || !strings.Contains(err.Error(), "无法找到检测账号") || calls != 0 {
		t.Fatalf("probe did not fail closed: error=%v calls=%d", err, calls)
	}
}
