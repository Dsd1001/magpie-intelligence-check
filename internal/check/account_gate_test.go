package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshForkDoesNotBlockProductionWhenModelProbeExpires(t *testing.T) {
	dir := t.TempDir()
	gate := filepath.Join(dir, "gate.js")
	s, err := New(dir, gate)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetTarget("codex/gpt-6.1-sol", true); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(gate)
	if err != nil {
		t.Fatal(err)
	}
	code := string(body)
	if strings.Contains(code, "ctx.reject(") || strings.Contains(code, "MAGPIE_QUALITY_TRIGGER") {
		t.Fatal("fork default gate would still block healthy production requests")
	}
	if !strings.Contains(code, "quality-probe/") || !strings.Contains(code, "body.model.slice") {
		t.Fatal("probe alias unwrap missing")
	}
}
