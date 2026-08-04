package git

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// The GitHub CI provider is already blank-imported by clone.go, registering
	// it for ci.Detect() package-wide.
)

// newBootstrapTestCommand builds a lightweight "git clone" command tree
// (mirroring the real gitCmd/cloneCmd parent/name relationship) with the same
// --ci/--all bool flags the real clone command registers, then parses rawArgs
// through it exactly as Cobra would before invoking PersistentPreRun/RunE.
// Returns the clone leaf and the resulting positional args.
func newBootstrapTestCommand(t *testing.T, rawArgs []string) (*cobra.Command, []string) {
	t.Helper()

	root := &cobra.Command{Use: "atmos"}
	git := &cobra.Command{Use: gitCmd.Name()}
	clone := &cobra.Command{Use: cloneCmd.Name()}
	clone.Flags().Bool(flagCI, false, "")
	clone.Flags().Bool(flagAll, false, "")
	root.AddCommand(git)
	git.AddCommand(clone)

	require.NoError(t, clone.ParseFlags(rawArgs))
	return clone, clone.Flags().Args()
}

func withCleanATMOSCIEnv(t *testing.T, value string) {
	t.Helper()
	if value == "" {
		original, wasSet := os.LookupEnv("ATMOS_CI")
		os.Unsetenv("ATMOS_CI")
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv("ATMOS_CI", original)
				return
			}
			os.Unsetenv("ATMOS_CI")
		})
		return
	}
	t.Setenv("ATMOS_CI", value)
}

func TestCICloneBootstrapRequested(t *testing.T) {
	tests := []struct {
		name        string
		rawArgs     []string
		ciDetected  bool
		atmosCIEnv  string
		wantRequest bool
	}{
		{
			name:        "no CI provider detected",
			rawArgs:     nil,
			ciDetected:  false,
			wantRequest: false,
		},
		{
			name:        "CI detected, no args, auto mode",
			rawArgs:     nil,
			ciDetected:  true,
			wantRequest: true,
		},
		{
			name:        "explicit --ci=false opts out",
			rawArgs:     []string{"--ci=false"},
			ciDetected:  true,
			wantRequest: false,
		},
		{
			name:        "ATMOS_CI=false opts out",
			rawArgs:     nil,
			ciDetected:  true,
			atmosCIEnv:  "false",
			wantRequest: false,
		},
		{
			name:        "ATMOS_CI uppercase TRUE is accepted (no drift vs pflag ParseBool)",
			rawArgs:     nil,
			ciDetected:  true,
			atmosCIEnv:  "TRUE",
			wantRequest: true,
		},
		{
			name:        "positional argument disqualifies bootstrap",
			rawArgs:     []string{"my-repo"},
			ciDetected:  true,
			wantRequest: false,
		},
		{
			name:        "native args after -- disqualify bootstrap",
			rawArgs:     []string{"--", "--no-tags"},
			ciDetected:  true,
			wantRequest: false,
		},
		{
			name:        "--all disqualifies bootstrap",
			rawArgs:     []string{"--all"},
			ciDetected:  true,
			wantRequest: false,
		},
		{
			name:        "invalid ATMOS_CI value defers to auto (RunE reports the real error)",
			rawArgs:     nil,
			ciDetected:  true,
			atmosCIEnv:  "sometimes",
			wantRequest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ciDetected {
				t.Setenv("GITHUB_ACTIONS", "true")
			} else {
				t.Setenv("GITHUB_ACTIONS", "false")
			}
			withCleanATMOSCIEnv(t, tt.atmosCIEnv)

			cmd, args := newBootstrapTestCommand(t, tt.rawArgs)
			assert.Equal(t, tt.wantRequest, CICloneBootstrapRequested(cmd, args))
		})
	}
}

func TestCICloneBootstrapRequested_WrongCommand(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	withCleanATMOSCIEnv(t, "")

	root := &cobra.Command{Use: "atmos"}
	terraform := &cobra.Command{Use: "terraform"}
	plan := &cobra.Command{Use: "plan"}
	root.AddCommand(terraform)
	terraform.AddCommand(plan)

	assert.False(t, CICloneBootstrapRequested(plan, nil))
	assert.False(t, CICloneBootstrapRequested(nil, nil))
}
