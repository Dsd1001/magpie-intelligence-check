package check

import (
	"testing"
	"time"
)

func TestAccountProbeScheduleCadence(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		state  string
		age    time.Duration
		passes int
		due    bool
	}{
		{"passed", 3 * time.Hour, 2, false},
		{"passed", 5 * time.Hour, 2, true},
		{"passed", 35 * time.Minute, 1, true}, // cadence below is checked in scheduler
		{"suspect", 29 * time.Minute, 0, false},
		{"suspect", 31 * time.Minute, 0, true},
		{"unavailable", 55 * time.Minute, 0, false},
		{"unavailable", 65 * time.Minute, 0, true},
	} {
		target := &AccountTarget{Enabled: true, CheckedAt: now.Add(-tc.age), State: tc.state, ConsecutivePasses: tc.passes}
		if got := dueAccountTarget(target, now); got != tc.due {
			t.Errorf("%s age=%v: %v", tc.state, tc.age, got)
		}
	}
}
func TestSchedulerDefaultIsOff(t *testing.T) {
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
