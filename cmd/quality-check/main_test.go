package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
)

func TestRejectSharedDataDirectory(t *testing.T) {
	dir := t.TempDir()
	lock := flock.New(filepath.Join(dir, "service.lock"))
	if err := lock.Lock(); err != nil { t.Fatal(err) }
	defer lock.Unlock()
	err := run(dir, filepath.Join(dir,"gate.js"), "127.0.0.1:0", "", false)
	if err == nil || !strings.Contains(err.Error(), "已有检测服务") { t.Fatalf("shared directory accepted: %v", err) }
}
