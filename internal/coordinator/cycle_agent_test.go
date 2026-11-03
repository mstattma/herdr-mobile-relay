package coordinator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/0cv/herdr-mobile-relay/internal/herdr"
	"github.com/0cv/herdr-mobile-relay/internal/protocol"
	"github.com/0cv/herdr-mobile-relay/internal/question"
)

func TestCycleAgentFreshPromptAndExactTarget(t *testing.T) {
	composer := "Historical response about questions and permissions.\n\n  \u2503\n  \u2503\n  \u2503\n  \u2503  Plan \u00b7 Example Model ExampleProvider \u00b7 high\n  \u2579" + strings.Repeat("\u2580", 80) + "\n   /work/example  12.0K (4%)  ctrl+p commands\n"
	for _, scenario := range []string{"normal", "home", "native draft", "permission", "question", "picker", "autocomplete", "unknown", "stale", "session changed", "read failed", "disconnected", "unauthorized", "wrong agent", "working", "targetless", "raw keys"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			record := filepath.Join(dir, "writes")
			binary := recordingSecretHerdr(t, dir, record)
			state := NewState(testLogger())
			seed := &AgentState{PaneID: "pane-1", TerminalID: "terminal-1", Agent: "opencode", Status: "idle", SessionID: "ses_cycle", AttentionKind: question.AttentionChat}
			if scenario == "wrong agent" {
				seed.Agent = "codex"
			}
			if scenario == "working" {
				seed.Status = "working"
			}
			state.CommitInventory([]*AgentState{seed}, state.RevisionCounter())
			agent, _ := state.Agent("pane-1")
			message := map[string]any{"type": "cycle_agent", "protocol": protocol.Version, "pane_id": "pane-1", "request_id": "cycle-1", "target": protocol.TargetRef{ServerSessionID: "primary", PaneID: "pane-1", TerminalID: "terminal-1", Generation: agent.Generation, AgentSessionID: "ses_cycle"}}
			content := composer
			switch scenario {
			case "home":
				content = strings.Replace(content, "\n  \u2503\n  \u2503\n  \u2503\n", "\n  \u2503\n  \u2503  Ask anything\u2026 \"Fix broken tests\"\n  \u2503\n", 1)
				content = strings.Replace(content, "/work/example  12.0K (4%)", "tab agents", 1)
				content += "\n\n\n  /work/example       1.18.29\n"
			case "native draft":
				content = strings.Replace(content, "\n  \u2503\n  \u2503\n  \u2503\n", "\n  \u2503\n  \u2503  Review the example changes\n  \u2503\n", 1)
			case "permission":
				content += "\nPermission required\nAllow once  Reject"
			case "question":
				content = "Which option?\n1. First\n2. Second\nenter submit  esc dismiss\n" + composer
			case "picker":
				content = "Select model  esc\n" + composer
			case "autocomplete":
				content += "\n/new Start new session\ntab complete"
			case "unknown":
				content = "unknown terminal state"
			case "targetless":
				delete(message, "target")
			case "raw keys":
				message["keys"] = []string{"Enter"}
			case "disconnected":
				state.MarkInventoryFailure(errors.New("offline"))
			}
			var afterRead func()
			if scenario == "stale" {
				afterRead = func() { state.BumpGeneration("pane-1") }
			}
			if scenario == "session changed" {
				afterRead = func() {
					next, _ := state.Agent("pane-1")
					next.SessionID = "ses_replaced"
					state.CommitInventory([]*AgentState{next}, state.RevisionCounter())
				}
			}
			ctx := context.Background()
			if scenario == "unauthorized" {
				ctx = herdr.WithDispatchCheck(ctx, func() error { return errors.New("revoked") })
			}
			client := herdr.NewClient(binary, secretReadSocket(t, content, scenario == "read failed", afterRead))
			defer client.Close()
			dispatcher := NewDispatcher(client, state, nil, testLogger())
			defer dispatcher.Close(context.Background())
			result := dispatcher.HandleCycleAgent(ctx, message, func() {})
			if scenario == "normal" || scenario == "home" {
				if !result.OK || result.Phase != "sent" || result.Action != "cycle_agent" {
					t.Fatalf("cycle = %+v", result)
				}
				invocations := recordedInvocations(t, record)
				if !reflect.DeepEqual(invocations, [][]string{{"pane", "send-keys", "pane-1", "Tab"}}) {
					t.Fatalf("unexpected native write: %#v", invocations)
				}
				if !reflect.DeepEqual(result.Data, map[string]any{"sent": true}) {
					t.Fatalf("cycle claimed a selected mode: %#v", result.Data)
				}
			} else {
				if result.OK {
					t.Fatalf("unsafe cycle accepted: %+v", result)
				}
				if _, err := os.Stat(record); err == nil {
					for _, invocation := range recordedInvocations(t, record) {
						if len(invocation) < 2 || invocation[0] != "pane" || invocation[1] != "read" {
							t.Fatalf("rejected cycle mutated terminal: %#v", invocation)
						}
					}
				} else if !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
		})
	}
}
