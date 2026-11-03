package question

import (
	"fmt"
	"strings"
	"testing"
)

func openCodeCycleFixture(input, footer string, sidebar bool) string {
	rows := []string{
		"Historical response about questions and permissions.",
		"",
		"  \u2503",
		"  \u2503  " + input,
		"  \u2503",
		"  \u2503  Plan \u00b7 Example Model ExampleProvider \u00b7 high",
		"  \u2579" + strings.Repeat("\u2580", 96),
		"   " + footer,
		"",
	}
	if sidebar {
		labels := []string{"Example session", "Context", "12,000 tokens", "Todo", "Review question handling", "/work/example", "", "\u2022 OpenCode 1.18.29", ""}
		for index := range rows {
			rows[index] = fmt.Sprintf("%-99s  %s", rows[index], labels[index])
		}
	}
	return strings.Join(rows, "\n")
}

func TestOpenCodeCyclePrompt(t *testing.T) {
	const footer = "/work/example  12.0K (4%)  ctrl+p commands"
	session := openCodeCycleFixture("", footer, false)
	sidebar := openCodeCycleFixture("", footer, true)
	ansi := "\x1b[38;2;224;108;117m\x1b[48;2;30;30;30m" + strings.ReplaceAll(sidebar, "\n", "\x1b[0m\n\x1b[38;2;224;108;117m\x1b[48;2;30;30;30m") + "\x1b[0m"
	home := func(placeholder string) string {
		rows := strings.Split(openCodeCycleFixture(placeholder, "tab agents  ctrl+p commands", false), "\n")
		for index := range rows {
			rows[index] = strings.Repeat(" ", 40) + rows[index]
		}
		return strings.Join(rows, "\n") + "\n\n\n   \u25cf Tip Press tab to cycle agents" + strings.Repeat("\n", 12) + "  /work/example  \u2299 2 MCP /status       1.18.29\n"
	}
	for _, test := range []struct {
		name string
		text string
		want bool
	}{
		{"home centered with tip space and footer", home("Ask anything\u2026 \"Fix broken tests\""), true},
		{"home ascii placeholder", home("Ask anything... \"Fix broken tests\""), true},
		{"home unknown trailing control", home("Ask anything\u2026 \"Fix broken tests\"") + "New control", false},
		{"home without native footer", openCodeCycleFixture("Ask anything\u2026 \"Fix broken tests\"", "tab agents  ctrl+p commands", false), false},
		{"session without placeholder or tab hint", session, true},
		{"session sidebar", sidebar, true},
		{"session ansi same panel colors as permission", ansi, true},
		{"session cost", openCodeCycleFixture("", "/work/example  12.0K (4%) \u00b7 $1.23  ctrl+p commands", true), true},
		{"historical questions", "# Questions\nWhich option?\n1. First\n2. Second\nAsked 2 questions\nRejected permission earlier.\n" + session, true},
		{"permission replaces composer", "\n\u2503 \u25b3 Permission required\n\u2503 Allow once  Allow always  Reject\n\u2503 \u21c6 select  enter confirm", false},
		{"permission same ansi panel colors", "\x1b[38;2;224;108;117m\x1b[48;2;30;30;30m\n\u2503 \u25b3 Permission required\n\u2503 Allow once  Allow always  Reject\n\u2503 \u21c6 select  enter confirm\x1b[0m", false},
		{"permission controls overlay", "\u2503 \u25b3 Permission required\n\u2503 Allow once  Allow always  Reject\n\u2503 \u21c6 select  enter confirm\n" + session, false},
		{"question replaces composer", "\n\u2503 Which option?\n\u2503 1. First\n\u2503 2. Second\n\u2503 \u2191\u2193 select  enter submit  esc dismiss", false},
		{"question controls overlay", "Which option?\n1. First\n2. Second\n\u2191\u2193 select  enter submit  esc dismiss\n" + session, false},
		{"rejection textarea", "\u2503 \u25b3 Reject permission\n\u2503 Tell OpenCode what to do differently\n\u2503\n\u2503 enter confirm  esc cancel\n" + session, false},
		{"picker overlay", "Select model                      esc\nSearch\nExample Model\n" + sidebar, false},
		{"other modal above composer", "Rename session                    esc\nEnter text\n" + session, false},
		{"help modal above composer", "Help                    esc/enter\nPress ctrl+p to see all available actions\n  ok\n" + session, false},
		{"native draft", openCodeCycleFixture("Review the example changes", footer, true), false},
		{"native placeholder-like draft in session", openCodeCycleFixture("Ask anything... \"Fix broken tests\"", footer, true), false},
		{"native multiline draft", strings.Replace(session, "\n\n  \u2503", "\n\n  \u2503 Draft above the visible empty row\n  \u2503", 1), false},
		{"autocomplete slash", openCodeCycleFixture("/new", footer, true), false},
		{"autocomplete mention", openCodeCycleFixture("@example", footer, false), false},
		{"autocomplete touching composer", strings.Replace(session, "\n\n  \u2503", "\n\u2503 /new  Start a new session\n  \u2503", 1), false},
		{"unknown overlay", session + "\nUnknown popup", false},
		{"no command hint", openCodeCycleFixture("", "/work/example  12.0K (4%)", false), false},
		{"unknown footer content", openCodeCycleFixture("", "Unknown control  12.0K (4%)  ctrl+p commands", false), false},
		{"working footer", openCodeCycleFixture("", "[busy] esc interrupt  12.0K (4%)  ctrl+p commands", false), true},
		{"shell mode", strings.Replace(session, "Plan \u00b7 Example Model ExampleProvider \u00b7 high", "Shell", 1), false},
		{"missing input row", strings.Replace(session, "\n  \u2503  \n", "\n", 1), false},
		{"missing top separator", strings.Replace(session, "\n\n  \u2503", "\n  \u2503", 1), false},
		{"no bottom border", strings.ReplaceAll(session, "\u2580", "-"), false},
		{"unframed log text", "Ask anything...\nBuild Example Model\ntab agents", false},
		{"unknown", "$ ", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := OpenCodeCyclePrompt(test.text); got != test.want {
				t.Fatalf("eligible = %v, want %v", got, test.want)
			}
		})
	}
}
