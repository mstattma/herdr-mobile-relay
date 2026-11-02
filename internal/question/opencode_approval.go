package question

import (
	"regexp"
	"strings"
)

// OpenCode renders permission requests (including those raised by subagent
// sessions) as a bordered dialog that replaces the prompt input:
//
//	┃  △ Permission required
//	┃    ← Access external directory ~/.local/state/example
//	┃    For session: Explore (@explore subagent) (ses_…)
//	┃
//	┃  Patterns
//	┃  - /root/.local/state/example/*
//	┃
//	┃   Allow once   Allow always   Reject      ctrl+f fullscreen  ⇆ select  enter confirm
//
// The options are a horizontal button row navigated with Left/Right, and the
// focused button is the one drawn with a distinct background. Choosing Allow
// always opens a second "Always allow" Confirm/Cancel dialog of the same shape;
// rejecting a subagent request opens "Reject permission", which takes an
// optional message and is confirmed with Enter or dismissed with Escape.

// Approval navigation styles. The empty style is the vertical numbered menu
// used by the other agents (Up/Down, then Enter).
const (
	ApprovalNavigationHorizontal = "horizontal"
	ApprovalNavigationConfirm    = "confirm"
)

const (
	openCodeDialogMaxLines  = 60
	openCodeDialogMaxPrompt = 2000
	openCodeSidebarGap      = 20
)

var (
	openCodeSGRPattern       = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)
	openCodeButtonGapPattern = regexp.MustCompile(`\s{2,}`)
	openCodeFooterStart      = regexp.MustCompile(`(?i)\s(?:ctrl\+f\s|⇆\s|enter\s+confirm)`)
)

type openCodeApproval struct {
	prompt     string
	command    string
	options    []string
	focus      int
	navigation string
	source     string
}

// openCodeDialogLine strips ANSI styling, the left border, and the right-hand
// sidebar column. Lines without the border, and rows whose content starts in
// the sidebar column, are empty.
func openCodeDialogLine(raw string) (text string, full string) {
	plain, backgrounds := openCodeStyledRunes(strings.ReplaceAll(raw, "\r", ""))
	border := -1
	for index, r := range plain {
		if r == '┃' {
			border = index
			break
		}
		if r != ' ' && r != '\t' {
			return "", ""
		}
	}
	if border < 0 {
		return "", ""
	}
	// The sidebar column is separated from the dialog by cells drawn with the
	// same background as the margin left of the border.
	end := len(plain)
	if margin := backgrounds[0]; border > 0 && margin != "" {
		for index := border + 1; index < len(plain); index++ {
			if backgrounds[index] == margin {
				end = index
				break
			}
		}
	}
	trimmed := strings.TrimRight(string(plain[border+1:end]), " \t")
	content := strings.TrimLeft(trimmed, " \t")
	if len(trimmed)-len(content) >= openCodeSidebarGap {
		return "", ""
	}
	full = strings.TrimSpace(content)
	if gap := openCodeColumnPattern.FindStringIndex(content); gap != nil {
		content = content[:gap[0]]
	}
	return strings.TrimSpace(content), full
}

func openCodeHorizontalFooter(full string) bool {
	lower := strings.ToLower(full)
	return strings.Contains(lower, "⇆ select") && strings.Contains(lower, "enter confirm")
}

func openCodeRejectFooter(full string) bool {
	lower := strings.ToLower(full)
	return strings.Contains(lower, "enter confirm") && strings.Contains(lower, "esc cancel") &&
		!strings.Contains(lower, "⇆")
}

// parseOpenCodeApproval recognizes a live OpenCode permission dialog at the
// bottom of the pane. It returns nil unless the dialog, its options, and the
// focused option are all unambiguous, so the caller falls back to "needs
// inspection" instead of sending keys to an unknown cursor position.
func parseOpenCodeApproval(text string) *openCodeApproval {
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := make([]string, len(rawLines))
	fulls := make([]string, len(rawLines))
	footer := -1
	for index, raw := range rawLines {
		lines[index], fulls[index] = openCodeDialogLine(raw)
		if fulls[index] != "" {
			footer = index
		}
	}
	if footer < 0 {
		return nil
	}
	switch {
	case openCodeHorizontalFooter(fulls[footer]):
		return parseOpenCodeButtonDialog(rawLines, lines, fulls, footer)
	case openCodeRejectFooter(fulls[footer]):
		return parseOpenCodeRejectDialog(lines, footer)
	case openCodeEmptyQuestionFooter(fulls[footer]):
		return parseOpenCodeEmptyQuestion(lines, footer)
	default:
		return nil
	}
}

