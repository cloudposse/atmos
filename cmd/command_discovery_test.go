package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	tuiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestDiscoveryColorFlags(t *testing.T) {
	_ = NewTestKit(t)
	mockDiscoveryDependencies(t)
	previous := ui.GetColorProfile()
	t.Cleanup(func() { ui.SetColorProfile(previous) })
	openDocsURL = func(string) error { return nil }
	for _, tc := range []struct {
		args    string
		noColor string
		colored bool
	}{
		{args: "--force-color", colored: true},
		{args: "--force-color=false"},
		{args: "--force-color --no-color"},
		{args: "--force-color --no-color=false", colored: true},
		{args: "--force-color", noColor: "1"},
	} {
		t.Run(tc.args+tc.noColor, func(t *testing.T) {
			t.Setenv("NO_COLOR", tc.noColor)
			ui.SetColorProfile(termenv.Ascii)
			root, _, output := discoveryTestRoot(t)
			handled, err := dispatchCommandDiscovery(root, strings.Fields("terraform --docs "+tc.args))
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Equal(t, tc.colored, strings.Contains(output.String(), "\x1b["))
			assert.Contains(t, ansi.Strip(output.String()), "Documentation: https://atmos.tools/terraform")
		})
	}
}

func TestCommandExampleConfirmationButtons(t *testing.T) {
	_ = NewTestKit(t)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{}) // No terminal size events, like Cobra's output wrapper.
	confirmed := false
	field := tuiutils.NewAtmosConfirm().Title("Initialize this example?").Affirmative("Yes").Negative("No").Value(&confirmed)
	form := commandExampleForm(cmd, field)
	form.Init()
	view := ansi.Strip(form.View())
	// Check the buttons themselves, not the "y Yes / n No" keyboard help.
	assert.Regexp(t, `Yes\s+No`, view)
	assert.Contains(t, view, "Initialize this example?")
	form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, confirmed, "Enter must decline by default")
}

func discoveryTestRoot(t *testing.T) (*cobra.Command, *cobra.Command, *bytes.Buffer) {
	t.Helper()
	root := &cobra.Command{Use: "atmos"}
	flags.NewGlobalOptionsBuilder().Build().RegisterPersistentFlags(root)
	parent := &cobra.Command{Use: "terraform", Aliases: []string{"tf"}}
	leaf := &cobra.Command{Use: "plan component", Args: cobra.ExactArgs(1)}
	leaf.Flags().StringP("stack", "s", "", "")
	leaf.Flags().String("label", "", "")
	leaf.RunE = func(*cobra.Command, []string) error { t.Fatal("command executed"); return nil }
	leaf.PreRunE = leaf.RunE
	root.PersistentPreRunE = leaf.RunE
	root.AddCommand(parent)
	parent.AddCommand(leaf)
	internal.RegisterCommandMetadata(parent, internal.WithDocumentationURL("https://atmos.tools/terraform"),
		internal.WithExample("examples/quick-start-simple", "Simple"), internal.WithExample("examples/quick-start-advanced", "Advanced"))
	internal.RegisterCommandMetadata(leaf, internal.WithDocumentationURL("https://atmos.tools/terraform/plan"))
	var output bytes.Buffer
	root.SetOut(&output)
	return root, leaf, &output
}

func mockDiscoveryDependencies(t *testing.T) {
	t.Helper()
	oldOpen, oldSelect, oldConfirm, oldInit, oldTTY := openDocsURL, discoverySelect, discoveryConfirm, discoveryInit, discoveryTTY
	t.Cleanup(func() {
		openDocsURL, discoverySelect, discoveryConfirm, discoveryInit, discoveryTTY = oldOpen, oldSelect, oldConfirm, oldInit, oldTTY
	})
	openDocsURL = func(string) error { t.Fatal("unexpected browser"); return nil }
	discoverySelect = func(*cobra.Command, []internal.CommandExample) (int, error) {
		t.Fatal("unexpected picker")
		return 0, nil
	}
	discoveryConfirm = func(*cobra.Command, bool) (bool, error) { t.Fatal("unexpected confirmation"); return false, nil }
	discoveryInit = func(*cobra.Command, []string) error { t.Fatal("unexpected installation"); return nil }
	discoveryTTY = func() bool { return false }
}

