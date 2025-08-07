package toolchain

import (
	"fmt"
	"os"
)

func resetLine(stderr *os.File, isTTY bool) {
	if isTTY {
		fmt.Fprintf(stderr, "\r\033[K")
		stderr.Sync()
	}
}

func printStatusLine(stderr *os.File, isTTY bool, line string) {
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
