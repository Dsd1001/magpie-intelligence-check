package check

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedProbeDoesNotFallback(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Magpie-Account") != "acct-c@example.com" {
			t.Errorf("probe did not pin requested account: %q", r.Header.Get("X-Magpie-Account"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing dedicated gateway key")
		}
		var data struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&data)
		if data.Model != "codex/gpt-6.1-sol" {
			t.Errorf("model %s", data.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}],"usage":{"total_tokens":7}}`))
	}))
	defer up.Close()
	got, tokens, err := CallAccount(context.Background(), Config{GatewayURL: up.URL, GatewayKey: "test-key"}, "codex/gpt-6.1-sol", "acct-c@example.com", "test")
	if err != nil || tokens != 7 || got != "46.24 inches" || calls != 1 {
		t.Fatalf("result=%q tokens=%d calls=%d error=%v", got, tokens, calls, err)
	}
}

func TestAccountProbesPersistAndAreIndependent(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("X-Magpie-Account") == "acct-b" {
			w.Write([]byte(`{"choices":[{"message":{"content":"48 inches"},"finish_reason":"stop"}],"usage":{"total_tokens":4}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}],"usage":{"total_tokens":4}}`))
	}))
	defer up.Close()
	s := fixture(t)
	s.mu.Lock()
	s.state.Config = Config{GatewayURL: up.URL, GatewayKey: "test-key", DailyLimit: 120}
	s.mu.Unlock()
	for _, account := range []string{"acct-a", "acct-b"} {
		if err := s.SetAccountTarget(account, "codex/gpt-6.1-sol", true); err != nil {
			t.Fatal(err)
		}
		if err := s.TriggerAccount(account, "codex/gpt-6.1-sol", "milk"); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			s.mu.Lock()
			busy := s.accountProbeActive
			s.mu.Unlock()
			if !busy {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("probe timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	state, _ := s.Snapshot()
	a := state.AccountTargets[accountTargetID("acct-a", "codex/gpt-6.1-sol")]
	b := state.AccountTargets[accountTargetID("acct-b", "codex/gpt-6.1-sol")]
	if a.State != "passed" || b.State != "suspect" || len(state.AccountResults) != 3 {
		t.Fatalf("account result mismatch: a=%+v b=%+v results=%d", a, b, len(state.AccountResults))
	}
	if state.Config.GatewayKey != "" || state.Alias != "" {
		t.Fatal("leaked a secret in API state")
	}
	var saved State
	if err := s.store.Load(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.AccountTargets[accountTargetID("acct-b", "codex/gpt-6.1-sol")].State != "suspect" {
		t.Fatal("account grade not persisted")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "check.db")); err != nil {
		t.Fatal(err)
	}
}

func TestAccountProbeValidation(t *testing.T) {
	for _, account := range []string{"", "acct\nX", " acct", strings.Repeat("x", 257)} {
		if ValidateAccount(account) == nil {
			t.Fatalf("accepted %q", account)
		}
	}
}
