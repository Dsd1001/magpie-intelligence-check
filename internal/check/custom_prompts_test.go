package check

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCustomPromptManualAutomaticAndStrictScoring(t *testing.T) {
	s := fixture(t)
	if _, err := s.AddCustomPrompt("ambiguous", "Return one answer", "", true, nil); err == nil {
		t.Fatal("automatic prompts cannot exist without exact expected answers")
	}
	if _, err := s.AddCustomPrompt("bad", "Question", "", true, []string{"1\n2"}); err == nil {
		t.Fatal("multiline expected answer was accepted")
	}
	if _, err := s.AddCustomPrompt("bad", "Question", "", true, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}); err == nil {
		t.Fatal("too many expected answers accepted")
	}
	manual, err := s.AddCustomPrompt("creative question", "Describe a pelican", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manual.Automatic || gradeConfiguredPrompt(manual, "anything") != "manual" {
		t.Fatal("manual custom prompt was graded automatically")
	}
	auto, err := s.AddCustomPrompt("arithmetics", "Only output the answer: 1+0", "", true, []string{" 1 ", "ONE", "one"})
	if err != nil {
		t.Fatal(err)
	}
	if !auto.Automatic || !auto.Custom || !reflect.DeepEqual(auto.ExpectedAnswers, []string{"1", "ONE"}) {
		t.Fatalf("automatic rubric not normalized: %+v", auto)
	}
	for _, tc := range []struct{ answer, verdict string }{
		{"1", "passed"}, {" one ", "passed"}, {"ONE", "passed"},
		{"  ONE\n", "passed"}, {"2", "failed"},
		{"答案是 1", "failed"}, {"11", "failed"}, {"", "failed"},
	} {
		if got := gradeConfiguredPrompt(auto, tc.answer); got != tc.verdict {
			t.Errorf("answer=%q got=%s expected=%s", tc.answer, got, tc.verdict)
		}
	}
	found := s.PromptCatalog()
	if len(found) != len(Prompts)+2 {
		t.Fatal("missing stored custom prompts")
	}
	if !validAutomaticCustom(auto) || validAutomaticCustom(manual) {
		t.Fatal("unsafe automatic recognition")
	}
}

