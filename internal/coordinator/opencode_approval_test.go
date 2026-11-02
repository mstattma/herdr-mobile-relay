package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0cv/herdr-mobile-relay/internal/herdr"
	"github.com/0cv/herdr-mobile-relay/internal/question"
)

// Minimal OpenCode dialog rows: margin, "┃" border, panel background, and
// a horizontal button row whose focused button has its own background.
func openCodeDialogRow(text string) string {
	return "\x1b[0m\x1b[48;2;10;10;10m  \x1b[0m\x1b[48;2;20;20;20m┃\x1b[0m\x1b[48;2;20;20;20m  " + text + "\x1b[0m"
}

func openCodeButtonRow(labels []string, focus int) string {
	row := "\x1b[0m\x1b[48;2;10;10;10m  \x1b[0m\x1b[48;2;20;20;20m┃\x1b[0m\x1b[48;2;30;30;30m  "
	for index, label := range labels {
		background := "\x1b[48;2;30;30;30m"
		if index == focus {
			background = "\x1b[48;2;245;167;66m"
		}
		row += "\x1b[0m" + background + " " + label + " " + "\x1b[0m\x1b[48;2;30;30;30m  "
	}
	return row + "    ⇆ select  enter confirm\x1b[0m"
}

func openCodePermissionScreen() string {
	return strings.Join([]string{
		openCodeDialogRow("△ Permission required"),
		openCodeDialogRow("  ← Access external directory ~/.local/state/example"),
		openCodeDialogRow(""),
		openCodeButtonRow([]string{"Allow once", "Allow always", "Reject"}, 0),
	}, "\n")
}

func openCodeAlwaysAllowScreen() string {
	return strings.Join([]string{
		openCodeDialogRow("△ Always allow"),
		openCodeDialogRow("This will allow the following patterns until OpenCode is restarted"),
		openCodeDialogRow("- /root/.local/state/example/*"),
		openCodeDialogRow(""),
		openCodeButtonRow([]string{"Confirm", "Cancel"}, 0),
	}, "\n")
}

func commitOpenCodeDialog(t *testing.T, state *State, screen string) question.Classification {
	t.Helper()
	classification := question.Classify(screen, "opencode")
	if classification.Kind != question.AttentionApproval {
		t.Fatalf("screen was not an approval: %+v", classification)
	}
	state.CommitInventory([]*AgentState{{
		PaneID: "pane-1", Agent: "opencode", Status: "blocked",
		AttentionKind:       question.AttentionApproval,
		Prompt:              classification.Prompt,
		Command:             classification.Command,
		Options:             append([]string(nil), classification.Options...),
		ApprovalFingerprint: question.ApprovalFingerprint(classification),
	}}, state.RevisionCounter())
	return classification
}

