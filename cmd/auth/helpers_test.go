package auth

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time guard: a rename of the Keyring/Type schema fields must break the build.
var _ = schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}}

// TestCreateAuthManager_HonorsKeyringTypeMemory reproduces issue #2544:
// `auth.keyring.type: memory` set in atmos.yaml must select the in-memory
// keyring backend. On unpatched code CreateAuthManager calls the no-arg
// credential-store constructor, dropping the config, so the default "system"
// keyring is chosen instead (which hangs on a broken-but-present keyring).
func TestCreateAuthManager_HonorsKeyringTypeMemory(t *testing.T) {
	// Ensure the env var does not mask the config under test (config, not env,
	// must drive the selection here).
	t.Setenv("ATMOS_KEYRING_TYPE", "")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{Type: authTypes.CredentialStoreTypeMemory},
	}

	mgr, err := CreateAuthManager(authConfig, t.TempDir())
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	assert.Equal(t, authTypes.CredentialStoreTypeMemory, mgr.CredentialStoreType(),
		"keyring.type: memory must select the in-memory credential store")
}

// initTestUI initializes the UI formatter for tests and returns a cleanup function.
func initTestUI(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("failed to create io context: %v", err)
	}
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		ui.Reset()
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "expired (negative duration)",
			duration: -1 * time.Hour,
			expected: "expired",
		},
		{
			name:     "zero seconds",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "one minute",
			duration: 1 * time.Minute,
			expected: "1m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "one hour",
			duration: 1 * time.Hour,
			expected: "1h 0m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 15*time.Minute,
			expected: "2h 15m",
		},
		{
			name:     "large duration",
			duration: 48*time.Hour + 30*time.Minute,
			expected: "48h 30m",
		},
		{
			name:     "just under one minute",
			duration: 59 * time.Second,
			expected: "59s",
		},
		{
			name:     "just under one hour",
			duration: 59*time.Minute + 59*time.Second,
			expected: "59m 59s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDisplayAuthSuccess(t *testing.T) {
	initTestUI(t)

	// Create test whoami info.
	expiration := time.Now().Add(1 * time.Hour)
	whoami := &authTypes.WhoamiInfo{
		Provider:   "aws-sso",
		Identity:   "prod-admin",
		Account:    "123456789012",
		Region:     "us-east-1",
		Expiration: &expiration,
	}

	// Should not panic when called with the UI layer initialized.
	assert.NotPanics(t, func() {
		displayAuthSuccess(whoami)
	})
}

func TestDisplayAuthSuccess_MinimalInfo(t *testing.T) {
	initTestUI(t)

	// Create minimal whoami info (no optional fields).
	whoami := &authTypes.WhoamiInfo{
		Provider: "azure",
		Identity: "dev",
	}

	// Should not panic when called with minimal info.
	assert.NotPanics(t, func() {
		displayAuthSuccess(whoami)
	})
}

func TestDisplayAuthSuccess_WithRealm(t *testing.T) {
	initTestUI(t)

	// Create whoami info with realm.
	whoami := &authTypes.WhoamiInfo{
		Realm:       "test-realm",
		RealmSource: "env",
		Provider:    "mock-provider",
		Identity:    "mock-identity",
		Region:      "us-east-1",
	}

	// Should not panic when called with realm info.
	assert.NotPanics(t, func() {
		displayAuthSuccess(whoami)
	})
}

func TestBuildConfigAndStacksInfo(t *testing.T) {
	// Create a test command.
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create a fresh viper instance.
	v := viper.New()

	// Call the function.
	result := BuildConfigAndStacksInfo(cmd, v)

	// Verify it returns a valid ConfigAndStacksInfo struct.
	// The struct should be empty since we haven't set any flags.
	assert.NotNil(t, result)
}

// newTestCommandWithGlobalFlags creates a test command with all global flags registered.
// This mirrors the pattern used in production where commands inherit global flags from RootCmd.
func newTestCommandWithGlobalFlags(use string) *cobra.Command {
	cmd, _ := newTestCommandWithGlobalParser(use)
	return cmd
}

// newTestCommandWithGlobalParser is the parser-returning variant used by tests
// that need to drive the real Cobra → Viper binding path (e.g. regression
// tests for the --profile flag — issue #1973). Production-equivalent flow:
//
//	cmd, parser := newTestCommandWithGlobalParser("auth")
//	cmd.Flags().Set("profile", "devops")
//	parser.BindFlagsToViper(cmd, v)
//	info := BuildConfigAndStacksInfo(cmd, v)
func newTestCommandWithGlobalParser(use string) (*cobra.Command, *flags.StandardParser) {
	cmd := &cobra.Command{Use: use}
	globalParser := flags.NewGlobalOptionsBuilder().Build()
	globalParser.RegisterPersistentFlags(cmd)
	return cmd, globalParser
}

