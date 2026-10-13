package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
)

// Compile-time sentinels for the Report fields these tests construct.
var _ = diagnostic.Report{Backup: "", Recovery: "", DryRun: false, OperationalError: ""}

const (
	reconfigureEnv = "ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE"
	// The legacy command config declares a semantic argument type that doctor reports as a warning.
	legacyCommandConfig = "commands:\n  - name: sample\n    arguments:\n      - name: target\n        type: stack\n"
	// The stack project config points doctor at a static stacks directory with a simple name pattern.
	stackProjectConfig = "base_path: ./\ncomponents:\n  terraform:\n    base_path: components/terraform\nstacks:\n  base_path: stacks\n  included_paths: ['**/*']\n  name_pattern: '{stage}'\n"
)

// captureIO routes the data (stdout) and UI (stderr) channels to buffers.
// Strip ANSI styling so assertions hold on terminals and CI runners that advertise color.
func plainText(b *bytes.Buffer) string {
	return ansi.Strip(b.String())
}

func captureIO(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	var out, errBuf bytes.Buffer
	ctx, err := iolib.NewContext(iolib.WithStreams(&testStreams{out: &out, err: &errBuf}))
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	data.InitWriter(ctx)
	t.Cleanup(ui.Reset)
	return &out, &errBuf
}

// isolateAmbientConfig points home and system configuration discovery at empty directories
// and clears the legacy reconfigure override, so only the project under test is visible.
func isolateAmbientConfig(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv(config.WindowsAppDataEnvVar, t.TempDir())
	t.Setenv(reconfigureEnv, "")
	previous := homedir.DisableCache
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = previous })
	return home
}

func writeProjectFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// newProjectDir creates a project with the given files and makes it the working directory.
func newProjectDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		writeProjectFile(t, root, name, content)
	}
	t.Chdir(root)
	return root
}

func executeDoctor(t *testing.T, args ...string) error {
	t.Helper()
	return executeDoctorContext(t, context.Background(), args...)
}

func executeDoctorContext(t *testing.T, ctx context.Context, args ...string) error {
	t.Helper()
	cmd := NewCommand()
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func requireExitCode(t *testing.T, err error, want int) {
	t.Helper()
	var exitErr errUtils.ExitCodeError
	if want == 0 {
		require.NoError(t, err)
		return
	}
	require.Error(t, err)
	require.Equal(t, want, errUtils.GetExitCode(err))
	// Finding thresholds and cancellation are reported through a silent exit code only;
	// invocation errors keep their message.
	if errors.As(err, &exitErr) {
		require.Equal(t, want, exitErr.Code)
	}
}

func TestCommandProviderBuildsUsableCommand(t *testing.T) {
	provider := &CommandProvider{}
	cmd := provider.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, provider.GetName(), cmd.Name())
	for _, flag := range []string{"fix", "dry-run", "format", "fail-on", "only", "exclude", "affected", "base", "list-checks", "offline"} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), "flag %q must be registered", flag)
	}
	// Doctor must not inherit the root's configuration-loading pre-run, since broken
	// configuration is exactly what it diagnoses.
	require.NotNil(t, cmd.PersistentPreRunE)
	assert.NoError(t, cmd.PersistentPreRunE(cmd, nil))
}

