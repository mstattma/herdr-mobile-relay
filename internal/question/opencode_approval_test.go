package question

import (
	"reflect"
	"strings"
	"testing"
)

// The builders below reproduce the row structure OpenCode 1.18 renders through
// Herdr's ANSI pane read: a two-cell margin, the "┃" dialog border, the panel
// background, and a sidebar column on the right separated by a wide gap.
const (
	ocReset  = "\x1b[0m"
	ocMargin = ocReset + "\x1b[38;2;255;255;255m\x1b[48;2;10;10;10m  "
	ocBorder = ocReset + "\x1b[38;2;245;167;66m\x1b[48;2;20;20;20m┃"
	ocPanel  = "\x1b[48;2;20;20;20m"
	ocMenu   = "\x1b[48;2;30;30;30m"
	ocFocus  = "\x1b[48;2;245;167;66m"
)

func ocText(background, text string) string {
	return ocReset + "\x1b[38;2;238;238;238m" + background + text
}

func ocSidebar(text string) string {
	return ocReset + ocPanel + strings.Repeat(" ", 30) + ocMargin + ocReset + ocPanel + "  " + text + "    " + ocReset + "\r"
}

func ocRow(text string) string {
	return ocMargin + ocBorder + ocText(ocPanel, "  "+text) + ocSidebar("sidebar · usage 32%")
}

func ocBlank() string {
	return ocMargin + ocBorder + ocText(ocPanel, "") + ocSidebar("")
}

func ocButtons(labels []string, focus int, footer string) string {
	row := ocMargin + ocBorder + ocText(ocMenu, "  ")
	for index, label := range labels {
		if index > 0 {
			row += ocText(ocMenu, "  ")
		}
		background := ocMenu
		if index == focus {
			background = ocFocus
		}
		row += ocText(background, " ") + ocText(background, label) + ocText(background, " ")
	}
	if footer != "" {
		row += ocText(ocMenu, "      "+footer)
	}
	return row + ocSidebar("")
}

func ocScreen(rows ...string) string {
	history := []string{
		ocRow("Earlier assistant output that must not leak into the dialog."),
		ocBlank(),
		ocRow("QUEUED"),
		ocBlank(),
	}
	tail := []string{"", "     ctrl+x down view subagents", ""}
	return strings.Join(append(append(history, rows...), tail...), "\n")
}

const ocHorizontalFooter = "ctrl+f fullscreen  ⇆ select  enter confirm"

func subagentPermissionScreen(focus int) string {
	return ocScreen(
		ocRow("△ Permission required"),
		ocRow("  ← Access external directory ~/.local/state/example-ops"),
		ocRow("  For session: Locate replay (@explore subagent) ("),
		ocRow("               ses_example123) - ."),
		ocBlank(),
		ocRow("Patterns"),
		ocBlank(),
		ocRow("- /root/.local/state/example-ops/*"),
		ocBlank(),
		ocButtons([]string{"Allow once", "Allow always", "Reject"}, focus, ocHorizontalFooter),
	)
}

func TestOpenCodeSubagentPermissionIsApproval(t *testing.T) {
	classification := Classify(subagentPermissionScreen(0), "opencode")
	if classification.Kind != AttentionApproval {
		t.Fatalf("kind = %q, prompt = %q", classification.Kind, classification.Prompt)
	}
	if want := []string{"Allow once", "Allow always", "Reject"}; !reflect.DeepEqual(classification.Options, want) {
		t.Fatalf("options = %q", classification.Options)
	}
	if classification.ApprovalFocus != 0 || classification.ApprovalNavigation != ApprovalNavigationHorizontal {
		t.Fatalf("focus = %d, navigation = %q", classification.ApprovalFocus, classification.ApprovalNavigation)
	}
	wantPrompt := strings.Join([]string{
		"Permission required",
		"← Access external directory ~/.local/state/example-ops",
		"For session: Locate replay (@explore subagent) (",
		"ses_example123) - .",
		"",
		"Patterns",
		"",
		"- /root/.local/state/example-ops/*",
	}, "\n")
	if classification.Prompt != wantPrompt {
		t.Fatalf("prompt = %q", classification.Prompt)
	}
	if classification.Command != "Access external directory ~/.local/state/example-ops" {
		t.Fatalf("command = %q", classification.Command)
	}
	if strings.Contains(classification.Prompt, "sidebar") || strings.Contains(classification.Prompt, "Earlier") {
		t.Fatalf("prompt leaked surrounding content: %q", classification.Prompt)
	}
}

