package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestStripHelpTokens verifies stripHelpTokens removes "--help", "-h", and the
// bare "help" subcommand keyword, leaving every other token untouched and in
// order.
func TestStripHelpTokens(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nil", in: nil, want: []string{}},
		{name: "empty", in: []string{}, want: []string{}},
		{name: "only --help", in: []string{"--help"}, want: []string{}},
		{name: "only -h", in: []string{"-h"}, want: []string{}},
		{name: "only bare help", in: []string{"help"}, want: []string{}},
		{name: "unknown subcommand with --help", in: []string{"versions", "--help"}, want: []string{"versions"}},
		{name: "unknown subcommand with -h", in: []string{"versions", "-h"}, want: []string{"versions"}},
		{name: "help tokens interspersed", in: []string{"--help", "versions", "-h"}, want: []string{"versions"}},
		{name: "no help tokens", in: []string{"component1", "-s", "dev"}, want: []string{"component1", "-s", "dev"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHelpTokens(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRejectUnknownSubcommandForHelp exercises the new guard in isolation
// against small synthetic command trees, mirroring the direct-call pattern
// used by TestUsageErrorHelpersExit and TestShowArgCountErrorAndExit_MessageContent
// elsewhere in this package. This is the core logic added to fix the bug where
// "atmos <parent> <bogus-subcommand> --help" silently rendered the parent's
// help instead of reporting the unknown subcommand.
func TestRejectUnknownSubcommandForHelp(t *testing.T) {
	newParentWithChildren := func() *cobra.Command {
		parent := &cobra.Command{Use: "toolchain", Args: cobra.NoArgs}
		install := &cobra.Command{Use: "install", Run: func(*cobra.Command, []string) {}}
		list := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
		parent.AddCommand(install, list)
		return parent
	}

	tests := []struct {
		name           string
		command        func() *cobra.Command
		arguments      []string
		wantRejected   bool
		wantErrSubstrs []string // only checked when wantRejected; each must appear somewhere in the output
	}{
		{
			name:           "unknown subcommand token errors",
			command:        newParentWithChildren,
			arguments:      []string{"versions"},
			wantRejected:   true,
			wantErrSubstrs: []string{"Unknown command", "versions", "atmos toolchain"},
		},
		{
			name:           "unknown subcommand token with help tokens mixed in still errors",
			command:        newParentWithChildren,
			arguments:      []string{"--help", "versions", "-h"},
			wantRejected:   true,
			wantErrSubstrs: []string{"Unknown command", "versions", "atmos toolchain"},
		},
		{
			name:         "no leftover arguments never rejects (bare parent --help)",
			command:      newParentWithChildren,
			arguments:    []string{},
			wantRejected: false,
		},
		{
			name:         "only help tokens strip down to empty and never reject",
			command:      newParentWithChildren,
			arguments:    []string{"--help"},
			wantRejected: false,
		},
		{
			name: "leaf command with no subcommands never rejects its own positional args",
			command: func() *cobra.Command {
				return &cobra.Command{Use: "plan", Run: func(*cobra.Command, []string) {}}
			},
			arguments:    []string{"component1"},
			wantRejected: false,
		},
		{
			name: "arguments that validate cleanly against the command's own Args validator never reject",
			command: func() *cobra.Command {
				parent := &cobra.Command{
					Use: "describe",
					Args: func(*cobra.Command, []string) error {
						return nil // Pretend every argument is a legitimate positional value.
					},
				}
				parent.AddCommand(&cobra.Command{Use: "component", Run: func(*cobra.Command, []string) {}})
				return parent
			},
			arguments:    []string{"component1"},
			wantRejected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Root command a parent CommandPath.
			root := &cobra.Command{Use: rootCommandName}
			command := tt.command()
			root.AddCommand(command)

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			exitCalled := false
			exitCode := 0
			errUtils.OsExit = func(code int) {
				exitCalled = true
				exitCode = code
				panic("exit")
			}

			oldStderr := os.Stderr
			r, w, pipeErr := os.Pipe()
			require.NoError(t, pipeErr)
			os.Stderr = w
			t.Cleanup(func() { os.Stderr = oldStderr })

			var rejected bool
			runFn := func() {
				rejected = rejectUnknownSubcommandForHelp(command, tt.arguments)
			}

			if tt.wantRejected {
				assert.Panics(t, runFn)
			} else {
				assert.NotPanics(t, runFn)
			}

			require.NoError(t, w.Close())
			os.Stderr = oldStderr
			var output bytes.Buffer
			_, copyErr := io.Copy(&output, r)
			require.NoError(t, copyErr)

			assert.Equal(t, tt.wantRejected, rejected || exitCalled,
				"rejectUnknownSubcommandForHelp return value and OsExit invocation must agree")

			if tt.wantRejected {
				assert.Equal(t, 1, exitCode)
				for _, want := range tt.wantErrSubstrs {
					assert.Contains(t, output.String(), want)
				}
			} else {
				assert.False(t, exitCalled, "must not exit when arguments resolve cleanly")
			}
		})
	}
}

// findChildCommand returns the direct child of parent with the given name, or
// nil if none exists. Used to reach into the REAL RootCmd tree (toolchain,
// terraform, version) for the integration-level tests below, which exercise
// the actual bug scenarios reported: "atmos <parent> <bogus-subcommand>
// --help" against the real, fully-registered command tree (including the
// registry's default cobra.NoArgs validator applied to top-level commands).
func findChildCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// TestRootHelpFunc_RealTree_UnknownSubcommandErrors is the regression test for
// the reported bug: "atmos <parent> <bogus-subcommand> --help" used to exit 0
// and silently render the PARENT's help (because rootHelpFunc's
// isHelpRequested short-circuit ran before any unknown-subcommand check).
// It must now report the same "Unknown command" error and exit 1 that the
// equivalent non-help invocation already reports, across multiple, unrelated
// command trees (toolchain, terraform, version) since this fix is
// cross-cutting.
func TestRootHelpFunc_RealTree_UnknownSubcommandErrors(t *testing.T) {
	tests := []struct {
		name           string
		parentName     string
		bogus          string
		wantErrSubstrs []string // each must appear somewhere in the output, in any rendering mode
	}{
		{
			name:           "toolchain versions --help",
			parentName:     "toolchain",
			bogus:          "versions",
			wantErrSubstrs: []string{"Unknown command", "versions", "atmos toolchain"},
		},
		{
			name:           "terraform bogus-subcommand --help",
			parentName:     "terraform",
			bogus:          "bogus-subcommand",
			wantErrSubstrs: []string{"Unknown command", "bogus-subcommand", "atmos terraform"},
		},
		{
			name:           "version bogus-subcommand --help",
			parentName:     "version",
			bogus:          "bogus-subcommand",
			wantErrSubstrs: []string{"Unknown command", "bogus-subcommand", "atmos version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t) // Restores RootCmd state and os.Args after the test.
			saveRestoreAtmosConfig(t)

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			exitCalled := false
			exitCode := 0
			errUtils.OsExit = func(code int) {
				exitCalled = true
				exitCode = code
				panic("exit")
			}

			parentCmd := findChildCommand(RootCmd, tt.parentName)
			require.NotNilf(t, parentCmd, "RootCmd must have a %q subcommand registered", tt.parentName)

			oldStderr := os.Stderr
			r, w, pipeErr := os.Pipe()
			require.NoError(t, pipeErr)
			os.Stderr = w
			t.Cleanup(func() { os.Stderr = oldStderr })

			args := []string{tt.parentName, tt.bogus, "--help"}
			os.Args = append([]string{"atmos"}, args...)
			RootCmd.SetArgs(args)

			// Drive real Cobra dispatch (Find -> ParseFlags -> HelpFunc) instead of
			// calling rootHelpFunc directly, so cmd.Flags().Args() is populated
			// exactly like it is for a real invocation. rootHelpFunc's arguments
			// come from flags.GetActualArgs, which reads cmd.Flags().Args() first.
			assert.Panics(t, func() {
				_, _ = RootCmd.ExecuteC()
			})

			require.NoError(t, w.Close())
			os.Stderr = oldStderr
			var output bytes.Buffer
			_, copyErr := io.Copy(&output, r)
			require.NoError(t, copyErr)

			assert.True(t, exitCalled, "unknown subcommand + --help must call OsExit")
			assert.Equal(t, 1, exitCode)
			for _, want := range tt.wantErrSubstrs {
				assert.Contains(t, output.String(), want)
			}
			assert.Contains(t, output.String(), "Valid subcommands are")
		})
	}
}

// TestRootHelpFunc_RealTree_ValidCasesStillRenderHelp locks in the required
// non-regression behavior alongside the fix above: genuinely valid help
// invocations against the real command tree -- a parent with no subcommand at
// all, and a parent plus a REAL registered subcommand -- must never be
// rejected by the new unknown-subcommand guard (rootHelpFunc must proceed to
// render help normally, i.e. errUtils.OsExit must never be invoked).
func TestRootHelpFunc_RealTree_ValidCasesStillRenderHelp(t *testing.T) {
	tests := []struct {
		name       string
		parentName string
		childName  string // empty means "no subcommand at all".
	}{
		{name: "toolchain --help (no subcommand)", parentName: "toolchain"},
		{name: "toolchain install --help (real subcommand)", parentName: "toolchain", childName: "install"},
		{name: "terraform --help (no subcommand)", parentName: "terraform"},
		{name: "terraform plan --help (real subcommand with its own flags)", parentName: "terraform", childName: "plan"},
		{name: "version --help (no subcommand)", parentName: "version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			saveRestoreAtmosConfig(t)
			// Avoid InitCliConfig filesystem/network lookups during help rendering
			// (see help_template.go's applyColoredHelpTemplateForTopic): a non-empty
			// BasePath makes it reuse the in-memory atmosConfig instead.
			atmosConfig.BasePath = t.TempDir()

			originalOsExit := errUtils.OsExit
			t.Cleanup(func() { errUtils.OsExit = originalOsExit })
			exitCalled := false
			errUtils.OsExit = func(code int) {
				exitCalled = true
				panic("exit")
			}

			target := findChildCommand(RootCmd, tt.parentName)
			require.NotNilf(t, target, "RootCmd must have a %q subcommand registered", tt.parentName)

			args := []string{tt.parentName}
			if tt.childName != "" {
				child := findChildCommand(target, tt.childName)
				require.NotNilf(t, child, "%q must have a %q subcommand registered", tt.parentName, tt.childName)
				args = append(args, tt.childName)
			}
			args = append(args, "--help")
			os.Args = append([]string{"atmos"}, args...)
			RootCmd.SetArgs(args)

			// Drive real Cobra dispatch (Find -> ParseFlags -> HelpFunc), same as
			// the error-path test above, so cmd.Flags().Args() is populated exactly
			// like a real invocation. Discard real rendered help output; RootCmd's
			// writer is pinned to the process's original stdout at package init
			// (see initCobraConfig), so os.Stdout redirection here would not
			// capture it anyway. Only the absence of an unknown-subcommand
			// rejection is under test.
			assert.NotPanics(t, func() {
				_, _ = RootCmd.ExecuteC()
			})
			assert.False(t, exitCalled, "a valid help invocation must never call OsExit")
		})
	}
}
