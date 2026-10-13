package check

import (
	"reflect"
	"testing"
)

func TestAutoPromptPolicySelectionAndRotation(t *testing.T) {
	s := fixture(t)
	p, err := s.AddCustomPrompt("custom", "Return 1", "", true, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.AutoPromptSettings(); got.Configured || !got.Rotate || !reflect.DeepEqual(got.PromptIDs, []string{"milk", p.ID}) {
		t.Fatalf("old install default: %+v", got)
	}
	if err := s.ConfigureAutoPrompts([]string{p.ID, "milk"}, true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	chosen := s.activeAccountPromptsLocked()
	s.mu.Unlock()
	if len(chosen) != 2 || chosen[0].ID != p.ID {
		t.Fatalf("ordering: %+v", chosen)
	}
	if got := selectAutomaticPrompt(AccountTarget{PromptRotation: 1}, false, true, chosen); got != "milk" {
		t.Fatal(got)
	}
	if got := selectAutomaticPrompt(AccountTarget{PromptRotation: 1}, false, false, chosen); got != p.ID {
		t.Fatal(got)
	}
	if got := selectAutomaticPrompt(AccountTarget{LastAutoPrompt: "milk", ConsecutiveFails: 1}, false, false, chosen); got != "milk" {
		t.Fatal("failed round switched question")
	}
	if got := selectAutomaticPrompt(AccountTarget{LastAutoPrompt: "milk", ConsecutivePasses: 1}, true, false, chosen); got != "milk" {
		t.Fatal("recovery switched question")
	}
	for _, ids := range [][]string{{}, {"creative"}, {"milk", "milk"}, {"missing"}} {
		if err := s.ConfigureAutoPrompts(ids, true); err == nil {
			t.Fatalf("accepted invalid IDs %v", ids)
		}
	}
	if err := s.ConfigureAutoPrompts([]string{p.ID}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCustomPrompt(p.ID); err != nil {
		t.Fatal(err)
	}
	got := s.AutoPromptSettings()
	if !reflect.DeepEqual(got.PromptIDs, []string{"milk"}) || got.Rotate {
		t.Fatalf("invalid safe fallback: %+v", got)
	}
}
