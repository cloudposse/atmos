package ansi

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// TrimRight removes trailing whitespace from an ANSI-coded string while
// preserving trailing spaces that are part of styled content (with background color).
// This distinguishes between:
//   - Styled content spaces: have background color (48;2;...) - PRESERVED
//   - Glamour padding spaces: only foreground color (38;2;...) - TRIMMED
//
// This preserves intentional suffix spaces (e.g., H1 header badges " About ") while
// removing Glamour's line-padding spaces that have no background styling.
func TrimRight(s string) string {
	defer perf.Track(nil, "ansi.TrimRight")()

	if s == "" {
		return s
	}

	// First, trim any plain spaces/tabs after the last ANSI code.
	result := trimPlainTrailing(s)

	// Then, iteratively trim trailing padding patterns.
	// Glamour wraps each padding space in ANSI codes like: \x1b[38;2;...m \x1b[0m
	// After trimming one, we may be left with bare ANSI codes that also need removing.
	for {
		// Try to trim consecutive reset codes (keep at most one).
		trimmed := trimConsecutiveResets(result)
		if trimmed != result {
			result = trimmed
			continue
		}

		// Try to trim a styled space (space + reset).
		trimmed = trimTrailingStyledSpace(result)
		if trimmed != result {
			result = trimmed
			continue
		}

		// Try to trim a trailing bare ANSI code (no content after it).
		trimmed = trimTrailingBareANSI(result)
		if trimmed != result {
			result = trimmed
			continue
		}

		break // Nothing more to trim.
	}

	return result
}

// TrimLinesRight trims trailing whitespace from each line in a multi-line string.
// This is useful after lipgloss.Render() which pads all lines to the same width.
// Uses ANSI-aware TrimRight to handle whitespace wrapped in ANSI codes.
//
// Background: Glamour pads content to ensure background colors render properly
// (see https://github.com/charmbracelet/glamour/issues/235). This padding is
// necessary for styled blocks but creates excessive trailing whitespace. We trim
// this padding while preserving intentional suffix spaces that are part of the
// styled content (e.g., H1 header badges like " About Atmos ").
func TrimLinesRight(s string) string {
	defer perf.Track(nil, "ansi.TrimLinesRight")()

	lines := strings.Split(s, newline)
	for i, line := range lines {
		lines[i] = TrimRight(line)
	}
	return strings.Join(lines, newline)
}

// trimConsecutiveResets removes consecutive reset codes at the end, keeping at most one.
// This handles Glamour output like "...\x1b[0m\x1b[0m" -> "...\x1b[0m".
func trimConsecutiveResets(s string) string {
	const resetCode = "\033[0m"
	const doubleReset = resetCode + resetCode

	if strings.HasSuffix(s, doubleReset) {
		return s[:len(s)-len(resetCode)]
	}
	return s
}

// trimPlainTrailing removes plain spaces/tabs after the last ANSI code.
func trimPlainTrailing(s string) string {
	lastEnd := findLastEnd(s)

	if lastEnd == -1 {
		return strings.TrimRight(s, " \t")
	}

	content := s[:lastEnd]
	padding := s[lastEnd:]
	return content + strings.TrimRight(padding, " \t")
}

// trimTrailingStyledSpace removes one trailing ANSI-wrapped space if it has no background color.
// It matches ESC[...m SPACE ESC[0m where the ANSI params have no "48;" (no background color).
// Returns the original string if no such space exists at the end.
func trimTrailingStyledSpace(s string) string {
	const resetCode = "\033[0m"
	if !strings.HasSuffix(s, resetCode) {
		return s
	}

	beforeReset := s[:len(s)-len(resetCode)]
	if len(beforeReset) == 0 {
		return s
	}

	// Check if it ends with a space.
	if beforeReset[len(beforeReset)-1] != ' ' {
		return s
	}

	// Find the ANSI code that styles this space.
	ansiStart := strings.LastIndex(beforeReset[:len(beforeReset)-1], "\033[")
	if ansiStart == -1 {
		return s
	}

	// Extract the ANSI parameters (between \x1b[ and m).
	ansiContent := beforeReset[ansiStart+2 : len(beforeReset)-1]
	mPos := strings.Index(ansiContent, "m")
	if mPos == -1 {
		return s
	}
	ansiParams := ansiContent[:mPos]

	// If it has background color (48;), this is styled content - preserve it.
	if strings.Contains(ansiParams, "48;") {
		return s
	}

	// No background color - this is a padding space. Remove it.
	return beforeReset[:ansiStart]
}

