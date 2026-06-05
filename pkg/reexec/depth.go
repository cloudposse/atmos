package reexec

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DepthEnvVar holds the re-exec depth counter. A value of 0 (or unset) means
// the process was not re-exec'd by another Atmos process. A value of 1 or
// higher means the current process is running inside a re-exec'd child; any
// code path that would normally trigger another re-exec must short-circuit
// instead of looping.
const DepthEnvVar = "ATMOS_REEXEC_DEPTH"

// Depth parses a DepthEnvVar value. Empty or non-numeric input returns 0 so
// callers can treat "missing" and "invalid" identically (both mean "not
// inside a re-exec").
func Depth(value string) int {
	defer perf.Track(nil, "reexec.Depth")()

	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// CurrentDepth reads the current depth from the process environment. It is
// shorthand for Depth(os.Getenv(DepthEnvVar)) and is the preferred entry
// point for callers that do not use dependency-injected env lookup.
func CurrentDepth() int {
	defer perf.Track(nil, "reexec.CurrentDepth")()

	return Depth(os.Getenv(DepthEnvVar)) //nolint:forbidigo // Loop-guard sentinel, not user-facing config.
}

// NextEnv returns a copy of environ with ATMOS_REEXEC_DEPTH incremented by
// one. If the input contains no depth entry, the returned slice has one
// appended set to "1". If it contains multiple, the last wins and the
// returned slice contains exactly one. Use this when building the env slice
// for a child process about to be re-exec'd.
func NextEnv(environ []string) []string {
	defer perf.Track(nil, "reexec.NextEnv")()

	current := ""
	filtered := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if strings.HasPrefix(kv, DepthEnvVar+"=") {
			current = strings.TrimPrefix(kv, DepthEnvVar+"=")
			continue
		}
		filtered = append(filtered, kv)
	}
	next := Depth(current) + 1
	return append(filtered, fmt.Sprintf("%s=%d", DepthEnvVar, next))
}
