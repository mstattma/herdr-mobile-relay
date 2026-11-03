package conversation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const sessionMetadataTimeout = 2 * time.Second

type SessionMetadata struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	CWD       string `json:"cwd"`
	database  string
}

type SessionTodo struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type SessionDetails struct {
	SessionID      string        `json:"session_id"`
	Title          string        `json:"title"`
	CWD            string        `json:"cwd"`
	Todos          []SessionTodo `json:"todos"`
	TodosAvailable bool          `json:"todos_available"`
	Reason         string        `json:"reason,omitempty"`
}

// OpenCodeMetadata selects the same database and lazily migrated table as
// history, but only reads the exact session row, never its message bodies.
func (r *Reader) OpenCodeMetadata(ctx context.Context, sessionID string) (SessionMetadata, string) {
	ctx, cancel := context.WithTimeout(ctx, sessionMetadataTimeout)
	defer cancel()
	if !validOpenCodeSessionID(sessionID) {
		return SessionMetadata{}, "invalid_session"
	}
	databases, code := r.openCode.databases()
	if code != "" {
		return SessionMetadata{}, code
	}
	firstFailure := ""
	for index, database := range databases {
		if index == 32 || ctx.Err() != nil {
			return SessionMetadata{}, "source_unavailable"
		}
		metadata, queryCode := r.openCode.metadata(ctx, database, sessionID)
		if queryCode != "" {
			if firstFailure == "" {
				firstFailure = queryCode
			}
			continue
		}
		if metadata.SessionID == sessionID {
			metadata.database = database
			metadata.Title, _ = clampText(sanitizeText(metadata.Title), 512)
			metadata.CWD, _ = clampText(sanitizeText(metadata.CWD), 4096)
			return metadata, ""
		}
	}
	if firstFailure != "" {
		return SessionMetadata{}, firstFailure
	}
	return SessionMetadata{}, "invalid_session"
}

func (r *openCodeReader) metadata(ctx context.Context, database, sessionID string) (SessionMetadata, string) {
	stamp, ok := openCodeStamp(database)
	layout, code := r.layoutContext(ctx, database, stamp, ok)
	if code != "" {
		return SessionMetadata{}, code
	}
	id := "CAST(X'" + hex.EncodeToString([]byte(sessionID)) + "' AS TEXT)"
	selectRow := func(table string) string {
		return fmt.Sprintf(`SELECT id AS session_id,substr(COALESCE(title,''),1,512) AS title,substr(COALESCE(directory,''),1,4096) AS cwd FROM %s WHERE id=%s`, table, id)
	}
	query := selectRow("session")
	if layout.v2 {
		query = selectRow("session_v2")
		if layout.v1 {
			// Match history's v2-with-turns, then v1, then empty-v2 precedence.
			query = `WITH chosen AS (` + query + ` AND (EXISTS(SELECT 1 FROM session_message WHERE session_id=` + id + ` LIMIT 1) OR NOT EXISTS(SELECT 1 FROM session WHERE id=` + id + `))) SELECT * FROM chosen UNION ALL ` + selectRow("session") + ` AND NOT EXISTS(SELECT 1 FROM chosen)`
		}
	}
	raw, code := runSQLiteJSON(ctx, r.queryExec(), database, query+" LIMIT 1;", 32*1024)
	if code != "" {
		return SessionMetadata{}, code
	}
	var rows []SessionMetadata
	if json.Unmarshal(raw, &rows) != nil || len(rows) > 1 {
		return SessionMetadata{}, "source_corrupt"
	}
	if len(rows) == 0 {
		return SessionMetadata{}, ""
	}
	return rows[0], ""
}

func (r *Reader) OpenCodeSessionDetails(ctx context.Context, sessionID string) SessionDetails {
	ctx, cancel := context.WithTimeout(ctx, sessionMetadataTimeout)
	defer cancel()
	details := SessionDetails{SessionID: sessionID, Todos: []SessionTodo{}}
	metadata, code := r.OpenCodeMetadata(ctx, sessionID)
	if code != "" {
		details.Reason = code
		return details
	}
	details.Title, details.CWD = metadata.Title, metadata.CWD
	raw, code := runSQLiteJSON(ctx, r.openCode.queryExec(), metadata.database,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='todo' LIMIT 1;`, 1024)
	var tables []struct{ Name string }
	if code != "" || json.Unmarshal(raw, &tables) != nil {
		details.Reason = "source_unavailable"
		return details
	}
	if len(tables) == 0 {
		details.Reason = "todos_unavailable"
		return details
	}
	query := fmt.Sprintf(`SELECT substr(content,1,1024) AS content,substr(status,1,32) AS status,substr(priority,1,32) AS priority FROM todo WHERE session_id=CAST(X'%s' AS TEXT) ORDER BY position LIMIT 100;`, hex.EncodeToString([]byte(sessionID)))
	raw, code = runSQLiteJSON(ctx, r.openCode.queryExec(), metadata.database, query, 256*1024)
	var todos []SessionTodo
	if code != "" || json.Unmarshal(raw, &todos) != nil {
		details.Reason = "source_unavailable"
		return details
	}
	for _, todo := range todos {
		todo.Content, _ = clampText(sanitizeText(todo.Content), 1024)
		todo.Status, _ = clampText(sanitizeText(todo.Status), 32)
		todo.Priority, _ = clampText(sanitizeText(todo.Priority), 32)
		details.Todos = append(details.Todos, todo)
	}
	details.TodosAvailable = true
	return details
}
