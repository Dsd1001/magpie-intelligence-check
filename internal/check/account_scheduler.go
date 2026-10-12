package check

import (
	"context"
	"errors"
	"sort"
	"time"
)

const (
	accountNormalInterval      = time.Hour
	accountSuspectInterval     = 2 * time.Minute
	accountQuarantinedInterval = 4 * time.Hour
	accountRetryInterval       = time.Hour
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

// dueAccountTarget follows one-answer-per-round semantics:
//
//	healthy: hourly; incorrect answers: every 2 minutes until 3 failures;
//	quarantined: every 4 hours; first correct recovery: retry in 2 minutes.
//
// Failed HTTP/quotas are neutral and retry hourly when not quarantined.
func dueAccountTarget(t *AccountTarget, quarantined bool, now time.Time) bool {
	if !t.Enabled {
		return false
	}
	if t.CheckedAt.IsZero() {
		return true
	}
	interval := accountNormalInterval
	switch {
	case quarantined && t.State == "passed" && t.ConsecutivePasses == 1:
		interval = accountSuspectInterval
	case quarantined:
		interval = accountQuarantinedInterval
	case t.State == "unavailable" || t.State == "manual":
		interval = accountRetryInterval
	case t.State == "suspect" && t.ConsecutiveFails > 0:
		interval = accountSuspectInterval
	}
	return now.Sub(t.CheckedAt) >= interval
}

// quarantinedAccount resolves the persistent Magpie stable ID. The account
// target can contain either a stable ID or a user-visible account name.
func quarantinedAccount(account string, quarantined map[string]bool, known []CodexAccount) bool {
	if quarantined[account] {
		return true
	}
	id, err := ResolveAccountID(known, account)
	return err == nil && quarantined[id]
}

// ScheduleOneAccount runs at most one fresh probe. It never starts a
// second probe while one is active, and tests the oldest due account first.
func (s *Service) ScheduleOneAccount(now time.Time) error {
	s.mu.Lock()
	if !s.state.Route.ScheduleEnabled || s.accountProbeActive || len(s.running) > 0 {
		s.mu.Unlock()
		return nil
	}
	targets := make([]AccountTarget, 0, len(s.state.AccountTargets))
	for _, target := range s.state.AccountTargets {
		targets = append(targets, *target)
	}
	quarantined := make(map[string]bool, len(s.state.Route.Quarantined))
	for id, active := range s.state.Route.Quarantined {
		if active {
			quarantined[id] = true
		}
	}
	s.mu.Unlock()
	var known []CodexAccount
	if len(quarantined) > 0 {
		// Only load identity metadata when isolation is actually in effect.
		// If discovery fails, do not schedule an account at an incorrect rate.
		dir, err := MagpieDir()
		if err != nil {
			return err
		}
		known, err = DiscoverCodexAccounts(dir)
		if err != nil {
			return err
		}
	}
	type pending struct {
		account, model string
		checked        time.Time
		priority       int
	}
	list := []pending{}
	for _, target := range targets {
		isolated := quarantinedAccount(target.Account, quarantined, known)
		if dueAccountTarget(&target, isolated, now) {
			priority := 1
			// An already due two-minute confirmation must not be starved
			// by a large backlog of ordinary hourly checks.
			if (isolated && target.State == "passed" && target.ConsecutivePasses == 1) ||
				(!isolated && target.State == "suspect" && target.ConsecutiveFails > 0) {
				priority = 0
			}
			list = append(list, pending{account: target.Account, model: target.Model, checked: target.CheckedAt, priority: priority})
		}
	}
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].priority != list[j].priority {
			return list[i].priority < list[j].priority
		}
		if list[i].checked.Equal(list[j].checked) {
			if list[i].account == list[j].account {
				return list[i].model < list[j].model
			}
			return list[i].account < list[j].account
		}
		return list[i].checked.Before(list[j].checked)
	})
	return s.TriggerAccount(list[0].account, list[0].model, "milk")
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
