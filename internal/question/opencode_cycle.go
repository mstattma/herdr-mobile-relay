package question

import (
	"regexp"
	"strings"
)

var (
	openCodeCycleBorder      = regexp.MustCompile(`^ *\x{2579}\x{2580}{16,}`)
	openCodeCycleModel       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 _-]{0,79} \x{00b7} \S[^\r\n]*$`)
	openCodeCyclePlaceholder = regexp.MustCompile(`^Ask anything(?:\.{3}|\x{2026}) "[^"\r\n]+"$`)
	openCodeCycleHint        = regexp.MustCompile(`^(?:(?:ctrl\+[a-z] variants|[/~][^\r\n]*|[A-Za-z]:[\\/][^\r\n]*)\s{2,})?(?:tab agents|[0-9][0-9.,]*[kKmMbBtT]?(?: \([0-9]+%\))?(?: \x{00b7} \$[0-9.,]+)?)\s+ctrl\+p commands$`)
	openCodeCycleHomeFooter  = regexp.MustCompile(`^(?:(?:[/~]|[A-Za-z]:[\\/])[^\r\n]*\s{2,})?(?:\x{2299} \d+ MCP /status\s{2,})?\d+\.\d+\.\d+(?:[-+][A-Za-z0-9._-]+)?$`)
	// Native modal headers place an isolated esc at the right of their title.
	// Permission/question panels have paired action hints, not just these words
	// in prose. Both can be visible above an otherwise intact composer.
	openCodeCycleModal          = regexp.MustCompile(`(?i)\S[^\r\n]*[ \t]{2,}esc(?:/enter)?(?:[ \t]{2,}|[ \t]*$)`)
	openCodeCycleDialogControls = regexp.MustCompile(`(?i)(?:\benter (?:confirm|submit|toggle)\b[^\r\n]*\besc (?:cancel|dismiss)\b|\x{21c6} select[^\r\n]*\benter confirm\b)`)
)

// OpenCodeCyclePrompt recognizes the stock 1.18.29 empty composer, not merely
// an agent name or placeholder in history. prompt/index.tsx renders three input
// rows (padding, textarea, padding), metadata, the bottom border, then hints or
// session usage. Tab is the default agent-cycle key; rebindings are unsupported.
func OpenCodeCyclePrompt(text string) bool {
	if len(text) > 128*1024 {
		return false
	}
	lines := strings.Split(ansiPattern.ReplaceAllString(strings.ReplaceAll(text, "\r", ""), ""), "\n")
	if len(lines) > 256 {
		return false
	}
	bottom, left, right := -1, 0, 0
	for index := len(lines) - 1; index >= 0; index-- {
		if border := openCodeCycleBorder.FindString(lines[index]); border != "" {
			bottom = index
			left = strings.Index(border, "\u2579")
			right = len([]rune(border))
			break
		}
	}
	if bottom < 5 || bottom+1 >= len(lines) || right > 4096 {
		return false
	}
	// The border fixes the main column's extent even with a right sidebar.
	// Preserve indentation; TrimSpace/cleanLines would destroy this evidence.
	main := func(index int) string {
		row := []rune(lines[index])
		return string(row[:min(right, len(row))])
	}
	start := bottom - 4
	if strings.TrimSpace(main(start-1)) != "" {
		return false // clipped textarea or an autocomplete menu touching the input
	}
	input := ""
	for index := start; index < bottom; index++ {
		row := []rune(main(index))
		if len(row) <= left || strings.TrimSpace(string(row[:left])) != "" || row[left] != '\u2503' {
			return false
		}
		value := strings.TrimSpace(string(row[left+1:]))
		switch index {
		case start, start + 2:
			if value != "" {
				return false
			}
		case start + 1:
			input = value
			if value != "" && !openCodeCyclePlaceholder.MatchString(value) {
				return false
			}
		case bottom - 1:
			if !openCodeCycleModel.MatchString(value) {
				return false
			}
		}
	}
	footer := strings.TrimSpace(main(bottom + 1))
	if !openCodeCycleHint.MatchString(footer) || input != "" && !strings.Contains(footer, "tab agents") {
		return false
	}
	home := input != ""
	tip, homeFooter := false, false
	for index := bottom + 2; index < len(lines); index++ {
		line := strings.TrimSpace(main(index))
		if home {
			// Home centers the composer and has a separate full-width footer,
			// optional one-line tip, and flexible blank space below it.
			line = strings.TrimSpace(lines[index])
		}
		if line == "" {
			continue
		}
		switch {
		case home && !homeFooter && !tip && index >= bottom+5 && strings.HasPrefix(line, "\u25cf Tip "):
			tip = true
		case home && !homeFooter && openCodeCycleHomeFooter.MatchString(line):
			homeFooter = true
		default:
			return false
		}
	}
	if home && !homeFooter {
		return false
	}
	for _, line := range lines {
		if openCodeCycleModal.MatchString(line) || openCodeCycleDialogControls.MatchString(line) {
			return false
		}
	}
	return true
}