func parseOpenCodeButtonDialog(rawLines, lines, fulls []string, footer int) *openCodeApproval {
	row := footer
	rowText := fulls[footer]
	if location := openCodeFooterStart.FindStringIndex(" " + rowText); location != nil {
		rowText = strings.TrimSpace(rowText[:max(location[0]-1, 0)])
	}
	if rowText == "" {
		// Narrow layout: the footer hints sit on their own line below the buttons.
		row = -1
		for index := footer - 1; index >= 0 && index >= footer-3; index-- {
			if lines[index] != "" {
				row, rowText = index, lines[index]
				break
			}
		}
		if row < 0 {
			return nil
		}
	}
	tokens := make([]string, 0, 6)
	for _, label := range openCodeButtonGapPattern.Split(rowText, -1) {
		if label = strings.TrimSpace(label); label != "" {
			tokens = append(tokens, label)
		}
	}
	options := openCodeButtonSet(tokens)
	if options == nil {
		return nil
	}
	focus, ok := openCodeFocusedButton(rawLines[row], options)
	if !ok {
		return nil
	}
	start := openCodeDialogTitle(lines, row)
	if start < 0 {
		return nil
	}
	prompt, command := openCodeDialogPrompt(lines[start:row])
	if prompt == "" {
		return nil
	}
	return &openCodeApproval{
		prompt:     prompt,
		command:    command,
		options:    options,
		focus:      focus,
		navigation: ApprovalNavigationHorizontal,
		source:     prompt,
	}
}

func parseOpenCodeRejectDialog(lines []string, footer int) *openCodeApproval {
	start := openCodeDialogTitle(lines, footer)
	if start < 0 || !strings.Contains(strings.ToLower(lines[start]), "reject permission") {
		return nil
	}
	prompt, _ := openCodeDialogPrompt(lines[start:footer])
	if prompt == "" {
		return nil
	}
	return &openCodeApproval{
		prompt:     prompt,
		command:    "Reject permission",
		options:    []string{"Reject", "Back"},
		navigation: ApprovalNavigationConfirm,
		source:     prompt,
	}
}

// OpenCode's permission prompts use fixed English button labels
// (packages/tui/src/routes/session/permission.tsx). Accepting only these sets
// keeps footer hints, whatever keybinding they show, from becoming options:
// Left/Right wrap around, so a phantom option would select a real one.
var openCodeButtonSets = [][]string{
	{"Allow once", "Allow always", "Reject"},
	{"Confirm", "Cancel"},
}

func openCodeButtonSet(tokens []string) []string {
	for _, set := range openCodeButtonSets {
		if len(tokens) < len(set) {
			continue
		}
		matches := true
		for index, label := range set {
			if tokens[index] != label {
				matches = false
				break
			}
		}
		if matches {
			return append([]string(nil), set...)
		}
	}
	return nil
}

func openCodeEmptyQuestionFooter(full string) bool {
	lower := strings.ToLower(full)
	return strings.Contains(lower, "enter submit") && strings.Contains(lower, "esc dismiss") &&
		!strings.Contains(lower, "select") && !strings.Contains(lower, "toggle")
}

// parseOpenCodeEmptyQuestion recognizes a question tool call with an empty
// question list. OpenCode then opens straight on its review step: a lone
// "Confirm" tab, a "Review" heading with no answers below it, and a footer
// where Enter submits (an empty answer) and Escape dismisses the question.
func parseOpenCodeEmptyQuestion(lines []string, footer int) *openCodeApproval {
	previous := func(from int) int {
		for index := from - 1; index >= 0 && index >= footer-8; index-- {
			if lines[index] != "" {
				return index
			}
		}
		return -1
	}
	review := previous(footer)
	if review < 0 || lines[review] != "Review" {
		return nil
	}
	tab := previous(review)
	if tab < 0 || lines[tab] != "Confirm" {
		return nil
	}
	prompt := "The agent opened a question without any questions.\n" +
		"Submit sends an empty answer; Dismiss rejects the question."
	return &openCodeApproval{
		prompt:     prompt,
		command:    "Empty question",
		options:    []string{"Submit", "Dismiss"},
		navigation: ApprovalNavigationConfirm,
		source:     prompt,
	}
}

// openCodeDialogTitle finds the "△ <title>" line that opens the dialog.
func openCodeDialogTitle(lines []string, end int) int {
	for index := end - 1; index >= 0 && index >= end-openCodeDialogMaxLines; index-- {
		if strings.HasPrefix(lines[index], "△") {
			return index
		}
	}
	return -1
}

