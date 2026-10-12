package doctor

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/validation"
)

func TestRenderSummary(t *testing.T) {
	for _, tt := range []struct {
		name   string
		report diagnostic.Report
		want   []string
		absent []string
	}{
		{
			name: "counts fixed and remaining findings",
			report: diagnostic.Report{
				ChangedFiles: []string{"atmos.yaml"},
				Findings: []diagnostic.Finding{
					{RuleID: "a", Status: "fixed"},
					{RuleID: "b", Status: "manual"},
					{RuleID: "c", Status: "available"},
				},
			},
			want:   []string{"Fixed 1 findings in 1 files. 2 findings remain."},
			absent: []string{"Dry run", "Backup:"},
		},
		{
			name: "explains skipped and incomplete checks only",
			report: diagnostic.Report{Checks: []diagnostic.Check{
				{ID: "stacks", Status: "skipped", Reason: "No stack manifests"},
				{ID: "imports", Status: "incomplete", Reason: "Unresolved import"},
				{ID: "schema", Status: "passed", Reason: "never shown"},
			}},
			want:   []string{"stacks: skipped. No stack manifests", "imports: incomplete. Unresolved import"},
			absent: []string{"never shown"},
		},
		{
			name:   "dry run is announced",
			report: diagnostic.Report{DryRun: true},
			want:   []string{"Dry run: project files and backups were not written."},
		},
		{
			name:   "backup location and recovery instructions are shown",
			report: diagnostic.Report{Backup: filepath.Join("var", "backup-1"), Recovery: "Restore with atmos doctor --recover"},
			want:   []string{"Backup: " + filepath.Join("var", "backup-1"), "Restore with atmos doctor --recover"},
		},
		{
			name:   "operational error is surfaced",
			report: diagnostic.Report{OperationalError: "backup could not be created"},
			want:   []string{"backup could not be created"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := captureIO(t)
			require.NoError(t, renderSummary(&tt.report))
			assert.Empty(t, stdout.String())
			for _, want := range tt.want {
				assert.Contains(t, stderr.String(), want)
			}
			for _, absent := range tt.absent {
				assert.NotContains(t, stderr.String(), absent)
			}
		})
	}
}

func TestAmbientInputs(t *testing.T) {
	const (
		reconfigureReason = "Legacy Terraform reconfigure environment override is active"
		discoveryReason   = "Home or system Atmos configuration exists outside this project"
	)

	t.Run("reconfigure override requires effective-input review", func(t *testing.T) {
		isolateAmbientConfig(t)
		t.Setenv(reconfigureEnv, "true")
		reasons := ambientInputs(&diagnostic.Options{ConfigFiles: []string{"atmos.yaml"}})
		require.Len(t, reasons, 1)
		assert.Contains(t, reasons[0], reconfigureReason)
	})

	t.Run("empty reconfigure override is ignored", func(t *testing.T) {
		isolateAmbientConfig(t)
		t.Setenv(reconfigureEnv, "")
		assert.Empty(t, ambientInputs(&diagnostic.Options{ConfigFiles: []string{"atmos.yaml"}}))
	})

	t.Run("explicit selections bypass home discovery", func(t *testing.T) {
		for name, options := range map[string]diagnostic.Options{
			"config file": {ConfigFiles: []string{"atmos.yaml"}},
			"config dir":  {ConfigDirs: []string{"conf"}},
		} {
			t.Run(name, func(t *testing.T) {
				home := isolateAmbientConfig(t)
				writeProjectFile(t, home, filepath.Join(".atmos", "atmos.yaml"), "{}\n")
				assert.Empty(t, ambientInputs(&options))
			})
		}
	})

	t.Run("home configuration is reported when nothing is selected", func(t *testing.T) {
		home := isolateAmbientConfig(t)
		writeProjectFile(t, home, filepath.Join(".atmos", "atmos.yaml"), "{}\n")
		reasons := ambientInputs(&diagnostic.Options{})
		require.Len(t, reasons, 1)
		assert.Contains(t, reasons[0], discoveryReason)
	})

	t.Run("override and home configuration are both reported", func(t *testing.T) {
		home := isolateAmbientConfig(t)
		writeProjectFile(t, home, filepath.Join(".atmos", "atmos.d", "extra.yaml"), "{}\n")
		t.Setenv(reconfigureEnv, "1")
		reasons := ambientInputs(&diagnostic.Options{})
		require.Len(t, reasons, 2)
		assert.Contains(t, reasons[0], reconfigureReason)
		assert.Contains(t, reasons[1], discoveryReason)
	})

	t.Run("clean home reports nothing", func(t *testing.T) {
		isolateAmbientConfig(t)
		if runtime.GOOS != "windows" && ambientDirectoryExists(config.SystemDirConfigFilePath) {
			t.Skip("machine-wide Atmos configuration exists on this host")
		}
		assert.Empty(t, ambientInputs(&diagnostic.Options{}))
	})
}

func TestAmbientDirectoryExists(t *testing.T) {
	assert.False(t, ambientDirectoryExists(""), "an empty directory name is never configuration")
	assert.False(t, ambientDirectoryExists(t.TempDir()), "an empty directory has no configuration")
	assert.False(t, ambientDirectoryExists(filepath.Join(t.TempDir(), "missing")))

	for _, name := range []string{"atmos.yaml", "atmos.yml", "atmos.d", ".atmos.d"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProjectFile(t, dir, filepath.Join(name, "marker"), "{}\n")
			assert.True(t, ambientDirectoryExists(dir))
		})
	}

	t.Run("unreadable locations are treated as present", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows reports a file used as a directory as not-exist.")
		}
		dir := t.TempDir()
		blocker := filepath.Join(dir, "file")
		require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
		assert.True(t, ambientDirectoryExists(blocker), "an inconclusive stat must not hide possible configuration")
	})
}