// trimTrailingBareANSI removes a trailing ANSI styling code that has no visible content after it.
// This handles the case where trimming a styled space leaves behind a bare ANSI code.
// Pattern: \x1b[...m at the very end of string (but NOT reset codes \x1b[0m).
// Reset codes are legitimate closers and should be preserved.
func trimTrailingBareANSI(s string) string {
	if len(s) == 0 {
		return s
	}

	// Find the last ANSI sequence.
	lastANSI := strings.LastIndex(s, "\033[")
	if lastANSI == -1 {
		return s
	}

	// Check if the ANSI sequence goes to the end of the string.
	// Find where this ANSI sequence ends (at 'm').
	for i := lastANSI + 2; i < len(s); i++ {
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
			// Found the end of ANSI sequence at position i.
			// If this is at the end of string, check if it's a reset code.
			if i == len(s)-1 {
				// Don't trim reset codes (\x1b[0m) - they're legitimate closers.
				ansiContent := s[lastANSI+2 : i]
				if ansiContent == "0" {
					return s // This is a reset code, preserve it.
				}
				return s[:lastANSI]
			}
			// There's content after this ANSI code - don't trim.
			return s
		}
	}

	// Malformed ANSI (no terminating letter) - leave as is.
	return s
}

// TrimRightSpaces removes only trailing spaces (not tabs) from an ANSI-coded string while
// preserving all ANSI escape sequences on the actual content.
// This is useful for removing Glamour's padding spaces while preserving intentional tabs.
func TrimRightSpaces(s string) string {
	defer perf.Track(nil, "ansi.TrimRightSpaces")()

	stripped := Strip(s)
	trimmed := strings.TrimRight(stripped, " ")

	if trimmed == stripped {
		return s
	}
	if trimmed == "" {
		return ""
	}

	result, i := copyContentAndANSI(s, len(trimmed))

	// Capture any trailing ANSI codes that immediately follow the last character.
	for i < len(s) && isStart(s, i) {
		start := i
		i = skip(s, i)
		result.WriteString(s[start:i])
	}

	return result.String()
}

// TrimLeftSpaces removes only leading spaces from an ANSI-coded string while
// preserving all ANSI escape sequences on the remaining content.
// This is useful for removing Glamour's paragraph indent while preserving styled content.
func TrimLeftSpaces(s string) string {
	defer perf.Track(nil, "ansi.TrimLeftSpaces")()

	stripped := Strip(s)
	trimmed := strings.TrimLeft(stripped, " ")

	if trimmed == stripped {
		return s // No leading spaces to remove.
	}
	if trimmed == "" {
		return "" // All spaces.
	}

	// Calculate how many leading spaces to skip.
	leadingSpaces := len(stripped) - len(trimmed)

	// Walk through original string, skipping ANSI codes and counting spaces.
	spacesSkipped := 0
	i := 0

	// Skip leading ANSI codes and spaces until we've skipped the required amount.
skipLoop:
	for i < len(s) && spacesSkipped < leadingSpaces {
		switch {
		case isStart(s, i):
			// Skip ANSI sequence (don't output it since it's styling skipped content).
			i = skip(s, i)
		case s[i] == ' ':
			spacesSkipped++
			i++
		default:
			break skipLoop // Non-space content found.
		}
	}

	// Return remaining content (including any ANSI codes).
	return s[i:]
}

// TrimTrailingWhitespace splits rendered markdown by newlines and trims trailing spaces
// that Glamour adds for padding (including ANSI-wrapped spaces). For empty lines (all whitespace),
// it preserves the leading indent (first 2 spaces) to maintain paragraph structure.
//
// Note: Glamour's padding ensures background colors render properly across the full width
// (see https://github.com/charmbracelet/glamour/issues/235). We remove this padding for cleaner
// output while TrimRight preserves intentional suffix spaces in styled content.
func TrimTrailingWhitespace(rendered string, paragraphIndent string, paragraphIndentWidth int) []string {
	defer perf.Track(nil, "ansi.TrimTrailingWhitespace")()

	lines := strings.Split(rendered, newline)
	for i := range lines {
		// Use TrimRightSpaces to remove trailing spaces while preserving tabs.
		line := TrimRightSpaces(lines[i])

		// If line became empty after trimming but had content before,
		// it was an empty line with indent - preserve the indent.
		if line == "" && len(lines[i]) > 0 {
			// Preserve up to 2 leading spaces for paragraph indent.
			if len(lines[i]) >= paragraphIndentWidth {
				lines[i] = paragraphIndent
			} else {
				lines[i] = lines[i][:len(lines[i])] // Keep whatever spaces there were.
			}
		} else {
			lines[i] = line
		}
	}
	return lines
}
