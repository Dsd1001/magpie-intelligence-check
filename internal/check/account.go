package check

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AccountTarget represents one (Magpie account, model) pair. A model-wide
// passing result must never be treated as proof for a different account.
type AccountTarget struct {
	Account           string    `json:"account"`
	Model             string    `json:"model"`
	Enabled           bool      `json:"enabled"`
	State             string    `json:"state"`
	Reason            string    `json:"reason,omitempty"`
	CheckedAt         time.Time `json:"checked_at,omitempty"`
	ConsecutivePasses int       `json:"consecutive_passes"`
	ConsecutiveFails  int       `json:"consecutive_fails"`
}

type AccountResult struct {
	ID      string    `json:"id"`
	Account string    `json:"account"`
	Model   string    `json:"model"`
	Prompt  string    `json:"prompt"`
	Verdict string    `json:"verdict"`
	Time    time.Time `json:"time"`
	Answer  string    `json:"answer,omitempty"`
	Error   string    `json:"error,omitempty"`
	Tokens  int       `json:"tokens"`
}

func ValidateAccount(account string) error {
	if account == "" || len(account) > 256 || strings.TrimSpace(account) != account {
		return errors.New("请输入有效的 Magpie 账号名")
	}
	for _, r := range account {
		if r < 0x20 || r == 0x7f {
			return errors.New("账号名不能包含控制字符")
		}
	}
	return nil
}

func validateAccountModel(model string) error {
	if model == "" || len(model) > 512 || !strings.Contains(model, "/") || strings.HasPrefix(model, "quality-probe/") {
		return errors.New("请输入完整模型 ID（provider/model）")
	}
	for _, r := range model {
		if r < 0x20 || r == 0x7f {
			return errors.New("模型 ID 不能包含控制字符")
		}
	}
	return nil
}

func accountTargetID(account, model string) string { return account + "\x1f" + model }

// SetAccountTarget enrolls an account and model; it does not change Magpie
// routes or credential availability. No API calls are made here.
func (s *Service) SetAccountTarget(account, model string, enabled bool) error {
	if err := ValidateAccount(account); err != nil {
		return err
	}
	if err := validateAccountModel(model); err != nil {
		return err
	}
	id := accountTargetID(account, model)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.AccountTargets == nil {
		s.state.AccountTargets = map[string]*AccountTarget{}
	}
	before := s.state.AccountTargets[id]
	if before == nil {
		before = &AccountTarget{Account: account, Model: model, State: "unverified"}
		s.state.AccountTargets[id] = before
	}
	previous := *before
	before.Enabled = enabled
	if err := s.saveLocked(); err != nil {
		*before = previous
		return err
	}
	return nil
}

func (s *Service) gatewayKeyConfigured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Config.GatewayKey != ""
}

func (s *Service) DeleteAccountTarget(account, model string) error {
	if err := ValidateAccount(account); err != nil {
		return err
	}
	if err := validateAccountModel(model); err != nil {
		return err
	}
	id := accountTargetID(account, model)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountProbeActive {
		return errors.New("账号检测进行中，暂不能删除目标")
	}
	t, exists := s.state.AccountTargets[id]
	if !exists {
		return errors.New("账号监测目标不存在")
	}
	delete(s.state.AccountTargets, id)
	if err := s.saveLocked(); err != nil {
		s.state.AccountTargets[id] = t
		return err
	}
	return nil
}

// TriggerAccount checks exactly one account and model, with a global
// single-flight guard for small VPS deployments. Suspected quality degradation
// is recorded, not automatically equated with an authentication/quota error.
// Route mutations are deliberately NOT performed by this method.
func (s *Service) TriggerAccount(account, model, prompt string) error {
	if err := ValidateAccount(account); err != nil {
		return err
	}
	if err := validateAccountModel(model); err != nil {
		return err
	}
	p, ok := FindPrompt(prompt)
	if !ok {
		return errors.New("未知检测题目")
	}
	id := accountTargetID(account, model)
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.state.AccountTargets[id]
	if target == nil || !target.Enabled {
		return errors.New("该账号及模型尚未启用监测")
	}
	if s.accountProbeActive || len(s.running) > 0 {
		return errors.New("已有检测正在执行；每次只允许一个探针")
	}
	cfg := s.state.Config
	if cfg.GatewayURL == "" || cfg.GatewayKey == "" {
		return errors.New("请先配置专用检测 Gateway Key 和 Magpie URL")
	}
	day := time.Now().UTC().Format("2006-01-02")
	count := 0
	for _, r := range s.state.AccountResults {
		if r.Time.UTC().Format("2006-01-02") == day {
			count++
		}
	}
	for _, r := range s.state.Results {
		if r.Time.UTC().Format("2006-01-02") == day {
			count++
		}
	}
	if count+2 > cfg.DailyLimit {
		return errors.New("检测预算不足，无法安全启动独立账号复测")
	}
	target.State = "checking"
	target.Reason = ""
	if err := s.saveLocked(); err != nil {
		return err
	}
	s.accountProbeActive = true
	alias := s.state.Alias
	s.wg.Add(1)
	go s.runAccount(s.ctx, id, account, model, alias, cfg, p)
	return nil
}

