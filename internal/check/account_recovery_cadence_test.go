package check

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// Verify the entire user-facing sequence without sleeping four hours:
// first wrong -> 2 min -> second wrong -> 2 min -> third wrong -> isolate
// -> wait four hours -> first correct -> 2 min -> second correct -> restore.
func TestFullAccountThreeFailuresAndTwoPassesWithQuarantineCadence(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		answer := "2"
		if n >= 4 {
			answer = "46.24 inches"
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"` + answer + `"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	s, dir := routingFixture(t)
	s.mu.Lock()
	s.state.Config.GatewayURL = up.URL
	s.state.Config.DailyLimit = 240
	s.mu.Unlock()
	const account = "C"
	const model = "codex/gpt-6.1-sol"
	if err := s.SetAccountTarget(account, model, true); err != nil {
		t.Fatal(err)
	}
	if err := s.AccountScheduleEnabled(true); err != nil {
		t.Fatal(err)
	}
	setCheckedAgo := func(age time.Duration) {
		t.Helper()
		s.mu.Lock()
		target := s.state.AccountTargets[accountTargetID(account, model)]
		target.CheckedAt = time.Now().Add(-age)
		s.mu.Unlock()
	}
	getTarget := func() AccountTarget {
		t.Helper()
		s.mu.Lock()
		defer s.mu.Unlock()
		return *s.state.AccountTargets[accountTargetID(account, model)]
	}
	runOne := func(expected int) {
		t.Helper()
		if err := s.ScheduleOneAccount(time.Now()); err != nil {
			t.Fatal(err)
		}
		waitAccountIdle(t, s)
		if n := int(calls.Load()); n != expected {
			t.Fatalf("round %d generated %d requests instead of one", expected, n)
		}
	}
	runOne(1)
	if got := getTarget().ConsecutiveFails; got != 1 {
		t.Fatalf("initial wrong answer: %d fails", got)
	}
	// A failed account must NOT be retested immediately.
	if err := s.ScheduleOneAccount(time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected immediate retest")
	}
	setCheckedAgo(2 * time.Minute)
	runOne(2)
	setCheckedAgo(2 * time.Minute)
	runOne(3)
	if got := getTarget().ConsecutiveFails; got != 3 {
		t.Fatalf("three wrong answers: %d failures", got)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !s.state.Route.Quarantined[account] {
		t.Fatal("third failed check did not isolate the account")
	}
	// Even though a quarantined account is still marked suspect, it now
	// waits four hours between ordinary rechecks.
	setCheckedAgo(3*time.Hour + 59*time.Minute)
	if err := s.ScheduleOneAccount(time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("isolated account was checked before 4 hours")
	}
	setCheckedAgo(4 * time.Hour)
	runOne(4)
	if got := getTarget(); got.ConsecutivePasses != 1 || got.ConsecutiveFails != 0 {
		t.Fatalf("first recovery probe: %+v", got)
	}
	// The first good answer is insufficient to return to the production pool.
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !s.state.Route.Quarantined[account] {
		t.Fatal("restored routing on only one good answer")
	}
	setCheckedAgo(time.Minute)
	if err := s.ScheduleOneAccount(time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatal("recovery rechecked before 2 minutes")
	}
	setCheckedAgo(2 * time.Minute)
	runOne(5)
	if got := getTarget().ConsecutivePasses; got != 2 {
		t.Fatalf("second recovery confirmation: %d passes", got)
	}
	if _, err := s.ApplyRouting(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if s.state.Route.Quarantined[account] {
		t.Fatal("did not restore routing after second correct answer")
	}
	_, keys, err := loadKeyDocuments(filepath.Join(dir, "caller-keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys[0].Accounts) != 0 || !slices.Contains(keys[1].Accounts, "codex/C") {
		t.Fatal("production keys did not regain their original permissions")
	}
	// After release, the account returns to the ordinary one-hour cadence.
	if dueAccountTarget(ptrAccountTarget(getTarget()), false, time.Now().Add(2*time.Minute)) {
		t.Fatal("released account should not still use two-minute interval")
	}
}

func ptrAccountTarget(target AccountTarget) *AccountTarget { return &target }

func TestAccountOneCorrectAnswerResetsTwoFailures(t *testing.T) {
	// Verifies "consecutive" means uninterrupted incorrect answers.
	s := fixture(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	s.mu.Lock()
	s.state.Config = Config{GatewayURL: up.URL, GatewayKey: "probe-key", DailyLimit: 240}
	s.mu.Unlock()
	const account, model = "C", "codex/gpt-6.1-sol"
	if err := s.SetAccountTarget(account, model, true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	tgt := s.state.AccountTargets[accountTargetID(account, model)]
	tgt.ConsecutiveFails = 2
	tgt.State = "suspect"
	s.mu.Unlock()
	if err := s.TriggerAccount(account, model, "milk"); err != nil {
		t.Fatal(err)
	}
	waitAccountIdle(t, s)
	after, _ := s.Snapshot()
	tgt = after.AccountTargets[accountTargetID(account, model)]
	if tgt.ConsecutiveFails != 0 || tgt.ConsecutivePasses != 1 {
		t.Fatalf("correct answer did not reset consecutive failures: %+v", tgt)
	}
}
