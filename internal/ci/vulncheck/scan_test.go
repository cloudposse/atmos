package vulncheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errScannerFailed = errors.New("scanner failed")

// fakeRunner records the command it was asked to run and answers with the
// canned stdout and error, standing in for govulncheck.
type fakeRunner struct {
	stdout string
	stderr string
	err    error

	name string
	args []string
}

func (f *fakeRunner) run(_ context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	f.name, f.args = name, args
	_, _ = io.WriteString(stdout, f.stdout)
	_, _ = io.WriteString(stderr, f.stderr)
	return f.err
}

const duplicatedReport = `{"runs":[{"results":[{"ruleId":"GO-1","stacks":[{"a":1},{"a":1},{"b":2}]}]}]}`

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// noTempFiles fails the test if a temporary report was left beside the output.
func noTempFiles(t *testing.T, dir string) {
	t.Helper()

	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestScan_WritesNormalizedReport(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "govulncheck.sarif")
	runner := &fakeRunner{stdout: duplicatedReport, stderr: "Scanning your code\n"}
	var stderr bytes.Buffer

	err := scan(context.Background(), &Params{Output: output, Stderr: &stderr}, runner.run)
	require.NoError(t, err)

	assert.JSONEq(t, `{"runs":[{"results":[{"ruleId":"GO-1","stacks":[{"a":1},{"b":2}]}]}]}`, readFile(t, output))
	assert.Equal(t, "Scanning your code\n", stderr.String(), "scanner diagnostics must reach the caller")
	noTempFiles(t, dir)
}

func TestScan_Command(t *testing.T) {
	tests := []struct {
		name     string
		packages []string
		wantArgs []string
	}{
		{"defaults to all packages", nil, []string{"-format", "sarif", "./..."}},
		{"honors package patterns", []string{"./cmd/...", "./pkg/..."}, []string{"-format", "sarif", "./cmd/...", "./pkg/..."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{stdout: duplicatedReport}
			params := &Params{Output: filepath.Join(t.TempDir(), "out.sarif"), Packages: tt.packages, Stderr: io.Discard}

			require.NoError(t, scan(context.Background(), params, runner.run))

			assert.Equal(t, "govulncheck", runner.name)
			assert.Equal(t, tt.wantArgs, runner.args)
		})
	}
}

func TestScan_FailureLeavesExistingReportUntouched(t *testing.T) {
	const previous = `{"runs":[],"previous":true}`

	tests := []struct {
		name    string
		runner  *fakeRunner
		wantErr error
	}{
		{"scanner exits non-zero", &fakeRunner{stdout: duplicatedReport, err: errScannerFailed}, errScannerFailed},
		{"scanner prints nothing", &fakeRunner{}, errInvalidSARIF},
		{"scanner prints garbage", &fakeRunner{stdout: "not sarif"}, errInvalidSARIF},
		{"scanner output is cut short", &fakeRunner{stdout: `{"runs":[{"results":[`}, errInvalidSARIF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "govulncheck.sarif")
			require.NoError(t, os.WriteFile(output, []byte(previous), 0o644))

			err := scan(context.Background(), &Params{Output: output, Stderr: io.Discard}, tt.runner.run)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, previous, readFile(t, output), "a failed scan must never replace the report")
			noTempFiles(t, dir)
		})
	}
}

func TestScan_FailureCreatesNoReport(t *testing.T) {
	output := filepath.Join(t.TempDir(), "govulncheck.sarif")
	runner := &fakeRunner{err: errScannerFailed}

	require.ErrorIs(t, scan(context.Background(), &Params{Output: output, Stderr: io.Discard}, runner.run), errScannerFailed)

	_, err := os.Stat(output)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestScan_ReplacesExistingReport(t *testing.T) {
	output := filepath.Join(t.TempDir(), "govulncheck.sarif")
	require.NoError(t, os.WriteFile(output, []byte("stale"), 0o644))
	runner := &fakeRunner{stdout: duplicatedReport}

	require.NoError(t, scan(context.Background(), &Params{Output: output, Stderr: io.Discard}, runner.run))

	assert.JSONEq(t, `{"runs":[{"results":[{"ruleId":"GO-1","stacks":[{"a":1},{"b":2}]}]}]}`, readFile(t, output))
}

func TestScan_RejectsMissingOutput(t *testing.T) {
	runner := &fakeRunner{stdout: duplicatedReport}

	for _, params := range []*Params{nil, {}} {
		require.ErrorIs(t, scan(context.Background(), params, runner.run), errInvalidParams)
	}
	assert.Empty(t, runner.name, "the scanner must not run without an output path")
}

func TestScan_UnwritableOutputDirectory(t *testing.T) {
	output := filepath.Join(t.TempDir(), "missing-dir", "govulncheck.sarif")
	runner := &fakeRunner{stdout: duplicatedReport}

	err := scan(context.Background(), &Params{Output: output, Stderr: io.Discard}, runner.run)

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunCommand_WiresStreams(t *testing.T) {
	// The test binary lists matching tests on stdout, which makes it a
	// cross-platform stand-in for an external tool.
	exe, err := os.Executable()
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	err = runCommand(context.Background(), &stdout, &stderr, exe, "-test.list", "^TestRunCommand_WiresStreams$")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "TestRunCommand_WiresStreams")
	assert.Empty(t, stderr.String())
}

func TestRunCommand_MissingBinary(t *testing.T) {
	err := runCommand(context.Background(), io.Discard, io.Discard, filepath.Join(t.TempDir(), "no-such-tool"))

	require.Error(t, err)
}

func TestScan_ReportsMissingGovulncheck(t *testing.T) {
	// With nothing on PATH the real runner cannot find the tool, and Scan must
	// say so instead of writing an empty report.
	t.Setenv("PATH", t.TempDir())
	output := filepath.Join(t.TempDir(), "govulncheck.sarif")

	err := Scan(context.Background(), &Params{Output: output, Stderr: io.Discard})

	require.ErrorIs(t, err, exec.ErrNotFound)
	assert.ErrorContains(t, err, "govulncheck")
	_, statErr := os.Stat(output)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
