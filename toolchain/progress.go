package toolchain

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

func resetLine(stderr *os.File, isTTY bool) {
	defer perf.Track(nil, "toolchain.DownloadWithProgress")()

	if isTTY {
		fmt.Fprintf(stderr, "\r\033[K")
		stderr.Sync()
	}
}

func printStatusLine(stderr *os.File, isTTY bool, line string) {
	defer perf.Track(nil, "toolchain.DownloadWithProgress")()

	resetLine(stderr, isTTY)
	fmt.Fprintln(stderr, line)
	stderr.Sync()
}

func printProgressBar(stderr *os.File, isTTY bool, line string) {
	if isTTY {
		fmt.Fprintf(stderr, "\r\033[K%s", line)
	} else {
		fmt.Fprintln(stderr, line)
	}
	stderr.Sync()
}