func (s *Service) runAccount(parent context.Context, id, account, model, alias string, cfg Config, p Prompt) {
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(parent, 180*time.Second)
	defer cancel()
	verdict, reason := "unavailable", ""
	maxAttempts := 1
	if p.Automatic {
		maxAttempts = 2
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// The special prefix bypasses this fork's legacy model-level gate.
		// Unlike an unpinned probe, X-Magpie-Account guarantees no fallback.
		answer, tokens, err := callWithAccount(ctx, cfg, "quality-probe/"+alias+"/"+model, account, p.Text, nil)
		record := AccountResult{
			ID: Secret()[:12], Account: account, Model: model, Prompt: p.ID,
			Time: time.Now(), Tokens: tokens, Answer: answer,
		}
		if len(record.Answer) > 8*1024 {
			record.Answer = record.Answer[:8*1024]
		}
		if err != nil {
			record.Verdict, record.Error = "unavailable", err.Error()
			verdict, reason = "unavailable", err.Error()
		} else {
			record.Verdict = Grade(p.ID, answer)
			verdict = record.Verdict
		}
		s.mu.Lock()
		s.state.AccountResults = append(s.state.AccountResults, record)
		if len(s.state.AccountResults) > 300 {
			s.state.AccountResults = append([]AccountResult{}, s.state.AccountResults[len(s.state.AccountResults)-300:]...)
		}
		_ = s.store.Save(s.state)
		s.mu.Unlock()
		if verdict != "failed" || ctx.Err() != nil {
			break // quota/network failures are NOT evidence of degradation
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountProbeActive = false
	t := s.state.AccountTargets[id]
	if t == nil {
		return
	}
	t.CheckedAt = time.Now()
	switch verdict {
	case "passed":
		t.State, t.Reason = "passed", ""
		t.ConsecutivePasses++
		t.ConsecutiveFails = 0
	case "failed":
		t.ConsecutiveFails++
		t.ConsecutivePasses = 0
		t.State = "suspect"
		t.Reason = fmt.Sprintf("质量题复测未通过（连续 %d 轮）；尚未自动隔离", t.ConsecutiveFails)
	case "manual":
		t.State = "manual"
		t.Reason = "人工题不能用于自动判定账号质量"
	default:
		t.State, t.Reason = "unavailable", reason
		// Do not change quality counters on 429, network errors or timeouts.
	}
	_ = s.saveLocked()
}

// DiscoverAccountTargets enrolls every Codex account that Magpie stored.
// Existing results are kept. One SQLite write updates the entire set.
func (s *Service) DiscoverAccountTargets(model string) (int, error) {
	if err := validateAccountModel(model); err != nil {
		return 0, err
	}
	dir, err := MagpieDir()
	if err != nil {
		return 0, err
	}
	accounts, err := DiscoverCodexAccounts(dir)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountProbeActive {
		return 0, errors.New("检测进行中，暂不能批量修改目标")
	}
	before := make(map[string]*AccountTarget, len(s.state.AccountTargets))
	for k, v := range s.state.AccountTargets {
		c := *v
		before[k] = &c
	}
	for _, acct := range accounts {
		id := accountTargetID(acct.ID, model)
		if _, ok := s.state.AccountTargets[id]; !ok {
			s.state.AccountTargets[id] = &AccountTarget{Account: acct.ID, Model: model, Enabled: true, State: "unverified"}
		}
	}
	if err := s.saveLocked(); err != nil {
		s.state.AccountTargets = before
		return 0, err
	}
	return len(accounts), nil
}