func TestCustomPromptCRUDPersistsAndCannotChangeBuiltins(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, filepath.Join(dir, "gate.js"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.AddCustomPrompt("我的题", "唯一的问题\n输出 1", "期待简短答案", true, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateCustomPrompt("milk", "changed", "bad", "", true, []string{"1"}); err == nil {
		t.Fatal("overwrote built-in question")
	}
	s.Close()
	next, err := New(dir, filepath.Join(dir, "gate.js"))
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	found, ok := func() (Prompt, bool) {
		next.mu.Lock()
		defer next.mu.Unlock()
		return next.findPromptLocked(p.ID)
	}()
	if !ok || !found.Automatic || found.ExpectedAnswers[0] != "1" {
		t.Fatalf("not persisted: %+v", found)
	}
	if err := next.UpdateCustomPrompt(p.ID, "改名", "换题", "note", false, nil); err != nil {
		t.Fatal(err)
	}
	found = next.PromptCatalog()[len(Prompts)]
	if found.Automatic || found.Name != "改名" {
		t.Fatal("manual switch not persisted")
	}
	if err := next.DeleteCustomPrompt(p.ID); err != nil {
		t.Fatal(err)
	}
	if len(next.PromptCatalog()) != len(Prompts) {
		t.Fatal("deletion didn't persist")
	}
	if err := next.DeleteCustomPrompt("milk"); err == nil {
		t.Fatal("deleted built-in prompt")
	}
}

func TestAutoRubricEditClearsStreakButPreservesQuarantineHold(t *testing.T) {
	s := fixture(t)
	p, err := s.AddCustomPrompt("logic", "Return 1", "", true, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountTarget("C", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	id := accountTargetID("C", "codex/gpt-6.1-sol")
	s.mu.Lock()
	target := s.state.AccountTargets[id]
	target.LastAutoPrompt = p.ID
	target.ConsecutiveFails = 3
	target.State = "suspect"
	target.CheckedAt = time.Now()
	s.state.Route.Quarantined = map[string]bool{"C": true}
	s.mu.Unlock()
	if err := s.UpdateCustomPrompt(p.ID, "logic changed", "Return 2", "", true, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	updated := *s.state.AccountTargets[id]
	hold := s.state.Route.Quarantined["C"]
	s.mu.Unlock()
	if updated.ConsecutiveFails != 0 || updated.LastAutoPrompt != "" || updated.State != "unverified" || !updated.CheckedAt.IsZero() {
		t.Fatalf("stale verdict remained after rubric edit: %+v", updated)
	}
	if !hold {
		t.Fatal("rubric edit silently released quarantined account")
	}
	if err := s.DeleteCustomPrompt(p.ID); err != nil {
		t.Fatal(err)
	}
	if !s.state.Route.Quarantined["C"] {
		t.Fatal("question removal released quarantine")
	}
}

func TestCustomPromptHTTPRequiresAuthAndExposesMode(t *testing.T) {
	s := fixture(t)
	h := s.Handler("CONTROL_TOKEN")
	noAuth := httptest.NewRecorder()
	h.ServeHTTP(noAuth, httptest.NewRequest(http.MethodPost, "/api/prompts/custom", strings.NewReader(`{"name":"bad","text":"one"}`)))
	if noAuth.Code != 401 {
		t.Fatal("endpoint allows unauthenticated writes")
	}
	post := controllerRequest(t, h, http.MethodPost, "/api/prompts/custom", `{"name":"custom","text":"Reply 1 only","automatic":true,"expected_answers":["1"]}`)
	if post.Code != 200 {
		t.Fatalf("add %d %s", post.Code, post.Body.String())
	}
	var p Prompt
	if err := json.Unmarshal(post.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if !p.Automatic || len(p.ExpectedAnswers) != 1 || !p.Custom {
		t.Fatal("mode not exposed correctly")
	}
	get := controllerRequest(t, h, http.MethodGet, "/api/state", "")
	if !strings.Contains(get.Body.String(), p.ID) || !strings.Contains(get.Body.String(), "\"automatic\":true") {
		t.Fatal("merged prompt catalog omitted automatic question")
	}
	put := controllerRequest(t, h, http.MethodPut, "/api/prompts/custom/"+p.ID, `{"name":"manual now","text":"A creative prompt","automatic":false}`)
	if put.Code != 200 {
		t.Fatalf("switch mode failed: %d %s", put.Code, put.Body.String())
	}
	byID, ok := func() (Prompt, bool) { s.mu.Lock(); defer s.mu.Unlock(); return s.findPromptLocked(p.ID) }()
	if !ok || byID.Automatic {
		t.Fatal("did not disable automatic scoring")
	}
	del := controllerRequest(t, h, http.MethodDelete, "/api/prompts/custom/"+p.ID, "")
	if del.Code != 200 {
		t.Fatalf("delete %d %s", del.Code, del.Body.String())
	}
	if _, ok := func() (Prompt, bool) { s.mu.Lock(); defer s.mu.Unlock(); return s.findPromptLocked(p.ID) }(); ok {
		t.Fatal("deleted question still active")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "check.db")); err != nil {
		t.Fatal(err)
	}
}

func TestManualCustomPromptDoesNotChangeQualityCountersOrTimer(t *testing.T) {
	useProbeAccountDirectory(t, map[string]string{"account-c": "account-c"})
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"2"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	s := fixture(t)
	s.mu.Lock()
	s.state.Config = Config{GatewayURL: up.URL, GatewayKey: "probe-only", DailyLimit: 240}
	s.mu.Unlock()
	p, err := s.AddCustomPrompt("manual question", "Answer freely", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountTarget("account-c", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	id := accountTargetID("account-c", "codex/gpt-6.1-sol")
	when := time.Now().Add(-90 * time.Second).Truncate(time.Second)
	s.mu.Lock()
	target := s.state.AccountTargets[id]
	target.State = "suspect"
	target.ConsecutiveFails = 2
	target.CheckedAt = when
	s.mu.Unlock()
	if err := s.TriggerAccount("account-c", "codex/gpt-6.1-sol", p.ID); err != nil {
		t.Fatal(err)
	}
	waitAccountIdle(t, s)
	s.mu.Lock()
	after := *s.state.AccountTargets[id]
	results := slices.Clone(s.state.AccountResults)
	s.mu.Unlock()
	if calls.Load() != 1 || after.CheckedAt != when || after.ConsecutiveFails != 2 || after.State != "suspect" {
		t.Fatalf("manual changed quality state or timer: calls=%d %+v", calls.Load(), after)
	}
	if len(results) != 1 || results[0].Verdict != "manual" || results[0].PromptName != "manual question" {
		t.Fatalf("manual result not persisted as manual: %+v", results)
	}
}

// Automatic questions are rotated, but a three-fail quarantine and
// two-pass recovery must use the SAME question rather than random samples.
func TestCustomAutomaticProbeRotationQuarantineAndRecovery(t *testing.T) {
	s, dir := routingFixture(t)
	var customCalls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		answer := "46.24 inches"
		if len(payload.Messages) > 0 && strings.Contains(payload.Messages[0].Content, "CUSTOM:") {
			n := customCalls.Add(1)
			answer = "2"
			if n > 3 {
				answer = "1"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
	}))
	defer up.Close()
	s.mu.Lock()
	s.state.Config.GatewayURL = up.URL
	s.state.Config.DailyLimit = 240
	s.mu.Unlock()
	p, err := s.AddCustomPrompt("custom math", "CUSTOM: Output 1 only.", "", true, []string{"1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccountTarget("C", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	if err := s.AccountScheduleEnabled(true); err != nil {
		t.Fatal(err)
	}
	id := accountTargetID("C", "codex/gpt-6.1-sol")
	probe := func(wantPrompt string, after time.Duration, failures, passes int) {
		t.Helper()
		s.mu.Lock()
		if after > 0 {
			s.state.AccountTargets[id].CheckedAt = time.Now().Add(-after)
		}
		s.mu.Unlock()
		if err := s.ScheduleOneAccount(time.Now()); err != nil {
			t.Fatal(err)
		}
		waitAccountIdle(t, s)
		s.mu.Lock()
		tt := *s.state.AccountTargets[id]
		s.mu.Unlock()
		if tt.LastAutoPrompt != wantPrompt || tt.ConsecutiveFails != failures || tt.ConsecutivePasses != passes {
			t.Fatalf("after %s: %+v wanted prompt=%s fails=%d passes=%d", after, tt, wantPrompt, failures, passes)
		}
	}
	// First healthy check uses milk, second hourly check rotates to custom.
	probe("milk", 0, 0, 1)
	probe(p.ID, time.Hour, 1, 0)
	probe(p.ID, 2*time.Minute, 2, 0)
	probe(p.ID, 2*time.Minute, 3, 0)
	if customCalls.Load() != 3 {
		t.Fatal("all confirmations must use same custom question")
	}
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !s.state.Route.Quarantined["C"] {
		t.Fatal("custom rubric did not lead to safe quarantine")
	}
	// On recovery a first correct answer at four hours requires another
	// confirmation after two minutes, with the identical prompt.
	probe(p.ID, 4*time.Hour, 0, 1)
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !s.state.Route.Quarantined["C"] {
		t.Fatal("one correct answer restored routing prematurely")
	}
	probe(p.ID, 2*time.Minute, 0, 2)
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if s.state.Route.Quarantined["C"] {
		t.Fatal("two correct custom answers failed recovery")
	}
	if customCalls.Load() != 5 {
		t.Fatal("automatic custom calls count unexpected")
	}
	_, keys, err := loadKeyDocuments(filepath.Join(dir, "caller-keys.json"))
	if err != nil || len(keys[0].Accounts) != 0 {
		t.Fatal("production white list was not restored")
	}
}