// runProfileFlagAppliedRegressionTest is the shared regression-test driver
// for issue #1973 (`--profile` global flag silently dropped on `auth exec`
// and `auth shell`). The exec_test and shell_test thin wrappers feed the
// same table into this helper so any future profile-precedence regression
// is caught at both surfaces with a single set of cases.
//
// The commandName is just the cosmetic cobra `Use` field used to construct
// the stand-in command — the actual command code under test is the shared
// BuildConfigAndStacksInfo helper which both `auth exec` and `auth shell`
// route through.
func runProfileFlagAppliedRegressionTest(t *testing.T, commandName string) {
	t.Helper()

	tests := []struct {
		name             string
		cliArgs          []string
		expectedProfiles []string
	}{
		{
			name:             "single profile via --profile flag",
			cliArgs:          []string{"--profile=devops"},
			expectedProfiles: []string{"devops"},
		},
		{
			name:             "multiple profiles",
			cliArgs:          []string{"--profile=devops", "--profile=platform"},
			expectedProfiles: []string{"devops", "platform"},
		},
		{
			name:             "no profile",
			cliArgs:          nil,
			expectedProfiles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Drive the real Cobra → Viper path that executeAuthExecCommand /
			// executeAuthShellCommand use in production:
			//   1. Register --profile as a persistent flag on the command.
			//   2. ParseFlags() with realistic CLI args — Cobra merges
			//      persistent flags into cmd.Flags() during parse, which the
			//      next step depends on.
			//   3. BindFlagsToViper plumbs cmd flags → viper.
			//   4. BuildConfigAndStacksInfo extracts the profile value.
			cmd, globalParser := newTestCommandWithGlobalParser(commandName)
			v := viper.New()

			// ParseFlags must run before BindFlagsToViper. Without it, the
			// parser's `cmd.Flags().Lookup("profile")` returns nil for the
			// persistent flag and viper sees only the default. This mirrors
			// production: Cobra parses CLI args before invoking RunE.
			if err := cmd.ParseFlags(tt.cliArgs); err != nil {
				t.Fatalf("ParseFlags(%v): %v", tt.cliArgs, err)
			}
			if err := globalParser.BindFlagsToViper(cmd, v); err != nil {
				t.Fatalf("BindFlagsToViper: %v", err)
			}

			info := BuildConfigAndStacksInfo(cmd, v)

			if tt.expectedProfiles == nil {
				// `viper.GetStringSlice` returns [] (not nil) for an unset
				// slice with a default of []. Accept either as "no profile";
				// downstream consumers only ever check len().
				assert.Empty(t, info.ProfilesFromArg,
					"no --profile flag must surface as an empty/nil slice")
				return
			}
			assert.Equal(t, tt.expectedProfiles, info.ProfilesFromArg,
				"--profile flag must reach ConfigAndStacksInfo (issue #1973)")
		})
	}
}

// resetAuthCmdFlags snapshots the cobra parser state of the given
// package-level command (auth*Cmd) and registers cleanup that restores every
// flag's Value + Changed flag to its pre-test state. Tests that call
// cmd.ParseFlags() on a shared *cobra.Command must use this so subsequent
// tests don't see leaked flag values (Cobra does not auto-reset between
// ParseFlags calls).
//
// This is the cmd/auth-local equivalent of cmd.NewTestKit — which lives in
// the cmd package and isn't reachable from here without a circular import.
//
// Usage:
//
//	cmd := authLogoutCmd
//	resetAuthCmdFlags(t, cmd)
//	require.NoError(t, cmd.ParseFlags([]string{"--dry-run"}))
func resetAuthCmdFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	type flagSnapshot struct {
		value   string
		changed bool
	}
	snapshot := map[string]flagSnapshot{}
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		snapshot[f.Name] = flagSnapshot{value: f.Value.String(), changed: f.Changed}
	})

	t.Cleanup(func() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			snap, ok := snapshot[f.Name]
			if !ok {
				return
			}
			// Best-effort restore: Set may fail for some custom Value types,
			// but in that case we still reset Changed which is the more
			// important pollution vector.
			_ = f.Value.Set(snap.value)
			f.Changed = snap.changed
		})
	})
}

// captureStdout redirects os.Stdout to an os.Pipe for the duration of the
// caller's test and returns a closure that flushes the write end and reads
// everything written. Cleanup is registered via t.Cleanup so if an
// intervening require.* aborts the test, os.Stdout is still restored and
// both pipe handles are closed — without this, a failing test bleeds its
// captured stdout into every subsequent test in the same package.
//
// Usage:
//
//	read := captureStdout(t)
//	doSomethingThatPrintsToStdout()
//	got := read()
func captureStdout(t *testing.T) func() string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("captureStdout: os.Pipe: %v", err)
	}
	os.Stdout = w

	// Guarantee restoration even if the test aborts mid-flight.
	restored := false
	t.Cleanup(func() {
		if !restored {
			os.Stdout = oldStdout
			_ = w.Close()
		}
		_ = r.Close()
	})

	return func() string {
		if err := w.Close(); err != nil {
			t.Fatalf("captureStdout: pipe close: %v", err)
		}
		os.Stdout = oldStdout
		restored = true

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r); err != nil {
			t.Fatalf("captureStdout: copy from pipe: %v", err)
		}
		return buf.String()
	}
}

