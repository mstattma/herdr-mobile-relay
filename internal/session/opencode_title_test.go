package session

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0cv/herdr-mobile-relay/internal/agentroots"
)

func TestOpenCodeTitleCacheRefreshesExactNativeSession(t *testing.T) {
	binary, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 unavailable for fixture creation")
	}
	root := t.TempDir()
	database := filepath.Join(root, "opencode.db")
	run := func(sql string) {
		t.Helper()
		cmd := exec.Command(binary, database)
		cmd.Stdin = strings.NewReader(sql)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture SQL: %v: %s", err, output)
		}
	}
	run(`CREATE TABLE session(id TEXT PRIMARY KEY,title TEXT,directory TEXT);
CREATE TABLE message(id TEXT PRIMARY KEY); CREATE TABLE part(id TEXT PRIMARY KEY);
INSERT INTO session VALUES('ses_title','Native title','/work/native');`)
	t.Setenv(agentroots.OpenCodeListEnv, root)
	r := NewResolver(t.TempDir())
	if title := r.SessionName("opencode", "/work/launcher", "ses_title"); title != "Native title" {
		t.Fatalf("title = %q", title)
	}
	run(`UPDATE session SET title='Renamed title';`)
	if title := r.SessionName("opencode", "/work/other", "ses_title"); title != "Native title" {
		t.Fatalf("poll did not use cache: %q", title)
	}
	entry := r.cache["opencode|ses_title"]
	entry.expires = time.Now().Add(-time.Second)
	r.cache["opencode|ses_title"] = entry
	if title := r.SessionName("opencode", "/work/launcher", "ses_title"); title != "Renamed title" {
		t.Fatalf("title did not refresh: %q", title)
	}
	if title := r.SessionName("opencode", "/work/native", "ses_missing"); title != "" {
		t.Fatalf("missing session borrowed another title: %q", title)
	}
}
