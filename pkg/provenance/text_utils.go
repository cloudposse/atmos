package provenance

import (
	"strings"
)

// wrapState holds state for line wrapping.
type wrapState struct {
	inEscape     *bool
	currentLine  *strings.Builder
	currentPlain *strings.Builder
	currentWidth *int
	maxWidth     int
	wrapped      *[]string
}

// processRune processes a single rune for line wrapping.
func processRune(r rune, state *wrapState) bool {
	// Handle ANSI escape sequences
	if r == '\x1b' {
		*state.inEscape = true
		state.currentLine.WriteRune(r)
		return false
	}

	if *state.inEscape {
		state.currentLine.WriteRune(r)
		if r == 'm' {
			*state.inEscape = false
		}
		return false
	}

	// Regular character - check if we need to wrap
	if *state.currentWidth >= state.maxWidth && (r == ' ' || r == '\t') {
		*state.wrapped = append(*state.wrapped, state.currentLine.String())
		state.currentLine.Reset()
		state.currentPlain.Reset()
		*state.currentWidth = 0
		return true
	}

	state.currentLine.WriteRune(r)
	state.currentPlain.WriteRune(r)
	*state.currentWidth++
	return false
}

// wrapLine wraps a line to fit within maxWidth, preserving ANSI codes.
func wrapLine(line string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{line}
	}

	plainText := stripANSI(line)
	if len(plainText) <= maxWidth {
		return []string{line}
	}

	var wrapped []string
	var currentLine strings.Builder
	var currentPlain strings.Builder
	currentWidth := 0
	inEscape := false

	runes := []rune(line)
	state := &wrapState{
		inEscape:     &inEscape,
		currentLine:  &currentLine,
		currentPlain: &currentPlain,
		currentWidth: &currentWidth,
		maxWidth:     maxWidth,
		wrapped:      &wrapped,
	}
	for i := 0; i < len(runes); i++ {
		processRune(runes[i], state)
	}

	// Add remaining content
	if currentLine.Len() > 0 {
		wrapped = append(wrapped, currentLine.String())
	}

	// If we couldn't wrap nicely, just hard-wrap at maxWidth
	if len(wrapped) == 0 && len(plainText) > maxWidth {
		wrapped = append(wrapped, line[:maxWidth])
		if len(line) > maxWidth {
			wrapped = append(wrapped, wrapLine(line[maxWidth:], maxWidth)...)
		}
	}

	return wrapped
}

// combineSideBySide combines left and right text into side-by-side layout.
func combineSideBySide(left, right string, leftWidth int) string {
	// Wrap left lines to fit within leftWidth
	var wrappedLeftLines []string
	for _, line := range strings.Split(left, newlineChar) {
		wrapped := wrapLine(line, leftWidth-2) // Reserve 2 chars for padding
		wrappedLeftLines = append(wrappedLeftLines, wrapped...)
	}

	rightLines := strings.Split(right, newlineChar)

	// Balance the lines by inserting blanks where needed
	balancedLeft, balancedRight := balanceColumns(wrappedLeftLines, rightLines)

	var buf strings.Builder

	// Header
	buf.WriteString("Configuration")
	buf.WriteString(strings.Repeat(pathSpace, leftWidth-13))
	buf.WriteString(" │  Provenance\n")
	buf.WriteString(strings.Repeat("─", leftWidth))
	buf.WriteString("┼")
	buf.WriteString(strings.Repeat("─", defaultSeparatorWidth))
	buf.WriteString(newlineChar)

	// Combine lines
	maxLines := max(len(balancedLeft), len(balancedRight))
	for i := 0; i < maxLines; i++ {
		// Left side
		leftLine := ""
		if i < len(balancedLeft) {
			leftLine = balancedLeft[i]
		}
		buf.WriteString(leftLine)

		// Pad to left width (accounting for ANSI color codes)
		padding := leftWidth - len(stripANSI(leftLine))
		if padding > 0 {
			buf.WriteString(strings.Repeat(pathSpace, padding))
		}

		// Separator
		buf.WriteString(" │  ")

		// Right side
		if i < len(balancedRight) {
			buf.WriteString(balancedRight[i])
		}

		buf.WriteString(newlineChar)
	}

	return buf.String()
}

// balanceColumns aligns left and right columns by inserting blank lines.
func balanceColumns(leftLines, rightLines []string) ([]string, []string) {
	// Build aligned output
	var balancedLeft, balancedRight []string
	leftIdx, rightIdx := 0, 0

	for leftIdx < len(leftLines) || rightIdx < len(rightLines) {
		// If both have content, check if they should align
		switch {
		case leftIdx < len(leftLines) && rightIdx < len(rightLines):
			// Both sides have content - add both
			balancedLeft = append(balancedLeft, leftLines[leftIdx])
			balancedRight = append(balancedRight, rightLines[rightIdx])
			leftIdx++
			rightIdx++
		case leftIdx < len(leftLines):
			// Only left has content - add blank to right
			balancedLeft = append(balancedLeft, leftLines[leftIdx])
			balancedRight = append(balancedRight, "")
			leftIdx++
		default:
			// Only right has content - add blank to left
			balancedLeft = append(balancedLeft, "")
			balancedRight = append(balancedRight, rightLines[rightIdx])
			rightIdx++
		}
	}

	return balancedLeft, balancedRight
}

// stripANSI removes ANSI escape codes from a string for length calculation.
func stripANSI(s string) string {
	// Simple ANSI stripping - removes escape sequences
	result := ""
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}
	return result
}

// max returns the maximum of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
