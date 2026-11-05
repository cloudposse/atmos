package conformance

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// maxDetailValue bounds each expected or actual value quoted in a failure detail.
const maxDetailValue = 2 << 10

// detailError attaches human-readable failure evidence to an error. It never changes the
// error's message or identity, so sentinel matching keeps working through it.
type detailError struct {
	err    error
	detail string
}

//nolint:lintroller // Trivial interface accessor with no independent I/O.
func (e *detailError) Error() string { return e.err.Error() }

//nolint:lintroller // Trivial interface accessor with no independent I/O.
func (e *detailError) Unwrap() error { return e.err }

// withDetail records evidence for a failed check. Every value interpolated into the detail must
// already have passed through clip, which truncates and masks it.
func withDetail(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &detailError{err: err, detail: fmt.Sprintf(format, args...)}
}

// detailOf returns the evidence attached to the outermost detailError in err's tree.
func detailOf(err error) string {
	var marked *detailError
	if errors.As(err, &marked) {
		return marked.detail
	}
	return ""
}

// clip masks secrets and bounds a value without splitting a multibyte rune.
func clip(value string) string {
	value = iolib.MaskString(value)
	if len(value) <= maxDetailValue {
		return value
	}
	cut := maxDetailValue
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return fmt.Sprintf("%s... (truncated, %d more bytes)", value[:cut], len(value)-cut)
}

// commandMismatch names each fixture field that differs from the observed command result.
func commandMismatch(fixture *CommandFixture, stdout, stderr string, exitCode int) string {
	var parts []string
	if stdout != fixture.Stdout {
		parts = append(parts, outputMismatch("stdout", fixture.Stdout, stdout))
	}
	if stderr != fixture.Stderr {
		parts = append(parts, outputMismatch("stderr", fixture.Stderr, stderr))
	}
	if exitCode != fixture.ExitCode {
		parts = append(parts, fmt.Sprintf("exit_code does not match the fixture: expected %d, actual %d", fixture.ExitCode, exitCode))
	}
	return strings.Join(parts, "\n")
}

func outputMismatch(field, expected, actual string) string {
	return fmt.Sprintf("%s does not match the fixture (first difference at byte %d):\n  expected: %q\n  actual:   %q",
		field, firstDifference(expected, actual), clip(expected), clip(actual))
}

// firstDifference returns the zero-based byte offset where two strings diverge.
func firstDifference(a, b string) int {
	limit := min(len(a), len(b))
	for index := range limit {
		if a[index] != b[index] {
			return index
		}
	}
	return limit
}