// openCodeDialogPrompt keeps the dialog text line by line (blank runs collapse
// to one blank line) and picks the request line under the title as the short
// command preview.
func openCodeDialogPrompt(lines []string) (string, string) {
	var kept []string
	command := ""
	shellCommand := false
	blank := false
	for index, line := range lines {
		if index == 0 {
			line = strings.TrimSpace(strings.TrimPrefix(line, "△"))
		}
		if line == "" {
			blank = len(kept) > 0
			continue
		}
		if blank {
			kept = append(kept, "")
			blank = false
		}
		if index > 0 && strings.HasPrefix(line, "$ ") && !shellCommand {
			command, shellCommand = strings.TrimSpace(line[2:]), true
		} else if index > 0 && command == "" {
			command = strings.TrimSpace(strings.TrimLeft(line, "←→⚙#◇✱%◈⟳ "))
		}
		kept = append(kept, line)
	}
	prompt := strings.Join(kept, "\n")
	if runes := []rune(prompt); len(runes) > openCodeDialogMaxPrompt {
		prompt = string(runes[:openCodeDialogMaxPrompt])
	}
	return prompt, compact(command, 240)
}

// openCodeFocusedButton maps each button label to the background it is drawn
// with. The dialog panel background is the one most of the row's blank cells
// use; exactly one button must be drawn on a different background, and that
// button is focused.
func openCodeFocusedButton(raw string, options []string) (int, bool) {
	plain, backgrounds := openCodeStyledRunes(raw)
	border := -1
	for index, r := range plain {
		if r == '┃' {
			border = index
			break
		}
	}
	if border < 0 {
		return 0, false
	}
	backgroundOf := make([]string, len(options))
	from := border + 1
	end := from
	for index, label := range options {
		position := strings.Index(string(plain[from:]), label)
		if position < 0 {
			return 0, false
		}
		runeStart := from + len([]rune(string(plain[from:])[:position]))
		backgroundOf[index] = backgrounds[runeStart]
		from = runeStart + len([]rune(label))
		end = from
	}
	blankCounts := map[string]int{}
	for index := border + 1; index < len(plain) && index < end+8; index++ {
		if plain[index] == ' ' {
			blankCounts[backgrounds[index]]++
		}
	}
	panel, best := "", 0
	for background, count := range blankCounts {
		if count > best {
			panel, best = background, count
		}
	}
	if best == 0 {
		return 0, false
	}
	focus := -1
	for index, background := range backgroundOf {
		if background != panel {
			if focus >= 0 {
				return 0, false
			}
			focus = index
		}
	}
	if focus < 0 {
		return 0, false
	}
	return focus, true
}

func openCodeStyledRunes(raw string) ([]rune, []string) {
	var plain []rune
	var backgrounds []string
	background := ""
	rest := raw
	for {
		location := openCodeSGRPattern.FindStringSubmatchIndex(rest)
		segment := rest
		if location != nil {
			segment = rest[:location[0]]
		}
		for _, r := range segment {
			plain = append(plain, r)
			backgrounds = append(backgrounds, background)
		}
		if location == nil {
			break
		}
		background = openCodeBackground(rest[location[2]:location[3]], background)
		rest = rest[location[1]:]
	}
	return plain, backgrounds
}

func openCodeBackground(params, current string) string {
	fields := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	if len(fields) == 0 {
		return ""
	}
	for index := 0; index < len(fields); index++ {
		switch field := fields[index]; {
		case field == "0" || field == "49":
			current = ""
		case field == "48" && index+1 < len(fields):
			width := 2
			if fields[index+1] == "2" {
				width = 4
			}
			end := min(index+1+width, len(fields))
			current = strings.Join(fields[index:end], ";")
			index = end - 1
		case len(field) == 2 && (field[0] == '4') && field[1] >= '0' && field[1] <= '7',
			len(field) == 3 && strings.HasPrefix(field, "10") && field[2] >= '0' && field[2] <= '7':
			current = field
		case field == "38" && index+1 < len(fields):
			// Skip the foreground color arguments so they are not read as a background.
			if fields[index+1] == "2" {
				index += 4
			} else {
				index += 2
			}
		}
	}
	return current
}

// ApprovalKeys returns the keystrokes that move from the focused option to
// target and confirm it, following the dialog's navigation style.
func ApprovalKeys(classification Classification, target int) []string {
	switch classification.ApprovalNavigation {
	case ApprovalNavigationConfirm:
		if target == 0 {
			return []string{"Enter"}
		}
		return []string{"Escape"}
	case ApprovalNavigationHorizontal:
		return stepKeys(target-classification.ApprovalFocus, "Left", "Right")
	default:
		return stepKeys(target-classification.ApprovalFocus, "Up", "Down")
	}
}

func stepKeys(distance int, backward, forward string) []string {
	key := forward
	if distance < 0 {
		key = backward
		distance = -distance
	}
	keys := make([]string, 0, distance+1)
	for range distance {
		keys = append(keys, key)
	}
	return append(keys, "Enter")
}