func TestRenderSARIFMarksOperationalFailure(t *testing.T) {
	for _, tt := range []struct {
		name        string
		report      diagnostic.Report
		wantSuccess bool
	}{
		{"clean run", diagnostic.Report{Version: 1}, true},
		{"operational failure", diagnostic.Report{Version: 1, OperationalError: "boom"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _ := captureIO(t)
			require.NoError(t, renderSARIF(&tt.report))
			var document struct {
				Runs []struct {
					Invocations []struct {
						ExecutionSuccessful bool `json:"executionSuccessful"`
					} `json:"invocations"`
				} `json:"runs"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &document))
			require.Len(t, document.Runs, 1)
			require.Len(t, document.Runs[0].Invocations, 1)
			assert.Equal(t, tt.wantSuccess, document.Runs[0].Invocations[0].ExecutionSuccessful)
		})
	}
}

type failingWriter struct{}

var errWriteRefused = errors.New("write refused")

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteRefused }

func TestRunReportsOutputWriteFailureAsOperationalError(t *testing.T) {
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
	var human bytes.Buffer
	ctx, err := iolib.NewContext(iolib.WithStreams(&testStreams{out: failingWriter{}, err: &human}))
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	data.InitWriter(ctx)
	t.Cleanup(ui.Reset)

	err = executeDoctor(t, "--offline", "--format=json")

	// A report that could not be delivered must not look like a clean run.
	requireExitCode(t, err, 2)
}

func TestRenderFindingsHidesFixedAndShowsGuidance(t *testing.T) {
	_, stderr := captureIO(t)
	report := diagnostic.Report{Findings: []diagnostic.Finding{
		{RuleID: "applied-rule", Severity: validation.SeverityWarning, File: "done.yaml", Line: 1, Message: "already repaired", Status: "fixed"},
		{RuleID: "open-rule", Severity: validation.SeverityError, File: "stacks.yaml", Line: 7, Message: "needs attention", Status: "manual", Reason: "cannot be verified", Remediation: "edit by hand"},
		{RuleID: "global-rule", Severity: validation.SeverityWarning, File: "atmos.yaml", Message: "no line known", Status: "available"},
	}}

	renderFindings(&report)

	output := stderr.String()
	assert.NotContains(t, output, "applied-rule")
	assert.Contains(t, output, "stacks.yaml:7 open-rule [error, manual] needs attention")
	assert.Contains(t, output, "cannot be verified")
	assert.Contains(t, output, "edit by hand")
	// Findings without a line report the file alone.
	assert.Contains(t, output, "atmos.yaml global-rule [warning, available] no line known")
	assert.NotContains(t, output, "atmos.yaml:0")
}

func TestProgressObserverTranslatesEngineEvents(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	wd, err := os.Getwd()
	require.NoError(t, err)

	var got []batch.Event
	observe := progressObserver(func(e batch.Event) { got = append(got, e) })

	observe(diagnostic.Event{Phase: "Check", Label: "atmos.yaml", Total: 2})
	finding := diagnostic.Finding{RuleID: "command-provides", Severity: validation.SeverityWarning, File: filepath.Join(wd, "atmos.yaml"), Line: 5, Message: "legacy", Status: "available"}
	observe(diagnostic.Event{Phase: "Check", Finding: &finding})
	observe(diagnostic.Event{Phase: "Check", Label: "atmos.yaml", Total: 2, Done: true, Status: "findings"})
	observe(diagnostic.Event{Phase: "Verify", Label: "candidate", Status: "warning"})
	observe(diagnostic.Event{Phase: "Verify", Label: "candidate", Done: true, Status: "passed"})

	require.Len(t, got, 7)
	// The first event of a phase resets progress to that phase's total.
	assert.Equal(t, batch.Event{Reset: true, Count: 2}, got[0])
	assert.Equal(t, "atmos.yaml", got[1].Label)
	assert.Equal(t, 2, got[1].Count)
	assert.True(t, got[1].Diagnostic)
	assert.False(t, got[1].Done)
	// Findings stream as warnings with the file relative to the working directory.
	assert.Equal(t, batch.Event{Warning: "atmos.yaml:5 command-provides [available] legacy", Severity: "warning"}, got[2])
	// The same label in the same phase keeps its identifier so it completes the item it started.
	assert.Equal(t, got[1].ID, got[3].ID)
	assert.True(t, got[3].Done)
	assert.Equal(t, "findings", got[3].Outcome)
	// A new phase resets again, and status "warning" becomes a plain warning line.
	assert.Equal(t, batch.Event{Reset: true}, got[4])
	assert.Equal(t, batch.Event{Warning: "candidate"}, got[5])
	// Untotaled work completes with an unknown count rather than a stale total.
	assert.Equal(t, -1, got[6].Count)
	assert.True(t, got[6].Done)
	assert.Equal(t, "passed", got[6].Outcome)
}

func TestProgressObserverMarksUntotaledWorkAsUnknown(t *testing.T) {
	var got []batch.Event
	observe := progressObserver(func(e batch.Event) { got = append(got, e) })

	observe(diagnostic.Event{Phase: "Discover", Label: "Project inputs"})
	observe(diagnostic.Event{Phase: "Discover", Label: "Other inputs"})

	require.Len(t, got, 3)
	assert.Equal(t, batch.Event{Reset: true}, got[0])
	assert.Equal(t, -1, got[1].Count)
	assert.Equal(t, -1, got[2].Count)
	assert.NotEqual(t, got[1].ID, got[2].ID, "distinct labels must not share an identifier")
}