func TestDiscoveryFlagParsing(t *testing.T) {
	cases := []struct {
		args                string
		command             string
		docs, example, help bool
		wantError           bool
	}{
		{"terraform plan --docs", "atmos terraform plan", true, false, false, false},
		{"--docs tf plan", "atmos terraform plan", true, false, false, false},
		{"--docs=false terraform plan", "atmos terraform plan", false, false, false, false},
		{"terraform plan --docs false", "atmos terraform plan", false, false, false, false},
		{"terraform plan --docs --docs=false", "atmos terraform plan", false, false, false, false},
		{"terraform plan --example=true", "atmos terraform plan", false, true, false, false},
		{"terraform plan -- --docs", "atmos terraform plan", false, false, false, false},
		{"terraform plan --label --docs", "atmos terraform plan", false, false, false, false},
		{"terraform plan --label=--docs", "atmos terraform plan", false, false, false, false},
		{"terraform plan -s --docs", "atmos terraform plan", false, false, false, false},
		{"--config --docs terraform plan", "atmos terraform plan", false, false, false, false},
		{"--config project.yaml --docs terraform plan", "atmos terraform plan", true, false, false, false},
		{"--pager terraform plan --docs", "atmos terraform plan", true, false, false, false},
		{"--pager less terraform plan --docs", "atmos terraform plan", true, false, false, false},
		{"--cast terraform plan --docs", "atmos terraform plan", true, false, false, false},
		{"-C/tmp terraform plan --docs", "atmos terraform plan", true, false, false, false},
		{"terraform plan --help=usage --docs", "atmos terraform plan", true, false, true, false},
		{"terraform plan --help=false --docs", "atmos terraform plan", true, false, false, false},
		{"help terraform plan --docs", "atmos terraform plan", true, false, true, false},
		{"terraform plan --docs --unknown", "atmos terraform plan", true, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.args, func(t *testing.T) {
			root, _, _ := discoveryTestRoot(t)
			request, err := parseDiscoveryRequest(root, strings.Fields(tc.args))
			assert.Equal(t, tc.wantError, err != nil)
			assert.Equal(t, tc.command, request.command.CommandPath())
			assert.Equal(t, tc.docs, request.docs)
			assert.Equal(t, tc.example, request.example)
			assert.Equal(t, tc.help, request.help)
		})
	}
}

func TestDiscoveryDocs(t *testing.T) {
	mockDiscoveryDependencies(t)
	root, leaf, output := discoveryTestRoot(t)
	// Required arguments and hooks must not run, even with custom flag parsing.
	leaf.Args = func(*cobra.Command, []string) error { t.Fatal("validated execution arguments"); return nil }
	leaf.DisableFlagParsing = true
	var opened string
	openDocsURL = func(url string) error { opened = url; return nil }
	handled, err := dispatchCommandDiscovery(root, []string{"tf", "plan", "--docs"})
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "https://atmos.tools/terraform/plan", opened)
	assert.Contains(t, ansi.Strip(output.String()), "Documentation: "+opened)
	openDocsURL = func(string) error { return errors.New("browser unavailable") }
	_, err = dispatchCommandDiscovery(root, []string{"terraform", "--docs"})
	assert.ErrorContains(t, err, "browser unavailable")
	assert.Contains(t, ansi.Strip(output.String()), "Documentation: https://atmos.tools/terraform")
}

