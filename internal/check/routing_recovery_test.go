package check

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func pendingRoutingFixture(t *testing.T) (*Service, string, RoutingPreview) {
	t.Helper()
	s, d := routingFixture(t)
	if err := s.SetAccountTarget("C", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.state.AccountTargets[accountTargetID("C", "codex/gpt-6.1-sol")].ConsecutiveFails = 3
	s.mu.Unlock()
	preview, err := s.RoutingPreview()
	if err != nil {
		t.Fatal(err)
	}
	_, keys, err := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := map[string][]string{}
	expected := map[string][]string{}
	for i, k := range keys {
		before[k.ID] = slices.Clone(k.Accounts)
		expected[k.ID] = slices.Clone(preview.Keys[i].After)
	}
	s.mu.Lock()
	s.state.Route.Pending = true
	s.state.Route.Previous = before
	s.state.Route.Expected = expected
	s.state.Route.PendingQuarantine = []string{"C"}
	if err := s.store.Save(s.state); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	return s, d, preview
}

func TestRoutingRecoveryBeforeWriteDisarms(t *testing.T) {
	s, _, _ := pendingRoutingFixture(t)
	result, err := s.RecoverRoutingJournal(context.Background())
	if err != nil || result != "aborted" {
		t.Fatalf("%s %v", result, err)
	}
	if s.state.Route.AutoApply || s.state.Route.Pending {
		t.Fatal("crashed state not disarmed")
	}
}
func TestRoutingRecoveryAfterWriteCommits(t *testing.T) {
	s, d, preview := pendingRoutingFixture(t)
	path := filepath.Join(d, "caller-keys.json")
	b, _ := os.ReadFile(path)
	var docs []map[string]json.RawMessage
	if err := json.Unmarshal(b, &docs); err != nil {
		t.Fatal(err)
	}
	for i, p := range preview.Keys {
		docs[i]["accounts"], _ = json.Marshal(p.After)
	}
	written, _ := json.Marshal(docs)
	if err := os.WriteFile(path, written, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := s.RecoverRoutingJournal(context.Background())
	if err != nil || result != "completed" {
		t.Fatalf("%s %v", result, err)
	}
	if !s.state.Route.Quarantined["C"] || s.state.Route.Pending {
		t.Fatal("committed state not recorded")
	}
}
func TestRoutingRecoveryRefusesMixedExternalMutation(t *testing.T) {
	s, d, preview := pendingRoutingFixture(t)
	path := filepath.Join(d, "caller-keys.json")
	b, _ := os.ReadFile(path)
	var docs []map[string]json.RawMessage
	_ = json.Unmarshal(b, &docs)
	docs[0]["accounts"], _ = json.Marshal(preview.Keys[0].After)
	written, _ := json.Marshal(docs)
	if err := os.WriteFile(path, written, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverRoutingJournal(context.Background()); err == nil || !strings.Contains(err.Error(), "混合") {
		t.Fatalf("accepted partial write: %v", err)
	}
	if !s.state.Route.Pending {
		t.Fatal("pending cleared despite mismatch")
	}
}
func TestRoutingExpectedAccountCountMismatchFailsClosed(t *testing.T) {
	s, d := routingFixture(t)
	if err := s.ConfigureExpectedAccountCount(6); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RoutingPreview(); err == nil || !strings.Contains(err.Error(), "期望数量") {
		t.Fatalf("discovery mismatch: %v", err)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err == nil {
		t.Fatal("unexpected write")
	}
	_, keys, _ := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if len(keys[0].Accounts) != 0 {
		t.Fatal("modified production key")
	}
}
func TestRoutingExplicitEmergencyRestore(t *testing.T) {
	s, d := routingFixture(t)
	s.mu.Lock()
	s.state.AccountTargets["bad"] = &AccountTarget{Account: "C", Model: "codex/gpt-6.1-sol", Enabled: true, ConsecutiveFails: 3}
	s.mu.Unlock()
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRouting(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	_, keys, _ := loadKeyDocuments(filepath.Join(d, "caller-keys.json"))
	if len(keys[0].Accounts) != 0 || !slices.Contains(keys[1].Accounts, "codex/C") {
		t.Fatal("restore did not return original permissions")
	}
	if s.state.Route.AutoApply || s.state.AccountTargets["bad"].ConsecutiveFails != 0 {
		t.Fatal("restore did not disarm")
	}
}

// An interrupted emergency restore must never re-enable automatic
// quarantine immediately after the operator repairs the journal.
func TestRoutingRecoveryAfterEmergencyRestoreDisarms(t *testing.T) {
	s, d, _ := pendingRoutingFixture(t)
	path := filepath.Join(d, "caller-keys.json")
	s.mu.Lock()
	s.state.Route.PendingRestore = true
	s.state.Route.PendingQuarantine = nil
	if err := s.store.Save(s.state); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	// The pending intended state is already on disk.
	_, keys, err := loadKeyDocuments(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys[0].Accounts) != 0 {
		t.Fatal("invalid fixture")
	}
	// Pending restore can be resumed if the current store equals target.
	s.mu.Lock()
	for _, k := range keys {
		s.state.Route.Expected[k.ID] = slices.Clone(k.Accounts)
	}
	_ = s.store.Save(s.state)
	s.mu.Unlock()
	action, err := s.RecoverRoutingJournal(context.Background())
	if err != nil || action != "completed" {
		t.Fatal(action, err)
	}
	if s.state.Route.AutoApply || s.state.Route.Pending || s.state.Route.Quarantined["C"] {
		t.Fatal("emergency restore was re-armed")
	}
}
