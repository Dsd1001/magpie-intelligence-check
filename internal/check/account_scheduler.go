package check

import (
	"context"
	"errors"
	"sort"
	"time"
)

const (
	accountNormalInterval  = 4 * time.Hour
	accountSuspectInterval = 30 * time.Minute
	accountRetryInterval   = 1 * time.Hour
)

// AccountScheduleEnabled is separate from production-key modification.
// Enabling probes never implicitly enables quarantine or changes routing.
func (s *Service) AccountScheduleEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state.Route.ScheduleEnabled
	s.state.Route.ScheduleEnabled = enabled
	if err := s.store.Save(s.state); err != nil {
		s.state.Route.ScheduleEnabled = previous
		return err
	}
	return nil
}

func dueAccountTarget(t *AccountTarget, now time.Time) bool {
	if !t.Enabled {
		return false
	}
	if t.CheckedAt.IsZero() {
		return true
	}
	interval := accountNormalInterval
	if t.ConsecutivePasses == 1 {
		interval = accountSuspectInterval
	}
	switch t.State {
	case "suspect", "checking":
		interval = accountSuspectInterval
	case "unavailable", "manual":
		interval = accountRetryInterval
	}
	// Already quarantined accounts must be periodically rechecked even
	// if their last failure was many hours ago.
	return now.Sub(t.CheckedAt) >= interval
}

// ScheduleOneAccount runs at most one fresh probe. It never starts a
// second probe while one is active, and tests the oldest due account first.
func (s *Service) ScheduleOneAccount(now time.Time) error {
	s.mu.Lock()
	if !s.state.Route.ScheduleEnabled {
		s.mu.Unlock()
		return nil
	}
	if s.accountProbeActive || len(s.running) > 0 {
		s.mu.Unlock()
		return nil
	}
	type pending struct {
		account, model string
		checked        time.Time
	}
	list := []pending{}
	for _, t := range s.state.AccountTargets {
		if dueAccountTarget(t, now) {
			list = append(list, pending{account: t.Account, model: t.Model, checked: t.CheckedAt})
		}
	}
	s.mu.Unlock()
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].checked.Before(list[j].checked) })
	if err := s.TriggerAccount(list[0].account, list[0].model, "milk"); err != nil {
		return err
	}
	return nil
}

// StartAccountScheduler is called by the owned Go service, NOT by New,
// which keeps unit tests and manual local use side-effect-free.
func (s *Service) StartAccountScheduler() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case now := <-ticker.C:
				err := s.ScheduleOneAccount(now)
				if err != nil {
					s.noteSchedulerError(err)
				}
				s.mu.Lock()
				active := s.state.Route.AutoApply
				pending := s.state.Route.Pending
				s.mu.Unlock()
				if !active || pending {
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
				_, err = s.ApplyRouting(ctx, false)
				cancel()
				if err != nil && !errors.Is(err, context.Canceled) {
					s.noteSchedulerError(err)
				}
			}
		}
	}()
}

func (s *Service) noteSchedulerError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Error text only (no private tokens, API responses, or prompts).
	message := err.Error()
	if len(message) > 280 {
		message = message[:280]
	}
	if s.state.Route.LastError == message {
		return
	}
	s.state.Route.LastError = message
	_ = s.store.Save(s.state)
}
