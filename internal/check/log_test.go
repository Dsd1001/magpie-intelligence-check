package check

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLogDemandAndRotation(t *testing.T) {
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"choices":[{"message":{"content":"46.24 inches"},"finish_reason":"stop"}]}`))
	}))
	defer up.Close()
	s := fixture(t)
	log := filepath.Join(t.TempDir(), "magpie.log")
	os.WriteFile(log, nil, 0600)
	if err := s.Configure(Config{GatewayURL: up.URL, GatewayKey: "fixture", LogPath: log, DailyLimit: 120}); err != nil {
		t.Fatal(err)
	}
	s.SetTarget("a/solo", true)
	s.WatchLog()
	time.Sleep(1200 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("idle service started probe")
	}
	appendTrigger := func() {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("MAGPIE_QUALITY_TRIGGER {\"model\":\"a/solo\"}\n")
		f.Close()
	}
	waitCalls := func(want int32) {
		deadline := time.Now().Add(5 * time.Second)
		for calls.Load() < want {
			if time.Now().After(deadline) {
				t.Fatalf("wanted %d calls, got %d", want, calls.Load())
			}
			time.Sleep(20 * time.Millisecond)
		}
		waitIdle(t, s)
	}
	appendTrigger()
	waitCalls(1)
	appendTrigger()
	time.Sleep(1200 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatal("fresh result was probed")
	}
	s.mu.Lock()
	s.state.Targets["a/solo"].CheckedAt = time.Now().Add(-16 * time.Minute)
	s.saveLocked()
	s.mu.Unlock()
	if err := os.Rename(log, log+".old"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(log, []byte("MAGPIE_QUALITY_TRIGGER {\"model\":\"a/solo\"}\n"), 0600)
	waitCalls(2)
}