func TestOpenCodePermissionFingerprintIgnoresFocus(t *testing.T) {
	first := Classify(subagentPermissionScreen(0), "opencode")
	moved := Classify(subagentPermissionScreen(2), "opencode")
	if moved.ApprovalFocus != 2 {
		t.Fatalf("focus = %d", moved.ApprovalFocus)
	}
	if ApprovalFingerprint(first) == "" || ApprovalFingerprint(first) != ApprovalFingerprint(moved) {
		t.Fatalf("fingerprints differ: %q vs %q", ApprovalFingerprint(first), ApprovalFingerprint(moved))
	}
}

func TestOpenCodePermissionKeysUseHorizontalNavigation(t *testing.T) {
	classification := Classify(subagentPermissionScreen(1), "opencode")
	tests := map[int][]string{
		0: {"Left", "Enter"},
		1: {"Enter"},
		2: {"Right", "Enter"},
	}
	for target, want := range tests {
		if got := ApprovalKeys(classification, target); !reflect.DeepEqual(got, want) {
			t.Errorf("target %d: keys = %q, want %q", target, got, want)
		}
	}
}

func TestOpenCodePermissionNarrowFooterLine(t *testing.T) {
	screen := ocScreen(
		ocRow("△ Permission required"),
		ocRow("  → Read ~/notes.md"),
		ocBlank(),
		ocButtons([]string{"Allow once", "Allow always", "Reject"}, 2, ""),
		ocRow("ctrl+f fullscreen ⇆ select enter confirm"),
	)
	classification := Classify(screen, "opencode")
	if classification.Kind != AttentionApproval || classification.ApprovalFocus != 2 ||
		len(classification.Options) != 3 {
		t.Fatalf("classification = %+v", classification)
	}
}

func TestOpenCodeAlwaysAllowConfirmation(t *testing.T) {
	screen := ocScreen(
		ocRow("△ Always allow"),
		ocRow("This will allow the following patterns until OpenCode is restarted"),
		ocRow("- /root/.local/state/example-ops/*"),
		ocBlank(),
		ocButtons([]string{"Confirm", "Cancel"}, 0, "⇆ select  enter confirm"),
	)
	classification := Classify(screen, "opencode")
	if classification.Kind != AttentionApproval ||
		!reflect.DeepEqual(classification.Options, []string{"Confirm", "Cancel"}) ||
		classification.ApprovalFocus != 0 {
		t.Fatalf("classification = %+v", classification)
	}
}

func TestOpenCodeRejectPermissionPrompt(t *testing.T) {
	screen := ocScreen(
		ocRow("△ Reject permission"),
		ocRow("Tell OpenCode what to do differently"),
		ocBlank(),
		ocRow(""),
		ocRow("enter confirm  esc cancel"),
	)
	classification := Classify(screen, "opencode")
	if classification.Kind != AttentionApproval ||
		!reflect.DeepEqual(classification.Options, []string{"Reject", "Back"}) ||
		classification.ApprovalNavigation != ApprovalNavigationConfirm {
		t.Fatalf("classification = %+v", classification)
	}
	if got := ApprovalKeys(classification, 0); !reflect.DeepEqual(got, []string{"Enter"}) {
		t.Fatalf("reject keys = %q", got)
	}
	if got := ApprovalKeys(classification, 1); !reflect.DeepEqual(got, []string{"Escape"}) {
		t.Fatalf("back keys = %q", got)
	}
}

func TestOpenCodePermissionRequiresUnambiguousFocus(t *testing.T) {
	if Classify(subagentPermissionScreen(-1), "opencode").Kind == AttentionApproval {
		t.Fatal("dialog without a focused button produced approval keys")
	}
}

func TestOpenCodeHistoricalPermissionIsNotLive(t *testing.T) {
	screen := subagentPermissionScreen(0) + "\n" + ocRow("Continuing with the task.")
	if Classify(screen, "opencode").Kind == AttentionApproval {
		t.Fatal("permission dialog above newer output was treated as live")
	}
}

func TestVerticalApprovalKeysUnchanged(t *testing.T) {
	classification := Classification{ApprovalFocus: 2}
	if got := ApprovalKeys(classification, 0); !reflect.DeepEqual(got, []string{"Up", "Up", "Enter"}) {
		t.Fatalf("keys = %q", got)
	}
}