func TestDiscoveryCustomFlagOwnership(t *testing.T) {
	for _, name := range []string{"docs", "example"} {
		for _, owner := range []string{"local", "persistent", "inherited persistent"} {
			for _, position := range []string{"before command", "between commands", "after command", "after separator", "disabled"} {
				t.Run(name+"/"+owner+"/"+position, func(t *testing.T) {
					mockDiscoveryDependencies(t)
					root, leaf, output := discoveryTestRoot(t)
					var enabled bool
					switch owner {
					case "local":
						leaf.Flags().BoolVar(&enabled, name, false, "Custom command option")
					case "persistent":
						leaf.PersistentFlags().BoolVar(&enabled, name, false, "Custom command option")
					case "inherited persistent":
						leaf.Parent().PersistentFlags().BoolVar(&enabled, name, false, "Custom command option")
					}
					var args, expectedArgs []string
					switch position {
					case "before command":
						args = []string{"--" + name, "terraform", "plan"}
					case "between commands":
						args = []string{"terraform", "--" + name, "plan"}
					case "after command":
						args = []string{"terraform", "plan", "--" + name}
					case "after separator":
						args = []string{"terraform", "plan", "--", "--docs", "--example"}
						expectedArgs = []string{"--docs", "--example"}
					case "disabled":
						args = []string{"--" + name + "=false", "terraform", "plan"}
					}
					originalArgs := append([]string(nil), args...)
					root.PersistentPreRunE = nil
					leaf.PreRunE = nil
					leaf.Args = cobra.ArbitraryArgs
					executed := false
					leaf.Run = func(_ *cobra.Command, actualArgs []string) {
						executed = true
						assert.Equal(t, expectedArgs, append([]string(nil), actualArgs...))
					}
					leaf.RunE = nil
					handled, err := dispatchCommandDiscovery(root, args)
					require.NoError(t, err)
					require.False(t, handled)
					assert.Equal(t, originalArgs, args)
					assert.Empty(t, output.String())
					root.SetArgs(args)
					require.NoError(t, root.Execute())
					assert.True(t, executed)
					assert.Equal(t, position != "after separator" && position != "disabled", enabled)
				})
			}
		}
	}
}

func TestDiscoveryParentLocalFlagDoesNotShadowChild(t *testing.T) {
	for _, name := range []string{"docs", "example"} {
		for _, args := range [][]string{
			{"--" + name, "terraform", "plan"},
			{"terraform", "--" + name, "plan"},
			{"terraform", "plan", "--" + name},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				mockDiscoveryDependencies(t)
				root, leaf, output := discoveryTestRoot(t)
				leaf.Parent().Flags().Bool(name, false, "Parent-local option")
				opened := false
				openDocsURL = func(string) error { opened = true; return nil }
				handled, err := dispatchCommandDiscovery(root, args)
				require.NoError(t, err)
				assert.True(t, handled)
				assert.Equal(t, name == "docs", opened)
				assert.Contains(t, output.String(), "Documentation:")
			})
		}
	}
}

func TestDiscoveryCustomFlagAlongsideGlobalDiscovery(t *testing.T) {
	for _, custom := range []string{"docs", "example"} {
		t.Run(custom, func(t *testing.T) {
			mockDiscoveryDependencies(t)
			root, leaf, _ := discoveryTestRoot(t)
			leaf.Flags().Bool(custom, false, "Custom option")
			global := "docs"
			if custom == global {
				global = "example"
			}
			opened := false
			openDocsURL = func(string) error { opened = true; return nil }
			// Discovery bypasses custom flag validation, even before the command.
			handled, err := dispatchCommandDiscovery(root, []string{"--" + custom + "=invalid", "terraform", "plan", "--" + global})
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Equal(t, global == "docs", opened)
		})
	}
}

func TestDiscoveryDispatchBoundaries(t *testing.T) {
	mockDiscoveryDependencies(t)
	root, _, _ := discoveryTestRoot(t)
	for _, args := range [][]string{
		{"terraform", "plan", "--help", "--docs", "--example"},
		{"terraform", "plan", "--docs=false"},
		{"terraform", "plan", "--", "--example"},
		{"terraform", "plan", "--label", "--docs"},
	} {
		handled, err := dispatchCommandDiscovery(root, args)
		require.NoError(t, err)
		assert.False(t, handled)
	}
	for _, args := range [][]string{
		{"terraform", "--docs", "--example"},
		{"terraform", "--docs=invalid"},
		{"terraform", "--example=invalid"},
		{"terraform", "unknown", "--docs"},
		{"unknown", "--example"},
	} {
		handled, err := dispatchCommandDiscovery(root, args)
		assert.True(t, handled)
		assert.Error(t, err)
	}
}