func TestListChecks(t *testing.T) {
	rules := diagnostic.Rules()
	require.NotEmpty(t, rules)

	t.Run("text lists every check on the data channel", func(t *testing.T) {
		stdout, stderr := captureIO(t)
		require.NoError(t, listChecks("text"))
		assert.Empty(t, stderr.String())
		for _, rule := range rules {
			assert.Contains(t, plainText(stdout), rule.ID+" ["+rule.Category+"]: "+rule.Description)
		}
	})

	t.Run("json lists every check on the data channel", func(t *testing.T) {
		stdout, stderr := captureIO(t)
		require.NoError(t, listChecks("json"))
		assert.Empty(t, stderr.String())
		var listed []diagnostic.Rule
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &listed))
		require.Len(t, listed, len(rules))
		assert.Equal(t, rules[0].ID, listed[0].ID)
		assert.Equal(t, rules[len(rules)-1].ID, listed[len(listed)-1].ID)
	})

	t.Run("sarif embeds the checks as listed", func(t *testing.T) {
		stdout, stderr := captureIO(t)
		require.NoError(t, listChecks("sarif"))
		assert.Empty(t, stderr.String())
		var document struct {
			Runs []struct {
				Properties struct {
					Doctor diagnostic.Report `json:"doctor"`
				} `json:"properties"`
			} `json:"runs"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &document))
		require.Len(t, document.Runs, 1)
		checks := document.Runs[0].Properties.Doctor.Checks
		require.Len(t, checks, len(rules))
		assert.Equal(t, diagnostic.Check{ID: rules[0].ID, Status: "listed", Reason: rules[0].Description}, checks[0])
		assert.Equal(t, rules[len(rules)-1].ID, checks[len(checks)-1].ID)
	})
}

func TestRunListChecksSkipsProjectDiscovery(t *testing.T) {
	isolateAmbientConfig(t)
	// An empty directory has no configuration; listing checks must not need any.
	t.Chdir(t.TempDir())
	stdout, _ := captureIO(t)
	require.NoError(t, executeDoctor(t, "--list-checks", "--format=json"))
	var listed []diagnostic.Rule
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &listed))
	assert.Len(t, listed, len(diagnostic.Rules()))
}

func TestRunInvalidOptionsExitWithUsageCode(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"list-checks still validates format", []string{"--list-checks", "--format=xml"}},
		{"list-checks still validates threshold", []string{"--list-checks", "--fail-on=never"}},
		{"unknown check selector", []string{"--offline", "--only=no-such-check"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
			captureIO(t)
			err := executeDoctor(t, tt.args...)
			requireExitCode(t, err, 2)
		})
	}
}

func TestRunFindingsThresholdsAndFormats(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		wantExit int
	}{
		{"text default threshold passes on warnings", []string{"--offline"}, 0},
		{"text warning threshold fails", []string{"--offline", "--fail-on=warning"}, 1},
		{"text none threshold passes", []string{"--offline", "--fail-on=none"}, 0},
		{"rich warning threshold fails", []string{"--offline", "--format=rich", "--fail-on=warning"}, 1},
		{"json default threshold passes on warnings", []string{"--offline", "--format=json"}, 0},
		{"json warning threshold fails", []string{"--offline", "--format=json", "--fail-on=warning"}, 1},
		{"sarif warning threshold fails", []string{"--offline", "--format=sarif", "--fail-on=warning"}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
			stdout, stderr := captureIO(t)

			err := executeDoctor(t, tt.args...)

			requireExitCode(t, err, tt.wantExit)
			combined := plainText(stdout) + plainText(stderr)
			assert.Contains(t, combined, "command-provides")
			assert.Contains(t, combined, "Legacy command-provides declaration")
		})
	}
}

func TestRunMachineFormatsKeepHumanChannelClean(t *testing.T) {
	for _, format := range []string{"json", "sarif"} {
		t.Run(format, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
			stdout, stderr := captureIO(t)

			require.NoError(t, executeDoctor(t, "--offline", "--format="+format))

			assert.Empty(t, stderr.String(), "machine formats must not stream progress to the UI channel")
			var document map[string]any
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &document))
		})
	}
}

func TestRunStacksCheckUsesStaticStackValidator(t *testing.T) {
	for _, tt := range []struct {
		name       string
		stack      string
		wantExit   int
		wantStatus string
	}{
		{"valid stack passes", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc:\n      vars: {a: 1}\n", 0, "passed"},
		{"unknown inherited component fails", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc:\n      metadata:\n        inherits: [missing]\n", 1, "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": stackProjectConfig, "stacks/dev.yaml": tt.stack})
			stdout, _ := captureIO(t)

			err := executeDoctor(t, "--offline", "--format=json", "--only=stacks")

			requireExitCode(t, err, tt.wantExit)
			var report diagnostic.Report
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
			require.Len(t, report.Checks, 1)
			assert.Equal(t, diagnostic.Check{ID: "stacks", Status: tt.wantStatus, Files: 1}, report.Checks[0])
			if tt.wantExit != 0 {
				require.Len(t, report.Findings, 1)
				assert.Contains(t, report.Findings[0].Message, "missing")
				assert.Equal(t, validation.SeverityError, report.Findings[0].Severity)
			}
		})
	}
}

func TestRunOperationalErrorForcesFailure(t *testing.T) {
	// Even with --fail-on=none an operational failure must never be suppressed.
	isolateAmbientConfig(t)
	newProjectDir(t, map[string]string{"atmos.yaml": "{}\n"})
	stdout, _ := captureIO(t)

	err := executeDoctor(t, "--offline", "--format=json", "--dry-run", "--fail-on=none")

	requireExitCode(t, err, 2)
	var report diagnostic.Report
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Contains(t, report.OperationalError, "--dry-run requires --fix")
}

func TestRunDryRunLeavesProjectUntouched(t *testing.T) {
	isolateAmbientConfig(t)
	root := newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	_, stderr := captureIO(t)

	require.NoError(t, executeDoctor(t, "--offline", "--fix", "--dry-run"))

	assert.Contains(t, plainText(stderr), "Dry run: project files and backups were not written.")
	content, err := os.ReadFile(filepath.Join(root, "atmos.yaml"))
	require.NoError(t, err)
	assert.Equal(t, legacyCommandConfig, string(content))
}

func TestRunCancellationExitsWithSIGINTCode(t *testing.T) {
	for _, format := range []string{"json", "text"} {
		t.Run(format, func(t *testing.T) {
			isolateAmbientConfig(t)
			newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
			captureIO(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := executeDoctorContext(t, ctx, "--offline", "--format="+format)

			var exitErr errUtils.ExitCodeError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, errUtils.ExitCodeSIGINT, exitErr.Code)
			assert.True(t, exitErr.Silent)
		})
	}
}

// initGitRepository creates a repository in dir with a single commit of its current files.
func initGitRepository(t *testing.T, dir string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=Doctor Test", "-c", "user.email=doctor@example.invalid", "-c", "commit.gpgsign=false"}, args...)
		output, err := exec.Command("git", full...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	git("init", "--quiet")
	git("add", "-A")
	git("commit", "--quiet", "-m", "fixture")
}

func TestRunAffectedSelectsOnlyChangedInputs(t *testing.T) {
	isolateAmbientConfig(t)
	root := newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	initGitRepository(t, root)
	captureIO(t)

	t.Run("unchanged project has nothing to check", func(t *testing.T) {
		stdout, _ := captureIO(t)
		err := executeDoctor(t, "--offline", "--format=json", "--affected", "--base=HEAD", "--fail-on=warning")
		require.NoError(t, err)
		var report diagnostic.Report
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
		assert.Empty(t, report.Findings)
	})

	t.Run("modified configuration is checked", func(t *testing.T) {
		writeProjectFile(t, root, "atmos.yaml", legacyCommandConfig+"# edited\n")
		stdout, _ := captureIO(t)
		err := executeDoctor(t, "--offline", "--format=json", "--affected", "--base=HEAD", "--fail-on=warning")
		requireExitCode(t, err, 1)
		var report diagnostic.Report
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
		require.Len(t, report.Findings, 1)
		assert.Equal(t, "command-provides", report.Findings[0].RuleID)
	})
}

func TestRunAffectedRejectsUnresolvableBase(t *testing.T) {
	isolateAmbientConfig(t)
	root := newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	initGitRepository(t, root)

	t.Run("text keeps the error message", func(t *testing.T) {
		captureIO(t)
		err := executeDoctor(t, "--offline", "--affected", "--base=no-such-ref-for-doctor")
		requireExitCode(t, err, 2)
		assert.Contains(t, err.Error(), "no-such-ref-for-doctor")
	})

	t.Run("json reports the failure on stdout", func(t *testing.T) {
		stdout, _ := captureIO(t)
		err := executeDoctor(t, "--offline", "--format=json", "--affected", "--base=no-such-ref-for-doctor")
		requireExitCode(t, err, 2)
		var report diagnostic.Report
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
		assert.Contains(t, report.OperationalError, "no-such-ref-for-doctor")
	})
}

func TestRunAffectedOutsideGitRepositoryFails(t *testing.T) {
	isolateAmbientConfig(t)
	root := newProjectDir(t, map[string]string{"atmos.yaml": legacyCommandConfig})
	// Keep git from discovering a repository in an ancestor of the temporary project.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	stdout, _ := captureIO(t)

	err := executeDoctor(t, "--offline", "--format=json", "--affected", "--base=HEAD")

	requireExitCode(t, err, 2)
	var report diagnostic.Report
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &report))
	assert.Contains(t, report.OperationalError, "resolve validation base")
}