// OpenCode asks to confirm "Allow always" in a second dialog while the pane
// stays blocked under the same event; both answers must be dispatched.
func TestOpenCodeFollowUpApprovalUnderSameBlockedEvent(t *testing.T) {
	dir := t.TempDir()
	panePath := filepath.Join(dir, "pane.ansi")
	record := filepath.Join(dir, "invocations.log")
	bin := writeScript(t, dir, "herdr", "#!/bin/sh\n"+
		"printf '%s\\n' \"$*\" >> \""+record+"\"\n"+
		"if [ \"$1 $2\" = \"pane read\" ]; then\n"+
		"  cat \""+panePath+"\"\n"+
		"else\n"+
		"  printf '{\"ok\":true}\\n'\n"+
		"fi\n")
	state := NewState(testLogger())
	d := NewDispatcher(herdr.NewClient(bin, filepath.Join(dir, "sock")), state, nil, testLogger())

	respond := func(requestID string, index int, total int) *CommandResult {
		fingerprint, choice := approvalIdentity(t, d, "pane-1", index)
		return d.Handle(context.Background(), map[string]any{
			"action":               "respond",
			"request_id":           requestID,
			"pane_id":              "pane-1",
			"event_id":             blockedEventID(t, d, "pane-1"),
			"approval_fingerprint": fingerprint,
			"choice":               choice,
			"index":                float64(index),
			"total":                float64(total),
		})
	}

	if err := os.WriteFile(panePath, []byte(openCodePermissionScreen()), 0o600); err != nil {
		t.Fatal(err)
	}
	commitOpenCodeDialog(t, state, openCodePermissionScreen())
	firstEvent := blockedEventID(t, d, "pane-1")
	if result := respond("allow-always", 1, 3); !result.OK || result.Phase != "accepted" {
		t.Fatalf("allow always result = %+v", result)
	}

	if err := os.WriteFile(panePath, []byte(openCodeAlwaysAllowScreen()), 0o600); err != nil {
		t.Fatal(err)
	}
	commitOpenCodeDialog(t, state, openCodeAlwaysAllowScreen())
	if blockedEventID(t, d, "pane-1") != firstEvent {
		t.Fatal("test precondition: follow-up dialog should share the blocked event")
	}
	if result := respond("confirm", 0, 2); !result.OK || result.Phase != "accepted" {
		t.Fatalf("confirm result = %+v", result)
	}

	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "pane send-keys pane-1 Right Enter") ||
		!strings.Contains(log, "pane send-keys pane-1 Enter") {
		t.Fatalf("expected Right Enter then Enter, got:\n%s", log)
	}
}

// "Cancel" on the "Always allow" confirmation redraws the original permission
// under the same blocked event; answering it again must dispatch new keys.
func TestOpenCodeRedrawnPermissionCanBeAnsweredAgain(t *testing.T) {
	dir := t.TempDir()
	panePath := filepath.Join(dir, "pane.ansi")
	record := filepath.Join(dir, "invocations.log")
	bin := writeScript(t, dir, "herdr", "#!/bin/sh\n"+
		"printf '%s\\n' \"$*\" >> \""+record+"\"\n"+
		"if [ \"$1 $2\" = \"pane read\" ]; then\n"+
		"  cat \""+panePath+"\"\n"+
		"else\n"+
		"  printf '{\"ok\":true}\\n'\n"+
		"fi\n")
	state := NewState(testLogger())
	d := NewDispatcher(herdr.NewClient(bin, filepath.Join(dir, "sock")), state, nil, testLogger())
	respond := func(requestID string, index, total int) *CommandResult {
		fingerprint, choice := approvalIdentity(t, d, "pane-1", index)
		return d.Handle(context.Background(), map[string]any{
			"action": "respond", "request_id": requestID, "pane_id": "pane-1",
			"event_id": blockedEventID(t, d, "pane-1"), "approval_fingerprint": fingerprint,
			"choice": choice, "index": float64(index), "total": float64(total),
		})
	}
	show := func(screen string) {
		if err := os.WriteFile(panePath, []byte(screen), 0o600); err != nil {
			t.Fatal(err)
		}
		commitOpenCodeDialog(t, state, screen)
	}

	show(openCodePermissionScreen())
	if result := respond("always", 1, 3); !result.OK || result.Phase != "accepted" {
		t.Fatalf("allow always = %+v", result)
	}
	show(openCodeAlwaysAllowScreen())
	if result := respond("cancel", 1, 2); !result.OK || result.Phase != "accepted" {
		t.Fatalf("cancel = %+v", result)
	}
	show(openCodePermissionScreen())
	if result := respond("reject", 2, 3); !result.OK || result.Phase != "accepted" {
		t.Fatalf("answer after redraw = %+v", result)
	}

	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "pane send-keys"); got != 3 {
		t.Fatalf("send-keys invocations = %d, want 3:\n%s", got, data)
	}
	if !strings.Contains(string(data), "pane send-keys pane-1 Right Right Enter") {
		t.Fatalf("reject after redraw was not dispatched:\n%s", data)
	}
}
