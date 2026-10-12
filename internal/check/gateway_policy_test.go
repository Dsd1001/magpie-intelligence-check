package check

import (
	"reflect"
	"strings"
	"testing"
)

func TestQuarantineAllProductionKeysButNotProbe(t *testing.T) {
	input := []GatewayKeyScope{
		{ID: "prod1"}, // empty means all accounts
		{ID: "prod2", Accounts: []string{"codex/B", "codex/C", "codex/D"}},
		{ID: "prod3", Accounts: []string{"other/P1"}}, // other providers unrestricted
		{ID: "probe"},
	}
	plan, err := PlanCodexQuarantine(input, "probe", []string{"A", "B", "C", "D", "E", "F"}, []string{"C"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"prod1": {"codex/A", "codex/B", "codex/D", "codex/E", "codex/F"},
		"prod2": {"codex/B", "codex/D"},
		"prod3": {"other/P1", "codex/A", "codex/B", "codex/D", "codex/E", "codex/F"},
		"probe": nil,
	}
	for _, change := range plan {
		if !reflect.DeepEqual(change.After, want[change.KeyID]) {
			t.Errorf("%s got %v, want %v", change.KeyID, change.After, want[change.KeyID])
		}
		if change.KeyID == "probe" && (change.Changed || !change.IsProbe) {
			t.Fatal("probe key was changed")
		}
	}
	// The planner may never change input baselines.
	if len(input[0].Accounts) != 0 || len(input[1].Accounts) != 3 {
		t.Fatal("mutated baseline")
	}
}
func TestQuarantineCanRestoreOriginalScopes(t *testing.T) {
	keys := []GatewayKeyScope{{ID: "prod"}, {ID: "limited", Accounts: []string{"codex/B", "other/X"}}, {ID: "probe"}}
	plan, err := PlanCodexQuarantine(keys, "probe", []string{"A", "B", "C"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range plan {
		if v.Changed || !reflect.DeepEqual(v.After, keys[i].Accounts) {
			t.Fatalf("restoration widened original %s: %+v", v.KeyID, v)
		}
	}
}
func TestQuarantineCannotFailOpen(t *testing.T) {
	tests := []struct {
		name     string
		keys     []GatewayKeyScope
		accounts []string
		bad      []string
		probe    string
	}{
		{"everyone", []GatewayKeyScope{{ID: "prod"}, {ID: "probe"}}, []string{"C"}, []string{"C"}, "probe"},
		{"all-permitted", []GatewayKeyScope{{ID: "prod", Accounts: []string{"codex/C"}}, {ID: "probe"}}, []string{"A", "C"}, []string{"C"}, "probe"},
		{"missing-test-key", []GatewayKeyScope{{ID: "prod"}}, []string{"A", "C"}, []string{"C"}, "probe"},
		{"unknown-account", []GatewayKeyScope{{ID: "prod"}, {ID: "probe"}}, []string{"A"}, []string{"C"}, "probe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, e := PlanCodexQuarantine(tt.keys, tt.probe, tt.accounts, tt.bad)
			if e == nil {
				t.Fatalf("unsafe plan: %+v", p)
			}
		})
	}
}
func TestQuarantineAccountNamesAreStableIDs(t *testing.T) {
	input := []GatewayKeyScope{{ID: "prod"}, {ID: "probe"}}
	for _, acct := range []string{"foo/bar", "control\nheader", ""} {
		_, err := PlanCodexQuarantine(input, "probe", []string{"A", acct}, []string{"A"})
		if err == nil {
			t.Fatalf("accepted invalid stable ID %q", acct)
		}
	}
	plan, err := PlanCodexQuarantine(input, "probe", []string{"A", "B"}, []string{"a"})
	if err != nil || !strings.Contains(strings.Join(plan[0].After, ","), "codex/B") {
		t.Fatal(plan, err)
	}
}