func TestDiscoveryNoninteractiveExamples(t *testing.T) {
	previous := ui.GetColorProfile()
	t.Cleanup(func() { ui.SetColorProfile(previous) })
	t.Setenv("NO_COLOR", "")
	for name, profile := range map[string]termenv.Profile{"plain": termenv.Ascii, "colored": termenv.TrueColor} {
		t.Run(name, func(t *testing.T) {
			mockDiscoveryDependencies(t)
			ui.SetColorProfile(profile)
			root, _, output := discoveryTestRoot(t)
			handled, err := dispatchCommandDiscovery(root, []string{"terraform", "plan", "--example"})
			require.NoError(t, err)
			assert.True(t, handled)
			assert.Contains(t, output.String(), "Simple (recommended)")
			assert.Contains(t, output.String(), "atmos init examples/quick-start-simple ./quick-start-simple")
			assert.Contains(t, output.String(), "atmos init examples/quick-start-advanced ./quick-start-advanced")
			output.Reset()
			_, err = dispatchCommandDiscovery(root, []string{"--example"})
			require.NoError(t, err)
			assert.Contains(t, output.String(), "No examples are registered")
			assert.Contains(t, ansi.Strip(output.String()), "Documentation: https://atmos.tools")
			assert.Equal(t, profile != termenv.Ascii, strings.Contains(output.String(), "\x1b["))
			discoveryTTY = func() bool { return true }
			_, err = dispatchCommandDiscovery(root, []string{"terraform", "--example", "--interactive=false"})
			require.NoError(t, err)
			t.Setenv("ATMOS_INTERACTIVE", "false")
			_, err = dispatchCommandDiscovery(root, []string{"terraform", "--example"})
			require.NoError(t, err)
		})
	}
}

func TestDiscoveryInteractiveExamples(t *testing.T) {
	_ = NewTestKit(t)
	for _, scenario := range []string{"install", "decline", "cancel-picker", "cancel-confirm", "picker-error", "confirm-error", "init-error", "single"} {
		t.Run(scenario, func(t *testing.T) {
			mockDiscoveryDependencies(t)
			root, leaf, output := discoveryTestRoot(t)
			discoveryTTY = func() bool { return true }
			if scenario == "single" {
				internal.RegisterCommandMetadata(leaf, internal.WithExample("examples/quick-start-simple", "Simple"))
			} else {
				discoverySelect = func(_ *cobra.Command, examples []internal.CommandExample) (int, error) {
					require.Len(t, examples, 2)
					switch scenario {
					case "cancel-picker":
						return 0, huh.ErrUserAborted
					case "picker-error":
						return 0, errors.New("picker failed")
					}
					return 1, nil
				}
			}
			discoveryConfirm = func(*cobra.Command, bool) (bool, error) {
				switch scenario {
				case "cancel-confirm":
					return false, huh.ErrUserAborted
				case "confirm-error":
					return false, errors.New("confirm failed")
				}
				return scenario != "decline", nil
			}
			installed := false
			discoveryInit = func(_ *cobra.Command, args []string) error {
				installed = true
				assert.Contains(t, args, "--force=false")
				assert.NotContains(t, args, "--update=false", "copy initialization rejects even an explicitly disabled update flag")
				assert.Contains(t, args, "config.yaml")
				assert.NotContains(t, args, "--example")
				assert.NotContains(t, args, "--stack")
				if scenario == "single" {
					assert.Contains(t, args, "./quick-start-simple")
				} else {
					assert.Contains(t, args, "./quick-start-advanced")
				}
				if scenario == "init-error" {
					return errors.New("destination occupied")
				}
				return nil
			}
			_, err := dispatchCommandDiscovery(root, []string{"--config", "config.yaml", "terraform", "plan", "--example", "--interactive=true", "--stack", "dev"})
			if strings.HasSuffix(scenario, "error") {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, scenario == "install" || scenario == "single" || scenario == "init-error", installed)
			if installed {
				assert.Contains(t, output.String(), "Destination:")
			}
			if scenario == "decline" {
				assert.Contains(t, ansi.Strip(output.String()), "\n\n⚠ Example initialization declined. No files were changed.")
			}
		})
	}
}

