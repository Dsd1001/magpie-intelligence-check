package check

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func routingFixture(t *testing.T) (*Service, string) {
	t.Helper()
	d := t.TempDir()
	t.Setenv("MAGPIE_ACCOUNT_QUALITY_MAGPIE_DIR", d)
	logins := `[
	{"agent":"codex","user":"A@example.com","id":"A","auth":{"token":"SECRET_TOKEN_A"}},
	{"agent":"codex","user":"B@example.com","id":"B"},
	{"agent":"codex","user":"C@example.com","id":"C"},
	{"agent":"codex","user":"D@example.com","id":"D"}
	]`
	if err := os.WriteFile(filepath.Join(d, "logins.json"), []byte(logins), 0600); err != nil {
		t.Fatal(err)
	}
	keys := `[
	{"id":"prod","secret":"PRODUCTION_KEY_SECRET","name":"sub2api","accounts":[],"models":["codex/*"],"unexpected":{"preserve_me":true}},
	{"id":"limited","secret":"LIMITED_KEY_SECRET","accounts":["codex/B","codex/C","custom/key"]},
	{"id":"newprod","secret":"OTHER_PRODUCTION_SECRET","accounts":["custom/key"]},
	{"id":"probe","secret":"UNIQUE_TESTING_GATEWAY_KEY_123456","name":"quality-probe","accounts":[]}
	]`
	if err := os.WriteFile(filepath.Join(d, "caller-keys.json"), []byte(keys), 0600); err != nil {
		t.Fatal(err)
	}
	s := fixture(t)
	s.mu.Lock()
	s.state.Config.GatewayKey = "UNIQUE_TESTING_GATEWAY_KEY_123456"
	s.state.Config.GatewayURL = "http://127.0.0.1:3425"
	s.mu.Unlock()
	if err := s.ConfigureRouting("probe", true, "ENABLE_ALL_PRODUCTION_KEYS"); err != nil {
		t.Fatal(err)
	}
	return s, d
}

func TestRoutingControllerQuarantineAcrossAllProductionKeysAndRestore(t *testing.T) {
	s, d := routingFixture(t)
	if err := s.SetAccountTarget("C@example.com", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	target := s.state.AccountTargets[accountTargetID("C@example.com", "codex/gpt-6.1-sol")]
	target.ConsecutiveFails = 3
	s.mu.Unlock()
	preview, err := s.RoutingPreview()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Quarantined) != 1 || preview.Quarantined[0] != "C" {
		t.Fatalf("quarantine: %v", preview.Quarantined)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	_, keys, err := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, k := range keys {
		got[k.ID] = k.Accounts
	}
	if !reflect.DeepEqual(got["prod"], []string{"codex/A", "codex/B", "codex/D"}) {
		t.Fatalf("unrestricted production: %v", got["prod"])
	}
	if !reflect.DeepEqual(got["limited"], []string{"codex/B", "custom/key"}) {
		t.Fatalf("restricted production: %v", got["limited"])
	}
	if !slices.Contains(got["newprod"], "custom/key") || slices.Contains(got["newprod"], "codex/C") {
		t.Fatalf("other provider lost permissions: %v", got["newprod"])
	}
	if len(got["probe"]) != 0 {
		t.Fatal("probe key changed")
	}
	raw, err := os.ReadFile(filepath.Join(d, "caller-keys.json"))
	if err != nil || !strings.Contains(string(raw), "PRODUCTION_KEY_SECRET") || !strings.Contains(string(raw), "preserve_me") || strings.Contains(string(raw), "SECRET_TOKEN_A") {
		t.Fatal("key content leaked or lost")
	}
	// A single good round MUST NOT restore C.
	s.mu.Lock()
	target.ConsecutivePasses = 1
	target.ConsecutiveFails = 0
	s.mu.Unlock()
	p1, err := s.RoutingPreview()
	if err != nil || !slices.Contains(p1.Quarantined, "C") {
		t.Fatalf("premature recovery: %+v %v", p1, err)
	}
	// The second good round restores exactly original per-key scope.
	s.mu.Lock()
	target.ConsecutivePasses = 2
	s.mu.Unlock()
	p2, err := s.ApplyRouting(context.Background(), false)
	if err != nil || len(p2.Quarantined) != 0 {
		t.Fatalf("could not restore: %+v %v", p2, err)
	}
	_, keys, err = loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys[0].Accounts) != 0 || !reflect.DeepEqual(keys[1].Accounts, []string{"codex/B", "codex/C", "custom/key"}) || len(keys[3].Accounts) != 0 {
		t.Fatalf("did not restore baselines: %+v", keys)
	}
	if s.state.Route.Pending || len(s.state.Route.Quarantined) != 0 {
		t.Fatal("route journal not committed")
	}
}

func TestRoutingControllerDetectsExternalChangeAndStops(t *testing.T) {
	s, d := routingFixture(t)
	s.mu.Lock()
	s.state.AccountTargets[accountTargetID("C", "codex/gpt-6.1-sol")] = &AccountTarget{Account: "C", Model: "codex/gpt-6.1-sol", Enabled: true, ConsecutiveFails: 3}
	s.mu.Unlock()
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d, "caller-keys.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var docs []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatal(err)
	}
	docs[0]["accounts"] = json.RawMessage(`["codex/B"]`)
	modified, _ := json.Marshal(docs)
	if err := os.WriteFile(path, modified, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err == nil || !strings.Contains(err.Error(), "外部修改") {
		t.Fatalf("missing drift detection: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(modified, after) {
		t.Fatal("manual changes overwritten")
	}
}

func TestRoutingControllerNewKeysAreCovered(t *testing.T) {
	s, d := routingFixture(t)
	s.mu.Lock()
	s.state.AccountTargets["x"] = &AccountTarget{Account: "C", Model: "codex/gpt-6.1-sol", Enabled: true, ConsecutiveFails: 3}
	s.mu.Unlock()
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d, "caller-keys.json")
	raw, _ := os.ReadFile(path)
	var docs []map[string]json.RawMessage
	_ = json.Unmarshal(raw, &docs)
	docs = append(docs, map[string]json.RawMessage{"id": json.RawMessage(`"fresh"`), "secret": json.RawMessage(`"FRESH_SECRET"`), "accounts": json.RawMessage(`[]`)})
	body, _ := json.Marshal(docs)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	_, keys, err := loadKeyDocuments(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range keys {
		if k.ID == "fresh" {
			found = true
			if !reflect.DeepEqual(k.Accounts, []string{"codex/A", "codex/B", "codex/D"}) {
				t.Fatalf("new key was not quarantined: %+v", k)
			}
		}
	}
	if !found {
		t.Fatal("missing new key")
	}
}

func TestRoutingControllerRefusesWrongProbeKeyAndPendingWrites(t *testing.T) {
	s, d := routingFixture(t)
	s.mu.Lock()
	s.state.Config.GatewayKey = "WRONG_RANDOM_KEY_123456"
	s.mu.Unlock()
	if _, err := s.ApplyRouting(context.Background(), false); err == nil || !strings.Contains(err.Error(), "不是独立检测 Key") {
		t.Fatalf("probe mismatch: %v", err)
	}
	s.mu.Lock()
	s.state.Config.GatewayKey = "UNIQUE_TESTING_GATEWAY_KEY_123456"
	s.state.Route.Pending = true
	s.mu.Unlock()
	if _, err := s.ApplyRouting(context.Background(), false); err == nil {
		t.Fatal("pending journal should block a write")
	}
	_, keys, err := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if err != nil || len(keys[0].Accounts) != 0 {
		t.Fatal("route was changed")
	}
}
