package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"al.essio.dev/pkg/shellescape"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	tuiutils "github.com/cloudposse/atmos/internal/tui/utils"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/startup"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Dependencies are replaceable so discovery tests never open a browser, prompt,
// or fetch an example. The browser helper is shared with the existing docs command.
var (
	discoverySelect  = selectCommandExample
	discoveryConfirm = confirmCommandExample
	discoveryInit    = initializeCommandExample
	discoveryTTY     = func() bool {
		return terminal.New().IsTTY(terminal.Stdin) && terminal.New().IsTTY(terminal.Stdout) && !telemetry.IsCI()
	}
)

// Unknown paths may name custom commands or configured aliases. Resolve those
// after configuration registration; known built-ins remain config independent.
func dispatchBuiltinCommandDiscovery(root *cobra.Command, args []string) (bool, error) {
	if !hasDiscoveryFlag(args) {
		return false, nil
	}
	request, _ := parseDiscoveryRequest(root, args)
	if len(request.positionals) > 0 && request.command.HasSubCommands() {
		return false, nil
	}
	return dispatchCommandDiscovery(root, args)
}

func dispatchCommandDiscovery(root *cobra.Command, args []string) (bool, error) {
	defer perf.Track(nil, "cmd.dispatchCommandDiscovery")()
	if !hasDiscoveryFlag(args) {
		return false, nil
	}
	request, parseErr := parseDiscoveryRequest(root, args)
	if request.help || (!request.docs && !request.example) {
		return false, nil
	}
	if parseErr != nil {
		return true, discoveryUsageError(request.command, parseErr)
	}
	if err := validateDiscoveryRequest(request); err != nil {
		return true, discoveryUsageError(request.command, err)
	}
	metadata := internal.ResolveCommandMetadata(request.command)
	// Discovery skips Cobra's parsing and startup hooks. Apply its parsed color
	// settings without mutating the original command's flags.
	colorCommand := &cobra.Command{}
	colorCommand.Flags().Bool("no-color", request.noColor, "")
	colorCommand.Flags().Bool("force-color", request.forceColor, "")
	startup.ConfigureEarlyColorProfile(colorCommand)
	printCommandDocumentation(request.command.OutOrStdout(), request.command)
	if request.docs {
		if err := openDocsURL(metadata.DocumentationURL); err != nil {
			return true, fmt.Errorf("could not open documentation: %w", err)
		}
		return true, nil
	}
	return true, discoverCommandExamples(request, metadata.Examples)
}

func validateDiscoveryRequest(request discoveryRequest) error {
	if request.docs && request.example {
		return fmt.Errorf("%w: --docs and --example", errUtils.ErrMutuallyExclusiveFlags)
	}
	// Match help's distinction between unknown subcommands and positional
	// arguments on runnable parents. Missing execution arguments are ignored.
	if len(request.positionals) > 0 && request.command.HasSubCommands() {
		if request.command.Args == nil || request.command.Args(request.command, request.positionals) != nil {
			return fmt.Errorf("%w: %q for %q", errUtils.ErrUnknownSubcommand, request.positionals[0], request.command.CommandPath())
		}
	}
	return nil
}

func discoveryUsageError(cmd *cobra.Command, err error) error {
	return errUtils.Build(err).
		WithHint("Documentation: " + internal.ResolveCommandMetadata(cmd).DocumentationURL).
		WithExitCode(2).Err()
}

func discoveryInteractive(request discoveryRequest) bool {
	if !discoveryTTY() {
		return false
	}
	if request.interactive != nil {
		return *request.interactive
	}
	if value, ok := os.LookupEnv("ATMOS_INTERACTIVE"); ok {
		enabled, err := strconv.ParseBool(value)
		return err == nil && enabled
	}
	return true
}

func discoverCommandExamples(request discoveryRequest, examples []internal.CommandExample) error {
	out := request.command.OutOrStdout()
	if len(examples) == 0 {
		fmt.Fprintln(out, "No examples are registered for this command.")
		return nil
	}
	if !discoveryInteractive(request) {
		for i, example := range examples {
			label := example.Label
			if i == 0 {
				label += " (recommended)"
			}
			fmt.Fprintf(out, "\n%s\n  %s\n", label, exampleInstallCommand(request.globals, example)) // #nosec G705 -- plain terminal text, not HTML.
		}
		return nil
	}
	err := initializeSelectedExample(request, examples)
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}

