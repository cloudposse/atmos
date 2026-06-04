package github

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

// TestHelperProcess is not a real test. It is invoked as a subprocess by fakeCLICmd
// to emulate the GitHub CLI: it prints HELPER_STDOUT and exits with HELPER_EXIT_CODE.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("HELPER_STDOUT"))
	if os.Getenv("HELPER_EXIT_CODE") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

// fakeCLICmd returns an *exec.Cmd that re-invokes the test binary as TestHelperProcess,
// emulating a CLI that prints stdout and exits with the given code. This is the standard
// cross-platform way to fake an external command (see Go's own os/exec tests).
func fakeCLICmd(stdout string, exitCode int) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"HELPER_STDOUT="+stdout,
		fmt.Sprintf("HELPER_EXIT_CODE=%d", exitCode),
	)
	return cmd
}

// withCommander swaps the package-level commander for the duration of the test.
func withCommander(t *testing.T, c execpkg.CommandExecutor) {
	t.Helper()
	orig := commander
	commander = c
	t.Cleanup(func() { commander = orig })
}

func TestGetGitHubToken(t *testing.T) {
	tests := []struct {
		name           string
		atmosToken     string
		githubToken    string
		expectedPrefix string // Token should start with this.
		expectEmpty    bool
	}{
		{
			name:        "no tokens set",
			atmosToken:  "",
			githubToken: "",
			expectEmpty: true, // May get token from gh CLI if installed.
		},
		{
			name:           "ATMOS_GITHUB_TOKEN set",
			atmosToken:     "atmos_token_123",
			githubToken:    "",
			expectedPrefix: "atmos_token_123",
		},
		{
			name:           "GITHUB_TOKEN set",
			atmosToken:     "",
			githubToken:    "github_token_456",
			expectedPrefix: "github_token_456",
		},
		{
			name:           "both tokens set - ATMOS takes precedence",
			atmosToken:     "atmos_token_789",
			githubToken:    "github_token_abc",
			expectedPrefix: "atmos_token_789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up env vars using t.Setenv for automatic cleanup.
			t.Setenv("ATMOS_GITHUB_TOKEN", tt.atmosToken)
			t.Setenv("GITHUB_TOKEN", tt.githubToken)

			token := GetGitHubToken()

			if tt.expectEmpty && tt.atmosToken == "" && tt.githubToken == "" {
				// With no env vars set, token comes from gh CLI (if installed) or is empty.
				// Either outcome is valid — we just verify it's not a test fixture value.
				if token != "" {
					assert.NotEqual(t, "atmos_token_123", token, "should not be a test fixture value")
					assert.NotEqual(t, "github_token_456", token, "should not be a test fixture value")
				}
			} else if tt.expectedPrefix != "" {
				assert.Equal(t, tt.expectedPrefix, token)
			}
		})
	}
}

func TestGetGitHubTokenOrError(t *testing.T) {
	t.Run("with token", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_TOKEN", "test_token")
		t.Setenv("GITHUB_TOKEN", "")

		token, err := GetGitHubTokenOrError()
		assert.NoError(t, err)
		assert.Equal(t, "test_token", token)
	})

	t.Run("without token and no gh CLI", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_TOKEN", "")
		t.Setenv("GITHUB_TOKEN", "")
		// Disable the CLI fallback so the result is deterministic regardless of host setup.
		t.Setenv("ATMOS_GITHUB_CLI", "")

		token, err := GetGitHubTokenOrError()
		assert.ErrorIs(t, err, ErrGitHubTokenRequired)
		assert.Empty(t, token)
	})
}

// TestGitHubCLIBinary verifies how the CLI binary name is resolved from ATMOS_GITHUB_CLI.
func TestGitHubCLIBinary(t *testing.T) {
	t.Run("defaults to gh when unset", func(t *testing.T) {
		// t.Setenv cannot unset, so save and restore manually around an explicit unset.
		orig, had := os.LookupEnv("ATMOS_GITHUB_CLI")
		t.Cleanup(func() {
			if had {
				_ = os.Setenv("ATMOS_GITHUB_CLI", orig)
			} else {
				_ = os.Unsetenv("ATMOS_GITHUB_CLI")
			}
		})
		require.NoError(t, os.Unsetenv("ATMOS_GITHUB_CLI"))

		assert.Equal(t, "gh", gitHubCLIBinary())
	})

	t.Run("honors a custom binary name", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "  mygh  ")
		assert.Equal(t, "mygh", gitHubCLIBinary())
	})

	t.Run("empty value disables the fallback", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "")
		assert.Empty(t, gitHubCLIBinary())
	})
}

// TestGetGitHubTokenFromCLI exercises the CLI fallback with a mock commander (no real gh).
func TestGetGitHubTokenFromCLI(t *testing.T) {
	t.Run("returns trimmed token from the default gh binary", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "gh")
		ctrl := gomock.NewController(t)
		mock := execpkg.NewMockCommandExecutor(ctrl)
		mock.EXPECT().
			CommandContext(gomock.Any(), "gh", "auth", "token").
			Return(fakeCLICmd("ghp_from_cli\n", 0))
		withCommander(t, mock)

		assert.Equal(t, "ghp_from_cli", getGitHubTokenFromCLI())
	})

	t.Run("uses the configured binary name", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "mygh")
		ctrl := gomock.NewController(t)
		mock := execpkg.NewMockCommandExecutor(ctrl)
		mock.EXPECT().
			CommandContext(gomock.Any(), "mygh", "auth", "token").
			Return(fakeCLICmd("ghp_custom\n", 0))
		withCommander(t, mock)

		assert.Equal(t, "ghp_custom", getGitHubTokenFromCLI())
	})

	t.Run("returns empty when the CLI exits non-zero", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "gh")
		ctrl := gomock.NewController(t)
		mock := execpkg.NewMockCommandExecutor(ctrl)
		mock.EXPECT().
			CommandContext(gomock.Any(), "gh", "auth", "token").
			Return(fakeCLICmd("", 1))
		withCommander(t, mock)

		assert.Empty(t, getGitHubTokenFromCLI())
	})

	t.Run("empty ATMOS_GITHUB_CLI disables the fallback without invoking the commander", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "")
		ctrl := gomock.NewController(t)
		// No EXPECT: any call to the commander fails the test.
		mock := execpkg.NewMockCommandExecutor(ctrl)
		withCommander(t, mock)

		assert.Empty(t, getGitHubTokenFromCLI())
	})

	t.Run("nonexistent binary forces the anonymous path (real commander)", func(t *testing.T) {
		t.Setenv("ATMOS_GITHUB_CLI", "atmos-nonexistent-gh-binary-xyz")
		// Uses the real default commander; the binary does not exist, so Output errors.
		assert.Empty(t, getGitHubTokenFromCLI())
	})
}

// TestGetGitHubToken_EnvWinsOverCLI verifies an explicit token short-circuits the CLI fallback.
func TestGetGitHubToken_EnvWinsOverCLI(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_TOKEN", "atmos_explicit")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	ctrl := gomock.NewController(t)
	// No EXPECT: the commander must not be invoked when an explicit token is present.
	mock := execpkg.NewMockCommandExecutor(ctrl)
	withCommander(t, mock)

	assert.Equal(t, "atmos_explicit", GetGitHubToken())
}