func TestDiscoveryExistingDestination(t *testing.T) {
	_ = NewTestKit(t)
	for _, scenario := range []string{"overwrite", "decline", "cancel", "empty", "file", "noninteractive", "init-error"} {
		t.Run(scenario, func(t *testing.T) {
			mockDiscoveryDependencies(t)
			t.Chdir(t.TempDir())
			root, leaf, output := discoveryTestRoot(t)
			internal.RegisterCommandMetadata(leaf, internal.WithExample("examples/quick-start-simple", "Simple"))
			discoveryTTY = func() bool { return scenario != "noninteractive" }
			const target = "quick-start-simple"
			if scenario == "file" {
				require.NoError(t, os.WriteFile(target, []byte("keep"), 0o600))
			} else {
				require.NoError(t, os.Mkdir(target, 0o700))
				if scenario != "empty" {
					require.NoError(t, os.WriteFile(target+"/README.md", []byte("keep"), 0o600))
				}
			}
			discoveryConfirm = func(_ *cobra.Command, overwrite bool) (bool, error) {
				assert.Equal(t, scenario != "empty", overwrite)
				if scenario == "cancel" {
					return false, huh.ErrUserAborted
				}
				return scenario != "decline", nil
			}
			installed := false
			discoveryInit = func(_ *cobra.Command, args []string) error {
				installed = true
				assert.Contains(t, args, "--force="+fmt.Sprint(scenario != "empty"))
				if scenario == "init-error" {
					return errors.New("copy failed")
				}
				return nil
			}
			_, err := dispatchCommandDiscovery(root, []string{"terraform", "plan", "--example"})
			if scenario == "file" || scenario == "init-error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, scenario == "overwrite" || scenario == "empty" || scenario == "init-error", installed)
			if scenario == "decline" {
				assert.Contains(t, ansi.Strip(output.String()), "\n\n⚠ Example initialization declined. No files were changed.")
			}
			if scenario == "cancel" {
				assert.Contains(t, ansi.Strip(output.String()), "\n\n⚠ Example initialization canceled. No files were changed.")
			}
			if scenario == "noninteractive" {
				assert.Contains(t, output.String(), "--force=false")
			}
			if scenario != "file" && scenario != "empty" {
				contents, readErr := os.ReadFile(target + "/README.md")
				require.NoError(t, readErr)
				assert.Equal(t, "keep", string(contents), "discovery itself must never change existing files")
			}
		})
	}
}

func TestInitializeCommandExampleExitStatus(t *testing.T) {
	if os.Getenv("ATMOS_TEST_INIT_EXIT") == "1" {
		fmt.Fprintln(os.Stderr, "Initialization failed")
		os.Exit(2)
	}
	t.Setenv("ATMOS_TEST_INIT_EXIT", "1")
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	err := initializeCommandExample(cmd, []string{"-test.run=^TestInitializeCommandExampleExitStatus$"})
	var exitError errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitError)
	assert.Equal(t, 2, exitError.Code)
	assert.True(t, exitError.Silent)
	assert.Equal(t, "Initialization failed\n", output.String())
}

func TestDiscoveryExecuteWithoutProject(t *testing.T) {
	mockDiscoveryDependencies(t)
	_ = NewTestKit(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: [yaml"), 0o600))
	oldArgs := os.Args
	oldChdir := chdirProcessed
	t.Cleanup(func() { os.Args = oldArgs; chdirProcessed = oldChdir })
	os.Args = []string{"atmos", "terraform", "plan", "--docs"}
	openDocsURL = func(url string) error {
		assert.Equal(t, "https://atmos.tools/cli/commands/terraform/plan", url)
		return nil
	}
	require.NoError(t, Execute())
	require.NoError(t, os.Remove("atmos.yaml"))
	require.NoError(t, Execute())
}