func initializeSelectedExample(request discoveryRequest, examples []internal.CommandExample) error {
	selected := examples[0]
	if len(examples) > 1 {
		index, err := discoverySelect(request.command, examples)
		if err != nil {
			return err
		}
		selected = examples[index]
	}
	target := exampleTarget(selected)
	absoluteTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	out := request.command.OutOrStdout()
	styles := createHelpStyles(ui.NewRenderer(out))
	// #nosec G705 -- plain terminal text, not HTML.
	fmt.Fprintf(out, "\n  %s %s\n  %s %s\n  %s %s\n\n  %s\n",
		styles.muted.Render("Example:"), styles.heading.Render(selected.Label),
		styles.muted.Render("Source:"), selected.Source,
		styles.muted.Render("Destination:"), absoluteTarget,
		styles.commandName.Bold(false).Render(exampleInstallCommand(request.globals, selected)))
	confirmed, err := discoveryConfirm(request.command)
	if err != nil || !confirmed {
		return err
	}
	return discoveryInit(request.command, exampleInitArgs(request.globals, selected))
}

func exampleTarget(example internal.CommandExample) string {
	return "./" + filepath.Base(example.Source)
}

func exampleInitArgs(globals []string, example internal.CommandExample) []string {
	args := append([]string{}, globals...)
	return append(args, "init", example.Source, exampleTarget(example), "--force=false")
}

func exampleInstallCommand(globals []string, example internal.CommandExample) string {
	return shellescape.QuoteCommand(append([]string{"atmos"}, exampleInitArgs(globals, example)...))
}

func discoveryContext(cmd *cobra.Command) context.Context {
	if cmd.Context() != nil {
		return cmd.Context()
	}
	return context.Background()
}

func selectCommandExample(cmd *cobra.Command, examples []internal.CommandExample) (int, error) {
	selected := 0
	options := make([]huh.Option[int], 0, len(examples))
	for i, example := range examples {
		label := example.Label
		if i == 0 {
			label += " (recommended)"
		}
		options = append(options, huh.NewOption(label, i))
	}
	field := huh.NewSelect[int]().Title("Choose an example").Options(options...).Value(&selected)
	err := commandExampleForm(cmd, field).RunWithContext(discoveryContext(cmd))
	return selected, err
}

func confirmCommandExample(cmd *cobra.Command) (bool, error) {
	confirmed := false
	field := tuiutils.NewAtmosConfirm().Title("Initialize this example?").Affirmative("Yes").Negative("No").Value(&confirmed)
	err := commandExampleForm(cmd, field).RunWithContext(discoveryContext(cmd))
	return confirmed, err
}

func commandExampleForm(cmd *cobra.Command, field huh.Field) *huh.Form {
	// Apply spacing before NewGroup measures the field, including when the output
	// writer cannot report a terminal size (for example a recording wrapper).
	theme := tuiutils.NewAtmosHuhTheme()
	field.WithTheme(theme)
	return huh.NewForm(huh.NewGroup(field)).WithTheme(theme).
		WithKeyMap(tuiutils.NewAtmosKeyMap()).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout())
}

// Use the current executable, never an unrelated Atmos from PATH. A fresh init
// invocation retains its preflight, config, repository, and README behavior and
// cannot inherit the selected command's local flags or execution hooks.
func initializeCommandExample(cmd *cobra.Command, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	child := exec.CommandContext(discoveryContext(cmd), executable, args...) // #nosec G204,G702 -- current executable and separate argv, never a shell.
	// --chdir (including ATMOS_CHDIR) was already applied by the parent.
	child.Env = reexec.FilterChdirEnv(os.Environ())
	child.Stdin, child.Stdout, child.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
	// Init applies its own masking. Preserve terminal handles so its existing
	// fetch spinner and other interactive output are not disabled by exec pipes.
	child.Stdout = iolib.ChildProcessOutput(child.Stdout)
	child.Stderr = iolib.ChildProcessOutput(child.Stderr)
	return child.Run()
}
