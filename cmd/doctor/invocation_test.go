package doctor

import (
	"encoding/json"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
)

// Compile-time sentinel for the report field that invocation errors must carry.
var _ = diagnostic.Report{OperationalError: "", DryRun: false}

func TestInvalidOptionValuesNameOptionValueAndChoices(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want []string
	}{
		{"format", []string{"--format=yaml"}, []string{"--format", `"yaml"`, "text, rich, json, sarif"}},
		{"fail-on", []string{"--fail-on=bogus"}, []string{"--fail-on", `"bogus"`, "error, warning, none"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
			captureIO(t)

			err := executeDoctor(t, tt.args...)

			requireExitCode(t, err, 2)
			require.ErrorIs(t, err, diagnostic.ErrOptions)
			assert.Contains(t, err.Error(), "invalid doctor options")
			for _, want := range tt.want {
				assert.Contains(t, err.Error()+" "+hintsOf(err), want)
			}
		})
	}
}

// hintsOf flattens the builder hints so tests can assert on valid values.
func hintsOf(err error) string {
	return strings.Join(cockroach.GetAllHints(err), "\n")
}

// TestMachineFormatsReportEveryInvocationErrorAsJSON verifies one stdout shape for all invocation errors.
func TestMachineFormatsReportEveryInvocationErrorAsJSON(t *testing.T) {
	for _, format := range []string{"json", "sarif"} {
		for _, tt := range []struct {
			name       string
			args       []string
			wantError  string
			wantDryRun bool
		}{
			{"invalid fail-on", []string{"--fail-on=bogus"}, `--fail-on="bogus"`, false},
			{"base without affected", []string{"--base=main"}, "--base requires --affected", false},
			{"unknown check", []string{"--only=bogus"}, `--only="bogus"`, false},
			{"dry-run without fix", []string{"--dry-run"}, "--dry-run requires --fix", true},
			{"list-checks with fix", []string{"--list-checks", "--fix"}, "--fix cannot be combined with --list-checks", false},
			{"invalid exclude glob", []string{"--exclude=["}, "", false},
		} {
			t.Run(format+"/"+tt.name, func(t *testing.T) {
				isolateAmbientConfig(t)
				newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
				stdout, stderr := captureIO(t)

				err := executeDoctor(t, append([]string{"--offline", "--format=" + format}, tt.args...)...)

				requireExitCode(t, err, 2)
				assert.Empty(t, stderr.String(), "machine formats keep the human channel clean")
				report := decodeReport(t, format, stdout.Bytes())
				assert.NotEmpty(t, report.OperationalError)
				assert.Contains(t, report.OperationalError, tt.wantError)
				assert.Equal(t, tt.wantDryRun, report.DryRun)
				assert.Equal(t, 1, report.Version)
			})
		}
	}
}

// decodeReport extracts the doctor report from either machine format.
func decodeReport(t *testing.T, format string, out []byte) diagnostic.Report {
	t.Helper()
	if format == "sarif" {
		var document struct {
			Runs []struct {
				Properties struct {
					Doctor diagnostic.Report `json:"doctor"`
				} `json:"properties"`
			} `json:"runs"`
		}
		require.NoError(t, json.Unmarshal(out, &document))
		require.Len(t, document.Runs, 1)
		return document.Runs[0].Properties.Doctor
	}
	var report diagnostic.Report
	require.NoError(t, json.Unmarshal(out, &report))
	return report
}

func TestInvalidFormatStaysAPlainError(t *testing.T) {
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
	stdout, _ := captureIO(t)

	err := executeDoctor(t, "--format=yaml", "--fail-on=bogus")

	requireExitCode(t, err, 2)
	assert.Contains(t, err.Error(), `--format="yaml"`)
	assert.Empty(t, stdout.String(), "an unknown format cannot select a machine encoding")
}

func TestTextInvocationErrorsPrintNoSummary(t *testing.T) {
	for _, args := range [][]string{
		{"--dry-run"},
		{"--only=bogus"},
		{"--base=main"},
		{"--fail-on=bogus"},
		{"--list-checks", "--dry-run"},
	} {
		t.Run(args[0], func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
			stdout, stderr := captureIO(t)

			err := executeDoctor(t, append([]string{"--offline"}, args...)...)

			requireExitCode(t, err, 2)
			require.ErrorIs(t, err, diagnostic.ErrOptions)
			combined := plainText(stdout) + plainText(stderr)
			assert.NotContains(t, combined, "Fixed")
			assert.NotContains(t, combined, "findings")
			assert.NotContains(t, combined, "Dry run")
		})
	}
}

func TestUnknownCheckHintListsValidSelectors(t *testing.T) {
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
	captureIO(t)

	err := executeDoctor(t, "--only=bogus")

	require.ErrorIs(t, err, diagnostic.ErrOptions)
	hints := hintsOf(err)
	for _, rule := range diagnostic.Rules() {
		assert.Contains(t, hints, rule.ID)
	}
	assert.Contains(t, hints, "--list-checks")
}

func TestDryRunReportsDryRunInJSON(t *testing.T) {
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	stdout, _ := captureIO(t)

	require.NoError(t, executeDoctor(t, "--offline", "--format=json", "--fix", "--dry-run"))

	report := decodeReport(t, "json", stdout.Bytes())
	assert.True(t, report.DryRun)
	assert.Empty(t, report.OperationalError)
}

func TestListChecksRejectsMeaninglessFlags(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"fix", []string{"--fix"}},
		{"dry-run", []string{"--dry-run"}},
		{"affected", []string{"--affected"}},
		{"base", []string{"--base=main"}},
		{"exclude", []string{"--exclude=vendor/**"}},
		{"only", []string{"--only=schema"}},
		{"offline", []string{"--offline"}},
		{"rich format", []string{"--format=rich"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := captureIO(t)

			err := executeDoctor(t, append([]string{"--list-checks"}, tt.args...)...)

			requireExitCode(t, err, 2)
			require.ErrorIs(t, err, diagnostic.ErrOptions)
			assert.Contains(t, err.Error(), "--list-checks")
			assert.Empty(t, stdout.String(), "no partial listing on an invalid combination")
		})
	}
}

func TestListChecksAcceptsFormatAndThreshold(t *testing.T) {
	for _, args := range [][]string{
		{"--list-checks"},
		{"--list-checks", "--format=json"},
		{"--list-checks", "--format=sarif", "--fail-on=none"},
		{"--list-checks", "--format=text", "--fail-on=warning"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			stdout, stderr := captureIO(t)

			require.NoError(t, executeDoctor(t, args...))

			assert.NotEmpty(t, stdout.String(), "the listing is data and belongs on stdout")
			assert.Empty(t, stderr.String())
		})
	}
}

func TestFixRunReportsFixedCountsAndCheckRunReportsFixable(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		want   string
		absent string
	}{
		{"check only", []string{"--offline"}, "1 finding (1 fixable). Run atmos doctor --fix to apply 1.", "Fixed"},
		{"fix", []string{"--offline", "--fix", "--dry-run"}, "findings remain.", "fixable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
			_, stderr := captureIO(t)

			require.NoError(t, executeDoctor(t, tt.args...))

			assert.Contains(t, plainText(stderr), tt.want)
			assert.NotContains(t, plainText(stderr), tt.absent)
		})
	}
}

func TestFindingIsPrintedOnce(t *testing.T) {
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	_, stderr := captureIO(t)

	require.NoError(t, executeDoctor(t, "--offline"))

	assert.Equal(t, 1, strings.Count(plainText(stderr), "Legacy command-provides declaration"))
}
