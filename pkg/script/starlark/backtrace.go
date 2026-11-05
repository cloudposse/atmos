package starlark

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// Number of repetitions of a repeated run of frames that are kept before the rest is collapsed.
	backtraceKeepRepeats = 3
	// Longest cycle of frames (1 for direct recursion, 2 for mutual recursion, and so on) that is collapsed.
	backtraceMaxPeriod = 8
)

// collapseBacktrace shortens runaway tracebacks. Within the stack-frame lines that follow the
// "Traceback" header, a block of one to backtraceMaxPeriod consecutive identical lines that repeats
// more than backtraceKeepRepeats times keeps its first backtraceKeepRepeats repetitions, and the
// remainder becomes a single marker line. The error message after the frames is never collapsed,
// so a multi-line fail() message keeps every line. Text without such a run is returned unchanged.
func collapseBacktrace(text string) string {
	lines := strings.Split(text, "\n")
	start, end := frameRange(lines)
	out := make([]string, 0, len(lines))
	out = append(out, lines[:start]...)
	changed := false
	for i := start; i < end; {
		period, repeats := repeatedBlock(lines, i, end)
		if repeats <= backtraceKeepRepeats {
			out = append(out, lines[i])
			i++
			continue
		}
		kept := period * backtraceKeepRepeats
		out = append(out, lines[i:i+kept]...)
		out = append(out, collapseMarker((repeats-backtraceKeepRepeats)*period))
		i += period * repeats
		changed = true
	}
	if !changed {
		return text
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// frameRange returns the half-open range of stack-frame lines: the indented lines directly after the
// "Traceback" header. Without a header the range is empty, so nothing is collapsed.
func frameRange(lines []string) (start, end int) {
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "Traceback") {
		return 0, 0
	}
	end = 1
	for end < len(lines) && strings.HasPrefix(lines[end], " ") {
		end++
	}
	return 1, end
}

// repeatedBlock returns the shortest period whose block starting at lines[start] repeats more than
// backtraceKeepRepeats times back to back within lines[:end], and the number of repetitions.
// Otherwise it returns period 1 with the (small) repeat count of the single line.
func repeatedBlock(lines []string, start, end int) (period, repeats int) {
	for p := 1; p <= backtraceMaxPeriod && start+p <= end; p++ {
		if r := countRepeats(lines[:end], start, p); r > backtraceKeepRepeats {
			return p, r
		}
	}
	return 1, 1
}

// countRepeats counts how many times the block lines[start:start+period] repeats consecutively.
func countRepeats(lines []string, start, period int) int {
	repeats := 1
	for next := start + period; next+period <= len(lines); next += period {
		for k := range period {
			if lines[next+k] != lines[start+k] {
				return repeats
			}
		}
		repeats++
	}
	return repeats
}

// collapseMarker is the line that replaces count omitted frames.
func collapseMarker(count int) string {
	noun := "frames"
	if count == 1 {
		noun = "frame"
	}
	return fmt.Sprintf("  ... (%s more identical %s)", groupThousands(count), noun)
}

// groupThousands formats n with comma thousands separators.
func groupThousands(n int) string {
	digits := strconv.Itoa(n)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}
