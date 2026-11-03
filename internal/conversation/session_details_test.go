package conversation

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0cv/herdr-mobile-relay/internal/agentroots"
	"github.com/0cv/herdr-mobile-relay/internal/sqliteexec"
)

const sessionDetailsSchema = `
CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT,title TEXT,time_updated INTEGER,agent TEXT);
CREATE TABLE message(id TEXT PRIMARY KEY,session_id TEXT,time_created INTEGER,data TEXT);
CREATE INDEX message_session ON message(session_id);
CREATE TABLE part(id TEXT PRIMARY KEY,message_id TEXT,data TEXT);
INSERT INTO session VALUES('ses_detail','/work/native','Native title',1,'opencode');
INSERT INTO session VALUES('ses_other','/work/other','Other title',2,'opencode');
`

func detailsSQL(t *testing.T, path, sql string) {
	t.Helper()
	binary, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 unavailable for fixture creation")
	}
	command := exec.Command(binary, path)
	command.Stdin = strings.NewReader(sql)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture SQL: %v: %s", err, output)
	}
}

type metadataQueryRecorder struct {
	sqliteexec.Executor
	queries []string
}

func (r *metadataQueryRecorder) QueryJSON(ctx context.Context, database, query string, limit int) ([]byte, error) {
	r.queries = append(r.queries, query)
	return r.Executor.QueryJSON(ctx, database, query, limit)
}

func TestOpenCodeSessionDetailsExactSessionAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "opencode.db")
	detailsSQL(t, database, sessionDetailsSchema+`
CREATE TABLE todo(session_id TEXT,position INTEGER,content TEXT,status TEXT,priority TEXT,PRIMARY KEY(session_id,position));
INSERT INTO todo VALUES('ses_detail',2,'Second','pending','low');
INSERT INTO todo VALUES('ses_other',0,'OTHER_SESSION_SECRET','pending','high');
INSERT INTO todo VALUES('ses_detail',1,'First','in_progress','high');
INSERT INTO message VALUES('msg1','ses_detail',1,'HISTORY_SECRET');`)
	t.Setenv(agentroots.OpenCodeListEnv, root)
	r := NewReader(t.TempDir())
	recorder := &metadataQueryRecorder{Executor: r.openCode.queryExec()}
	r.openCode.executor = recorder
	metadata, code := r.OpenCodeMetadata(context.Background(), "ses_detail")
	if code != "" || metadata.Title != "Native title" || metadata.CWD != "/work/native" || metadata.SessionID != "ses_detail" {
		t.Fatalf("metadata = %+v, %q", metadata, code)
	}
	for _, query := range recorder.queries {
		if strings.Contains(query, "FROM message") || strings.Contains(query, "FROM part") || strings.Contains(query, "COUNT(") {
			t.Fatalf("metadata read history: %s", query)
		}
	}
	details := r.OpenCodeSessionDetails(context.Background(), "ses_detail")
	if !details.TodosAvailable || len(details.Todos) != 2 || details.Todos[0].Content != "First" || details.Todos[1].Content != "Second" || details.Todos[0].Priority != "high" {
		t.Fatalf("details = %+v", details)
	}
	missing := r.OpenCodeSessionDetails(context.Background(), "ses_missing")
	if missing.TodosAvailable || len(missing.Todos) != 0 || missing.SessionID != "ses_missing" || missing.Reason != "invalid_session" {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestOpenCodeSessionDetailsMissingTodoTableAndBounds(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "opencode.db")
	detailsSQL(t, database, sessionDetailsSchema)
	t.Setenv(agentroots.OpenCodeListEnv, root)
	r := NewReader(t.TempDir())
	details := r.OpenCodeSessionDetails(context.Background(), "ses_detail")
	if details.TodosAvailable || details.Todos == nil || details.Title != "Native title" || details.Reason != "todos_unavailable" {
		t.Fatalf("missing table = %+v", details)
	}
	detailsSQL(t, database, `CREATE TABLE todo(session_id TEXT,position INTEGER,content TEXT,status TEXT,priority TEXT,PRIMARY KEY(session_id,position));
WITH RECURSIVE seq(n) AS (SELECT 0 UNION ALL SELECT n+1 FROM seq WHERE n<104)
INSERT INTO todo SELECT 'ses_detail',n,printf('%.*c',2048,'x'),'pending','low' FROM seq;`)
	details = r.OpenCodeSessionDetails(context.Background(), "ses_detail")
	if !details.TodosAvailable || len(details.Todos) != 100 || len(details.Todos[0].Content) != 1024 {
		t.Fatalf("bounded todos = count %d available %v", len(details.Todos), details.TodosAvailable)
	}
}

func TestOpenCodeSessionMetadataMigrationAndChannelPrecedence(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "opencode.db")
	detailsSQL(t, database, sessionDetailsSchema+`
CREATE TABLE session_v2(id TEXT PRIMARY KEY,directory TEXT,title TEXT,time_updated INTEGER,agent TEXT);
CREATE TABLE session_message(id TEXT PRIMARY KEY,session_id TEXT,seq INTEGER,type TEXT,time_created INTEGER,data TEXT);
CREATE INDEX session_message_session ON session_message(session_id,seq);
INSERT INTO session_v2 VALUES('ses_detail','/work/v2','V2 title',3,'opencode');`)
	channel := filepath.Join(root, "opencode-preview.db")
	detailsSQL(t, channel, sessionDetailsSchema+`UPDATE session SET title='Channel title'; INSERT INTO session VALUES('ses_channel','/work/channel','Channel only',1,'opencode');`)
	t.Setenv(agentroots.OpenCodeListEnv, root)
	r := NewReader(t.TempDir())
	assertTitle := func(id, title string) {
		t.Helper()
		metadata, code := r.OpenCodeMetadata(context.Background(), id)
		if code != "" || metadata.Title != title {
			t.Fatalf("%s metadata = %+v, %q, want %q", id, metadata, code, title)
		}
	}
	assertTitle("ses_detail", "Native title")
	assertTitle("ses_channel", "Channel only")
	detailsSQL(t, database, `INSERT INTO session_message VALUES('msg2','ses_detail',1,'user',3,'HISTORY_SECRET');`)
	assertTitle("ses_detail", "V2 title")
	detailsSQL(t, database, `DELETE FROM session_message; DELETE FROM session WHERE id='ses_detail';`)
	assertTitle("ses_detail", "V2 title")
	detailsSQL(t, database, `DROP TABLE part; DROP TABLE message; DROP TABLE session;`)
	assertTitle("ses_detail", "V2 title")
}
