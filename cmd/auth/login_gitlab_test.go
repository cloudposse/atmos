package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/signals"
)

// TestAuthLoginGitLabCredentialCleanup exercises the actual login command with
// mock STS, including chained-role and file-setup failures after credentials exist.
func TestAuthLoginGitLabCredentialCleanup(t *testing.T) {
	for _, tc := range []struct {
		name          string
		target        string
		failChain     bool
		failFiles     bool
		missingToken  bool
		inherited     bool
		interrupt     bool
		wantActions   []string
		wantAuthError bool
	}{
		{name: "direct role", target: "runner", wantActions: []string{"AssumeRoleWithWebIdentity"}},
		{name: "chained role", target: "production", wantActions: []string{"AssumeRoleWithWebIdentity", "AssumeRole"}},
		{name: "mixed case identity", target: "RUNNER", wantActions: []string{"AssumeRoleWithWebIdentity"}},
		{name: "enclosing exec scope", target: "runner", inherited: true, wantActions: []string{"AssumeRoleWithWebIdentity"}},
		{name: "chained role failure", target: "production", failChain: true, wantActions: []string{"AssumeRoleWithWebIdentity", "AssumeRole"}, wantAuthError: true},
		{name: "partial file setup", target: "runner", failFiles: true, wantActions: []string{"AssumeRoleWithWebIdentity"}, wantAuthError: true},
		{name: "missing job token", target: "runner", missingToken: true, wantAuthError: true},
		{name: "interrupted login", target: "runner", interrupt: true, wantActions: []string{"AssumeRoleWithWebIdentity"}, wantAuthError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var files *awsCloud.AWSFileManager
			var privateDirectory string
			files, actions := setupGitLabExecFixture(t, tc.failChain, func() error {
				directories, err := filepath.Glob(filepath.Join(os.TempDir(), "atmos-auth-login-*"))
				if err != nil {
					return err
				}
				if len(directories) != 1 {
					if !tc.failFiles {
						return nil
					}
					return fmt.Errorf("expected one private login directory, got %d", len(directories))
				}
				privateDirectory = directories[0]
				info, err := os.Stat(privateDirectory)
				if err != nil {
					return err
				}
				if runtime.GOOS != "windows" {
					assert.Zero(t, info.Mode().Perm()&0o077, "login credentials exclude other users")
				}
				if tc.failFiles {
					privateFiles, err := awsCloud.NewAWSFileManager(privateDirectory, files.GetRealm())
					if err != nil {
						return err
					}
					// Block config creation after the credentials file has been written.
					return os.MkdirAll(privateFiles.GetConfigPath("gitlab"), 0o700)
				}
				if tc.interrupt {
					assert.True(t, signals.HandleShutdownSignal(syscall.SIGTERM))
				}
				return nil
			})
			if tc.missingToken {
				t.Setenv("ATMOS_OIDC_TOKEN", "")
			}
			if tc.inherited {
				parentDirectory := t.TempDir()
				t.Setenv(auth.AuthExecAWSFilesBasePathEnv, parentDirectory)
				sentinel := filepath.Join(parentDirectory, "parent-invocation")
				require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0o600))
				t.Cleanup(func() { assert.FileExists(t, sentinel) })
			}
			cmd := newIdentityFlowTestCmd(t)
			loginParser.RegisterFlags(cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--identity=" + tc.target}))

			err := executeAuthLoginCommand(cmd, nil)

			if tc.wantAuthError {
				require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
			} else {
				require.NoError(t, err)
			}
			if tc.interrupt {
				assert.Equal(t, signals.ExitCodeForSignal(syscall.SIGTERM), errUtils.GetExitCode(err))
			}
			stored, loadErr := awsCloud.LoadINIFile(files.GetCredentialsPath("gitlab"))
			require.NoError(t, loadErr)
			assert.False(t, stored.HasSection(strings.ToLower(tc.target)), "login must not retain derived AWS credentials")
			assertGitLabExecSharedProfilesPreserved(t, files)
			if len(tc.wantActions) == 0 {
				assert.Empty(t, actions())
			} else {
				assert.Equal(t, tc.wantActions, actions())
				require.NotEmpty(t, privateDirectory)
				assert.NoDirExists(t, privateDirectory, "login must remove its files before returning")
			}
			directories, err := filepath.Glob(filepath.Join(os.TempDir(), "atmos-auth-login-*"))
			require.NoError(t, err)
			assert.Empty(t, directories, "failed login must also clean up its private directory")
		})
	}
}

// TestAuthLoginGitLabCleanupFailure verifies that a filesystem cleanup error is
// reported without hiding an earlier authentication failure or deleting shared profiles.
func TestAuthLoginGitLabCleanupFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permissions enforced for an unprivileged user")
	}
	for _, failChain := range []bool{false, true} {
		t.Run(fmt.Sprintf("authentication-failure=%t", failChain), func(t *testing.T) {
			var lockedDirectory string
			files, _ := setupGitLabExecFixture(t, failChain, func() error {
				directories, err := filepath.Glob(filepath.Join(os.TempDir(), "atmos-auth-login-*"))
				if err != nil {
					return err
				}
				if len(directories) != 1 {
					return fmt.Errorf("expected one private login directory, got %d", len(directories))
				}
				lockedDirectory = filepath.Join(directories[0], "locked")
				if err := os.Mkdir(lockedDirectory, 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(lockedDirectory, "keep"), []byte("synthetic"), 0o600); err != nil {
					return err
				}
				return os.Chmod(lockedDirectory, 0o500)
			})
			t.Cleanup(func() {
				if lockedDirectory != "" {
					require.NoError(t, os.Chmod(lockedDirectory, 0o700))
				}
			})
			target := "runner"
			if failChain {
				target = "production"
			}
			cmd := newIdentityFlowTestCmd(t)
			loginParser.RegisterFlags(cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--identity=" + target}))

			err := executeAuthLoginCommand(cmd, nil)

			require.ErrorContains(t, err, "failed to clean up GitLab OIDC login credential directory")
			if failChain {
				assert.ErrorContains(t, err, "test rejection", "cleanup must preserve the STS error")
			}
			assertGitLabExecSharedProfilesPreserved(t, files)
		})
	}
}

// TestAuthLoginGitLabUnavailableTempDirectory verifies that login fails before
// STS rather than falling back to shared credential storage when isolation fails.
func TestAuthLoginGitLabUnavailableTempDirectory(t *testing.T) {
	files, actions := setupGitLabExecFixture(t, false)
	missingDirectory := filepath.Join(os.TempDir(), "missing")
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, missingDirectory)
	}
	cmd := newIdentityFlowTestCmd(t)
	loginParser.RegisterFlags(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--identity=runner"}))

	err := executeAuthLoginCommand(cmd, nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to create GitLab OIDC login credential directory")
	assert.Empty(t, actions())
	assertGitLabExecSharedProfilesPreserved(t, files)
}
