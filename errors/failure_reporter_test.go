package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubExit replaces the process exit and records its code.
func stubExit(t *testing.T) *[]int {
	t.Helper()
	var codes []int
	previous := OsExit
	OsExit = func(code int) { codes = append(codes, code) }
	t.Cleanup(func() { OsExit = previous })
	return &codes
}

func TestCheckErrorPrintAndExitReportsFailures(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantReported bool
		wantExit     []int
	}{
		{name: "no error", err: nil},
		{name: "generic error", err: errors.New("failed"), wantReported: true, wantExit: []int{1}},
		{name: "exit code error", err: ExitCodeError{Code: 3}, wantReported: true, wantExit: []int{3}},
		{name: "successful exit code", err: ExitCodeError{Code: 0}, wantExit: []int{0}},
		{name: "silent exit", err: ExitCodeError{Code: 4, Silent: true}, wantExit: []int{4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exits := stubExit(t)
			var reported []error
			restore := SetFailureReporter(func(err error) { reported = append(reported, err) })
			t.Cleanup(restore)

			CheckErrorPrintAndExit(tc.err, "", "")

			if tc.wantReported {
				require.Len(t, reported, 1)
				assert.Equal(t, tc.err, reported[0])
			} else {
				assert.Empty(t, reported)
			}
			assert.Equal(t, tc.wantExit, append([]int(nil), *exits...))
		})
	}
}

func TestSetFailureReporterRestoresThePreviousReporter(t *testing.T) {
	stubExit(t)
	var first, second int
	restoreFirst := SetFailureReporter(func(error) { first++ })
	t.Cleanup(restoreFirst)
	restoreSecond := SetFailureReporter(func(error) { second++ })

	CheckErrorPrintAndExit(errors.New("one"), "", "")
	restoreSecond()
	CheckErrorPrintAndExit(errors.New("two"), "", "")

	assert.Equal(t, 1, second, "the newer reporter ran only while installed")
	assert.Equal(t, 1, first, "the earlier reporter is back after restore")
}
