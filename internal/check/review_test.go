package check

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestReviewPersistsWithoutChangingVerdictOrGate(t *testing.T) {
	s := fixture(t)
	s.SetTarget("a/solo", true)
	s.Action("a/solo", "hold")
	s.state.Results = append(s.state.Results, Result{ID: "answer", Model: "a/solo", Prompt: "repeat", Answer: "original", Verdict: "manual", Time: time.Now()})
	if err := s.ReviewResult("answer", "accepted", "checked"); err != nil {
		t.Fatal(err)
	}
	var stored State
	if err := s.store.Load(&stored); err != nil {
		t.Fatal(err)
	}
	r := stored.Results[0]
	if r.Answer != "original" || r.Verdict != "manual" || r.Review != "accepted" || r.ReviewNote != "checked" || r.ReviewedAt.IsZero() {
		t.Fatal(r)
	}
	if stored.Targets["a/solo"].State != "review" {
		t.Fatal("human review changed gate")
	}
	if s.ReviewResult("answer", "passed", "") == nil || s.ReviewResult("missing", "pending", "") == nil {
		t.Fatal("invalid review accepted")
	}
	s.state.Results = append(s.state.Results, Result{ID: "running", Verdict: "running"})
	if s.ReviewResult("running", "accepted", "") == nil {
		t.Fatal("reviewed pending answer")
	}
}

func TestDeleteTargetPreservesHistoryAndRemovesGate(t *testing.T) {
	s := fixture(t)
	s.SetTarget("a/solo", true)
	s.state.Results = append(s.state.Results, Result{ID: "old", Model: "a/solo", Answer: "kept"})
	s.running["a/solo"] = true
	if s.DeleteTarget("a/solo") == nil {
		t.Fatal("deleted active target")
	}
	delete(s.running, "a/solo")
	if err := s.DeleteTarget("a/solo"); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Snapshot()
	if len(state.Targets) != 0 || len(state.Results) != 1 {
		t.Fatal("history removed or target retained")
	}
	gate, _ := os.ReadFile(s.gate)
	if strings.Contains(string(gate), "a/solo") {
		t.Fatal("deleted target remains gated")
	}
}
