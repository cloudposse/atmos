package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// authExecChildEnv turns the test binary into the child process `auth exec` runs.
	authExecChildEnv = "_ATMOS_TEST_AUTH_EXEC_CHILD"
	// authExecChildReportEnv names the file the child writes its observations to.
	authExecChildReportEnv = "_ATMOS_TEST_AUTH_EXEC_REPORT"
	// authExecChildExitEnv is the exit code the child terminates with.
	authExecChildExitEnv = "_ATMOS_TEST_AUTH_EXEC_EXIT"
)

// authExecChildReport is what the child process observed.
type authExecChildReport struct {
	Args     []string `json:"args"`
	Identity string   `json:"identity"`
}

// runAuthExecChild records the child's argv and authenticated identity, then
// exits with the requested code. It is invoked from TestMain.
func runAuthExecChild() int {
	report, err := json.Marshal(authExecChildReport{Args: os.Args[1:], Identity: os.Getenv("ATMOS_IDENTITY")})
	if err != nil {
		return 98
	}
	if err := os.WriteFile(os.Getenv(authExecChildReportEnv), report, 0o600); err != nil {
		return 99
	}
	code, err := strconv.Atoi(os.Getenv(authExecChildExitEnv))
	if err != nil {
		return 97
	}
	return code
}

// TestExecuteAuthExecCommand_ChildArgsAndExitCode runs a real child process
// through `auth exec` and verifies that arguments after "--" reach it verbatim
// (no shell interpretation, no Atmos flag parsing), that it runs with the
// authenticated identity's environment, and that its exit status propagates.
func TestExecuteAuthExecCommand_ChildArgsAndExitCode(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	childArgs := []string{
		"two words",
		`it's "quoted"`,
		"$HOME and $(whoami) and `id`",
		"line1\nline2",
		"--identity=not-an-atmos-flag",
		"",
	}

	for _, tt := range []struct {
		name     string
		exitCode int
	}{
		{name: "success", exitCode: 0},
		{name: "failure exit code propagates", exitCode: 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupMockAuthFixture(t)
			reportPath := filepath.Join(t.TempDir(), "child-report.json")
			t.Setenv(authExecChildEnv, "1")
			t.Setenv(authExecChildReportEnv, reportPath)
			t.Setenv(authExecChildExitEnv, strconv.Itoa(tt.exitCode))

			cmd := authExecCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags(append([]string{"--", executable}, childArgs...)))

			err := executeAuthExecCommand(cmd, cmd.Flags().Args())

			if tt.exitCode == 0 {
				require.NoError(t, err)
			} else {
				var exitErr errUtils.ExitCodeError
				require.True(t, errors.As(err, &exitErr), "expected ExitCodeError, got %v", err)
				assert.Equal(t, tt.exitCode, exitErr.Code)
				assert.Equal(t, tt.exitCode, errUtils.GetExitCode(err))
			}

			raw, readErr := os.ReadFile(reportPath)
			require.NoError(t, readErr, "the child process must have run")
			var report authExecChildReport
			require.NoError(t, json.Unmarshal(raw, &report))
			assert.Equal(t, childArgs, report.Args, "arguments after -- must reach the child verbatim")
			require.Len(t, report.Args, len(childArgs))
			assert.Equal(t, childArgs[0], report.Args[0])
			assert.Equal(t, childArgs[len(childArgs)-1], report.Args[len(report.Args)-1])
			assert.Equal(t, "mock-identity", report.Identity, "the child must run as the resolved default identity")
		})
	}
}
