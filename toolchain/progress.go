package toolchain

import (
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// isTTY returns true if stderr is a TTY (for progress bar display).
func isTTY() bool {
	return term.IsTTYSupportForStderr()
}

func resetLine() {
	defer perf.Track(nil, "toolchain.resetLine")()

	if isTTY() {
		_ = ui.Write("\r\033[K")
	}
}

func printStatusLine(line string) {
	defer perf.Track(nil, "toolchain.printStatusLine")()

	resetLine()
	_ = ui.Writeln(line)
}

func printProgressBar(line string) {
	defer perf.Track(nil, "toolchain.printProgressBar")()

	// UI layer automatically handles TTY detection and ANSI codes.
	// The isTTY check is kept for progress bar logic (show animation vs static output).
	if isTTY() {
		_ = ui.Write("\r\033[K" + line)
	} else {
		_ = ui.Writeln(line)
	}
}
