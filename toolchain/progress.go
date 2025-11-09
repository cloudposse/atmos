package toolchain

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

func resetLine(stderr *os.File, isTTY bool) {
	defer perf.Track(nil, "toolchain.resetLine")()

	if isTTY {
		fmt.Fprintf(stderr, "\r\033[K")
	}
}

func printStatusLine(stderr *os.File, isTTY bool, line string) {
	defer perf.Track(nil, "toolchain.printStatusLine")()

	resetLine(stderr, isTTY)
	fmt.Fprintln(stderr, line)
}

func printProgressBar(stderr *os.File, isTTY bool, line string) {
	defer perf.Track(nil, "toolchain.printProgressBar")()

	if isTTY {
		fmt.Fprintf(stderr, "\r\033[K%s", line)
	} else {
		fmt.Fprintln(stderr, line)
	}
}