func TestOpenCodeRemappedFullscreenHintIsNotAnOption(t *testing.T) {
	for _, footer := range []string{
		"fullscreen  ⇆ select  enter confirm",
		"ctrl+g fullscreen  ⇆ select  enter confirm",
		"ctrl+f minimize  ⇆ select  enter confirm",
	} {
		screen := ocScreen(
			ocRow("△ Permission required"),
			ocRow("  → Read ~/notes.md"),
			ocBlank(),
			ocButtons([]string{"Allow once", "Allow always", "Reject"}, 0, footer),
		)
		classification := Classify(screen, "opencode")
		if want := []string{"Allow once", "Allow always", "Reject"}; !reflect.DeepEqual(classification.Options, want) {
			t.Errorf("footer %q: options = %q", footer, classification.Options)
		}
	}
}

func TestOpenCodeUnknownButtonsAreNotAnApproval(t *testing.T) {
	screen := ocScreen(
		ocRow("△ Something new"),
		ocBlank(),
		ocButtons([]string{"Yes", "No"}, 0, ocHorizontalFooter),
	)
	if Classify(screen, "opencode").Kind == AttentionApproval {
		t.Fatal("unknown button set produced approval keys")
	}
}

func TestOpenCodeShellPermissionPreviewsCommand(t *testing.T) {
	screen := ocScreen(
		ocRow("△ Permission required"),
		ocRow("  # List release artifacts"),
		ocBlank(),
		ocRow("$ ls -la dist/release"),
		ocBlank(),
		ocButtons([]string{"Allow once", "Allow always", "Reject"}, 0, ocHorizontalFooter),
	)
	classification := Classify(screen, "opencode")
	if classification.Command != "ls -la dist/release" {
		t.Fatalf("command = %q", classification.Command)
	}
	if !strings.Contains(classification.Prompt, "List release artifacts") {
		t.Fatalf("prompt = %q", classification.Prompt)
	}
}

// A question tool call with `questions: []` opens OpenCode's question dialog
// directly on its review step, with nothing to review.
func emptyQuestionScreen() string {
	return ocScreen(
		ocRow(" Confirm "),
		ocBlank(),
		ocRow("Review"),
		ocBlank(),
		ocRow("⇆ tab  enter submit  esc dismiss"),
	)
}

func TestOpenCodeEmptyQuestionIsApproval(t *testing.T) {
	classification := Classify(emptyQuestionScreen(), "opencode")
	if classification.Kind != AttentionApproval ||
		!reflect.DeepEqual(classification.Options, []string{"Submit", "Dismiss"}) ||
		classification.ApprovalNavigation != ApprovalNavigationConfirm {
		t.Fatalf("classification = %+v", classification)
	}
	if got := ApprovalKeys(classification, 0); !reflect.DeepEqual(got, []string{"Enter"}) {
		t.Fatalf("submit keys = %q", got)
	}
	if got := ApprovalKeys(classification, 1); !reflect.DeepEqual(got, []string{"Escape"}) {
		t.Fatalf("dismiss keys = %q", got)
	}
}

func TestOpenCodeReviewWithAnswersIsNotEmptyQuestion(t *testing.T) {
	screen := ocScreen(
		ocRow(" Scope   Confirm "),
		ocBlank(),
		ocRow("Review"),
		ocRow("Scope: Everything"),
		ocBlank(),
		ocRow("⇆ tab  enter submit  esc dismiss"),
	)
	if c := Classify(screen, "opencode"); c.Kind == AttentionApproval {
		t.Fatalf("review with answers became an empty-question approval: %+v", c)
	}
}

func TestOpenCodeWrappedQuestionKeepsEveryRow(t *testing.T) {
	fixture := attentionFixture(t, "opencode-single-question.ansi")
	lines := strings.Split(fixture, "\n")
	inserted := false
	for index, line := range lines {
		if strings.Contains(line, "What would you like to work on today?") {
			first := strings.Replace(line, "What would you like to work on today?", "Before we start: the long context of this question", 1)
			lines = append(lines[:index], append([]string{first}, lines[index:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		t.Fatal("fixture question row not found")
	}
	interaction := Parse(strings.Join(lines, "\n"), "opencode")
	want := "Before we start: the long context of this question What would you like to work on today?"
	if interaction == nil || interaction.Question != want {
		t.Fatalf("interaction = %+v", interaction)
	}
}
