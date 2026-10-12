package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCodexFixture(t *testing.T, raw string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logins.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDiscoverCodexAccountsUsesStableIDsAndNeverReadsSecrets(t *testing.T) {
	dir := writeCodexFixture(t, `[
		{"id":"stable-a","agent":"codex","user":"a@example.com","auth":{"access_token":"DO_NOT_EXPOSE"}},
		{"id":"stable-b","agent":"codex","user":"b@example.com","auth":{"refresh_token":"DO_NOT_EXPOSE"}},
		{"id":"other","agent":"claude","user":"c@example.com","auth":{"access_token":"DO_NOT_EXPOSE"}},
		{"id":"hidden","agent":"codex","user":"h@example.com","hidden":"no"}
	]`)
	acc, err := DiscoverCodexAccounts(dir)
	if err != nil || len(acc) != 2 || acc[1].ID != "stable-b" {
		t.Fatalf("discovery %+v %v", acc, err)
	}
	body, err := json.Marshal(acc)
	if err != nil || strings.Contains(string(body), "DO_NOT_EXPOSE") {
		t.Fatal("credentials leaked")
	}
	id, err := ResolveAccountID(acc, "B@EXAMPLE.COM")
	if err != nil || id != "stable-b" {
		t.Fatalf("account identity: %q %v", id, err)
	}
}
func TestDiscoverFailsClosedOnNoAccountsAndBadFile(t *testing.T) {
	for _, raw := range []string{`[]`, `bad-json`, `[{"agent":"codex","user":"x","id":"same"},{"agent":"codex","user":"y","id":"same"}]`} {
		if _, err := DiscoverCodexAccounts(writeCodexFixture(t, raw)); err == nil {
			t.Fatalf("accepted unusable account source %s", raw)
		}
	}
}
func TestDiscoverMatchesMagpieLegacyID(t *testing.T) {
	a, err := DiscoverCodexAccounts(writeCodexFixture(t, `[{"agent":"codex","user":"Test@Example.COM"}]`))
	if err != nil || len(a) != 1 || len(a[0].ID) != 16 {
		t.Fatal(a, err)
	}
}