// setupMockAuthFixture writes a minimal atmos.yaml wired to the mock/aws
// provider into a tempdir and isolates keyring + XDG state to that dir.
// Tests use this to exercise auth orchestrators end-to-end without touching
// the host's real keyring or atmos.yaml. The function also chdirs into the
// fixture and resets viper so subsequent calls in the test see clean state.
func setupMockAuthFixture(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()

	// Isolate keyring + XDG to the tempdir.
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", tmp)

	atmosYaml := `base_path: "./"
stacks:
  base_path: stacks
  included_paths: ["**/*"]
  name_pattern: "{stage}"
auth:
  providers:
    mock-provider:
      kind: mock/aws
      config:
        description: "Mock AWS provider for testing"
  identities:
    mock-identity:
      kind: mock/aws
      default: true
      via:
        provider: mock-provider
      principal:
        account:
          id: "123456789012"
logs:
  level: Info
`
	if err := os.WriteFile(filepath.Join(tmp, "atmos.yaml"), []byte(atmosYaml), 0o644); err != nil {
		t.Fatalf("setupMockAuthFixture: write atmos.yaml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "stacks"), 0o755); err != nil {
		t.Fatalf("setupMockAuthFixture: mkdir stacks: %v", err)
	}

	t.Chdir(tmp)

	// Reset viper since auth tests share the global instance.
	viper.Reset()
	t.Cleanup(viper.Reset)
}

// TestBuildConfigAndStacksInfo_ProfileFlag tests that the --profile global flag
// is properly extracted into ProfilesFromArg. This is the fix for issue #1973 where
// --profile didn't work with auth exec and auth shell commands.
func TestBuildConfigAndStacksInfo_ProfileFlag(t *testing.T) {
	tests := []struct {
		name             string
		profiles         []string
		expectedProfiles []string
	}{
		{
			name:             "single profile",
			profiles:         []string{"dev"},
			expectedProfiles: []string{"dev"},
		},
		{
			name:             "multiple profiles",
			profiles:         []string{"dev", "staging"},
			expectedProfiles: []string{"dev", "staging"},
		},
		{
			name:             "no profile",
			profiles:         nil,
			expectedProfiles: nil,
		},
		{
			name:             "profile with special characters",
			profiles:         []string{"us-east-1/prod"},
			expectedProfiles: []string{"us-east-1/prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a command with global flags registered (including --profile).
			cmd := newTestCommandWithGlobalFlags("test")

			// Create a fresh Viper instance and set profile values.
			v := viper.New()
			if tt.profiles != nil {
				v.Set("profile", tt.profiles)
			}

			// Call BuildConfigAndStacksInfo which should extract the profile flag.
			result := BuildConfigAndStacksInfo(cmd, v)

			// Verify ProfilesFromArg contains the expected values.
			assert.Equal(t, tt.expectedProfiles, result.ProfilesFromArg,
				"BuildConfigAndStacksInfo should extract --profile flag into ProfilesFromArg")
		})
	}
}

// TestBuildConfigAndStacksInfo_ProfileFlag_EnvironmentVariable tests that ATMOS_PROFILE
// environment variable is properly extracted. This verifies the workaround mentioned
// in issue #1973 continues to work.
func TestBuildConfigAndStacksInfo_ProfileFlag_EnvironmentVariable(t *testing.T) {
	// Set environment variable.
	t.Setenv("ATMOS_PROFILE", "env-profile")

	// Create command with global flags.
	cmd := newTestCommandWithGlobalFlags("test")

	// Create Viper and bind to the global parser (which includes ATMOS_PROFILE binding).
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvPrefix("ATMOS")
	_ = v.BindEnv("profile", "ATMOS_PROFILE")

	// Call BuildConfigAndStacksInfo.
	result := BuildConfigAndStacksInfo(cmd, v)

	// Verify environment variable was picked up.
	assert.Equal(t, []string{"env-profile"}, result.ProfilesFromArg,
		"BuildConfigAndStacksInfo should extract ATMOS_PROFILE env var into ProfilesFromArg")
}

func TestCreateAuthManager(t *testing.T) {
	// Test with nil auth config - should return error or handle gracefully.
	// This tests that the function doesn't panic with minimal input.
	t.Run("with empty auth config", func(t *testing.T) {
		// Note: CreateAuthManager requires valid config to work.
		// We're testing that it handles the call without panicking.
		// A nil config will likely return an error.
		manager, err := CreateAuthManager(nil, "")
		// With nil config, we expect an error.
		assert.Error(t, err)
		assert.Nil(t, manager)
	})
}

func TestHandleHelpRequest(t *testing.T) {
	// Note: handleHelpRequest calls os.Exit(0) when help is requested,
	// so we can only test the case where help is NOT requested.
	t.Run("no help flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		args := []string{"some", "args"}

		// Should not panic or exit.
		assert.NotPanics(t, func() {
			handleHelpRequest(cmd, args)
		})
	})

	t.Run("empty args", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		args := []string{}

		// Should not panic or exit.
		assert.NotPanics(t, func() {
			handleHelpRequest(cmd, args)
		})
	})
}
