package check

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountProbeScheduleCadence(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name          string
		state         string
		age           time.Duration
		passes, fails int
		quarantined   bool
		enabled       bool
		wantDue       bool
	}{
		{"initial unverified", "unverified", 0, 0, 0, false, true, true},
		{"healthy before hour", "passed", 59 * time.Minute, 2, 0, false, true, false},
		{"healthy exactly hour", "passed", time.Hour, 2, 0, false, true, true},
		{"healthy past hour", "passed", 70 * time.Minute, 2, 0, false, true, true},
		{"first failure before two minutes", "suspect", time.Minute + 59*time.Second, 0, 1, false, true, false},
		{"first failure at two minutes", "suspect", 2 * time.Minute, 0, 1, false, true, true},
		{"second failure at two minutes", "suspect", 2 * time.Minute, 0, 2, false, true, true},
		{"confirmed third failure pending route", "suspect", 3 * time.Minute, 0, 3, false, true, true},
		{"quarantined wrong before four hours", "suspect", 4*time.Hour - time.Second, 0, 3, true, true, false},
		{"quarantined wrong at four hours", "suspect", 4 * time.Hour, 0, 3, true, true, true},
		{"quarantined one good before two min", "passed", time.Minute, 1, 0, true, true, false},
		{"quarantined one good at two min", "passed", 2 * time.Minute, 1, 0, true, true, true},
		{"quarantined awaiting first recovery after 1h", "unavailable", time.Hour, 0, 3, true, true, false},
		{"unavailable before hour", "unavailable", 59 * time.Minute, 0, 0, false, true, false},
		{"unavailable after hour", "unavailable", time.Hour, 0, 0, false, true, true},
		{"healthy after first recovery and route release", "passed", 59 * time.Minute, 2, 0, false, true, false},
		{"disabled always skipped", "suspect", 10 * time.Hour, 0, 2, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := &AccountTarget{
				Enabled: tc.enabled, State: tc.state, CheckedAt: now.Add(-tc.age),
				ConsecutivePasses: tc.passes, ConsecutiveFails: tc.fails,
			}
			if tc.name == "initial unverified" {
				target.CheckedAt = time.Time{}
			}
			if actual := dueAccountTarget(target, tc.quarantined, now); actual != tc.wantDue {
				t.Errorf("state=%s age=%s isolated=%t actualDue=%t expected=%t",
					tc.state, tc.age, tc.quarantined, actual, tc.wantDue)
			}
		})
	}
}

func TestQuarantinedAccountMatchesIDAndDisplayName(t *testing.T) {
	accounts := []CodexAccount{{ID: "stable-c", User: "c@example.com"}}
	quarantined := map[string]bool{"stable-c": true}
	for _, ref := range []string{"stable-c", "C@EXAMPLE.COM"} {
		if !quarantinedAccount(ref, quarantined, accounts) {
			t.Errorf("failed to identify isolated account %q", ref)
		}
	}
	if quarantinedAccount("not-isolated", quarantined, accounts) {
		t.Fatal("accidentally isolated a healthy account")
	}
}

func TestAccountSchedulerDefaultIsOff(t *testing.T) {
	s := fixture(t)
	if err := s.SetAccountTarget("account-a", "codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleOneAccount(time.Now()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountProbeActive {
		t.Fatal("unexpected background probe")
	}
}

func TestScheduledProbeOneAnswerPerRound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"2"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	s := fixture(t)
	s.mu.Lock()
	s.state.Config = Config{GatewayURL: server.URL, GatewayKey: "probe-only", DailyLimit: 240}
	s.mu.Unlock()
	account, model := "account-c", "codex/gpt-6.1-sol"
	if err := s.SetAccountTarget(account, model, true); err != nil {
		t.Fatal(err)
	}
	if err := s.AccountScheduleEnabled(true); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for attempt := 1; attempt <= 3; attempt++ {
		if err := s.ScheduleOneAccount(start); err != nil {
			t.Fatal(err)
		}
		waitAccountIdle(t, s)
		s.mu.Lock()
		target := s.state.AccountTargets[accountTargetID(account, model)]
		if target.ConsecutiveFails != attempt {
			t.Fatalf("round %d: consecutive fails %d", attempt, target.ConsecutiveFails)
		}
		// Simulate 2 minutes passing, without real waiting or extra API calls.
		target.CheckedAt = start.Add(-2 * time.Minute)
		_ = s.store.Save(s.state)
		s.mu.Unlock()
		if got := int(calls.Load()); got != attempt {
			t.Fatalf("round %d issued %d upstream calls (wanted one per round)", attempt, got)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls for 3 failed rounds, got %d", calls.Load())
	}
	state, _ := s.Snapshot()
	if state.AccountTargets[accountTargetID(account, model)].ConsecutiveFails != 3 {
		t.Fatal("third incorrect answer did not reach the quarantine threshold")
	}
}

func waitAccountIdle(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		running := s.accountProbeActive
		s.mu.Unlock()
		if !running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("account probe timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A rapid 2-minute confirmation takes precedence over unrelated overdue
// hourly checks, while the same global one-probe-at-a-time guard is respected.
func TestSuspectTwoMinuteProbePrioritizedOverNormalHourlyBacklog(t *testing.T) {
	var selected atomic.Value
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		selected.Store(r.Header.Get("X-Magpie-Account"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	s := fixture(t)
	s.mu.Lock()
	s.state.Config = Config{GatewayURL: up.URL, GatewayKey: "probe", DailyLimit: 240}
	s.mu.Unlock()
	const model = "codex/gpt-6.1-sol"
	for _, account := range []string{"normal", "suspect"} {
		if err := s.SetAccountTarget(account, model, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AccountScheduleEnabled(true); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.state.AccountTargets[accountTargetID("normal", model)].State = "passed"
	s.state.AccountTargets[accountTargetID("normal", model)].CheckedAt = time.Now().Add(-75 * time.Minute)
	s.state.AccountTargets[accountTargetID("suspect", model)].State = "suspect"
	s.state.AccountTargets[accountTargetID("suspect", model)].ConsecutiveFails = 1
	s.state.AccountTargets[accountTargetID("suspect", model)].CheckedAt = time.Now().Add(-2 * time.Minute)
	s.mu.Unlock()
	if err := s.ScheduleOneAccount(time.Now()); err != nil {
		t.Fatal(err)
	}
	waitAccountIdle(t, s)
	if actual, ok := selected.Load().(string); !ok || actual != "suspect" {
		t.Fatalf("selected %q instead of urgent suspect account", actual)
	}
}

func TestDefaultBudgetSupportsSixAccountsHourly(t *testing.T) {
	s := fixture(t)
	s.mu.Lock()
	limit := s.state.Config.DailyLimit
	s.mu.Unlock()
	// 6 accounts * 24 hourly checks; the remainder covers anomalies.
	if limit < 6*24 {
		t.Fatalf("default %d probes/day cannot sustain six hourly targets", limit)
	}
}
