package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ExecuteOptions configures streaming execution.
type ExecuteOptions struct {
	Command    string   // terraform or opentofu binary.
	Args       []string // Command arguments (plan, apply, etc.).
	WorkingDir string   // Component directory.
	Env        []string // Environment variables.
	Component  string   // Component name for display.
	Stack      string   // Stack name for display.
	SubCommand string   // "plan", "apply", "init", "refresh", "workspace".
	Workspace  string   // Workspace name (for workspace select/new).
	DryRun     bool     // If true, don't execute.
}

// Execute runs a terraform command with streaming UI output.
// Returns an error with exit code preserved via errUtils.ExitCodeError.
func Execute(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.Execute")()

	// Check TTY availability.
	if !term.IsTTYSupportForStdout() {
		return errUtils.ErrStreamingNotSupported
	}

	// Check if in CI environment.
	if telemetry.IsCI() {
		return errUtils.ErrStreamingNotSupported
	}

	if opts.DryRun {
		return nil
	}

	// Build args with -json flag.
	args := buildArgsWithJSON(opts.Args, opts.SubCommand)

	// Create command.
	cmd := exec.CommandContext(ctx, opts.Command, args...)
	cmd.Dir = opts.WorkingDir
	cmd.Env = opts.Env

	// Get stdout pipe for streaming.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrStdoutPipe, err)
	}

	// Suppress stderr since we parse JSON diagnostics from stdout.
	// Terraform outputs human-readable warnings to stderr even with -json flag,
	// but we route those through the Atmos logger via parsed JSON diagnostics.
	cmd.Stderr = io.Discard

	// Start command.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCommandStart, err)
	}

	// Create and run TUI model.
	model := NewModel(opts.Component, opts.Stack, opts.SubCommand, stdout)

	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	// Run TUI - this blocks until completion.
	finalModel, err := p.Run()
	if err != nil {
		// Kill the process if TUI failed.
		_ = cmd.Process.Kill()
		return fmt.Errorf("%w: %w", errUtils.ErrTUIRun, err)
	}

	// Wait for command to finish.
	cmdErr := cmd.Wait()

	// Get exit code.
	var exitCode int
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return cmdErr
		}
	}

	// Check if model has an error.
	m := finalModel.(Model)

	// Log diagnostics after TUI completes (warnings appear after completion message).
	m.LogDiagnostics()

	if m.GetError() != nil {
		return m.GetError()
	}

	// If model captured an exit code, use it.
	if m.GetExitCode() != 0 {
		exitCode = m.GetExitCode()
	}

	// Return exit code error if non-zero.
	if exitCode != 0 {
		return errUtils.ExitCodeError{Code: exitCode}
	}

	// Display outputs after successful apply.
	if opts.SubCommand == "apply" && exitCode == 0 {
		displayOutputs(m.GetTracker())
	}

	return nil
}

// buildArgsWithJSON adds the -json and -compact-warnings flags to the arguments.
// -json enables structured JSON output for parsing.
// -compact-warnings suppresses verbose human-readable warnings since we route
// diagnostics through the Atmos logger via parsed JSON.
func buildArgsWithJSON(args []string, subCommand string) []string {
	// Check if -json is already present.
	hasJSON := false
	hasCompactWarnings := false
	for _, arg := range args {
		if arg == "-json" || strings.HasPrefix(arg, "-json=") {
			hasJSON = true
		}
		if arg == "-compact-warnings" {
			hasCompactWarnings = true
		}
	}

	// If both flags are already present, return as-is.
	if hasJSON && hasCompactWarnings {
		return args
	}

	// Find the position to insert flags (after the subcommand).
	// Let append handle capacity growth to avoid integer overflow concerns.
	var result []string

	for i, arg := range args {
		result = append(result, arg)
		// Insert flags after the subcommand (plan, apply, init, refresh).
		if i == 0 && (arg == "plan" || arg == "apply" || arg == "init" || arg == "refresh") {
			if !hasJSON {
				result = append(result, "-json")
			}
			if !hasCompactWarnings {
				result = append(result, "-compact-warnings")
			}
		}
	}

	// If no subcommand was found at position 0, just prepend flags.
	if len(result) == len(args) {
		var flags []string
		if !hasJSON {
			flags = append(flags, "-json")
		}
		if !hasCompactWarnings {
			flags = append(flags, "-compact-warnings")
		}
		result = append(flags, args...)
	}

	return result
}

// ExecutePlan runs terraform plan with streaming UI and displays the dependency tree.
// It generates a temp planfile to parse for the tree, then cleans it up.
func ExecutePlan(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecutePlan")()

	// Check if user already specified -out flag - if so, use their planfile.
	userPlanFile := extractOutFlag(opts.Args)
	if userPlanFile != "" {
		// User specified their own planfile, run normally then show tree.
		if err := Execute(ctx, opts); err != nil {
			return err
		}

		// Display tree from user's planfile with badge summary.
		tree, treeErr := BuildDependencyTree(ctx, userPlanFile, opts.Command, opts.WorkingDir, opts.Stack, opts.Component)
		if treeErr == nil {
			add, change, remove := tree.GetChangeSummary()
			// Only render tree if there are changes; otherwise just show badge.
			if add > 0 || change > 0 || remove > 0 {
				_ = ui.Writef("\n%s", tree.RenderTree())
			}
			_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
		}

		return nil
	}

	// Generate temp planfile path.
	planFile := filepath.Join(opts.WorkingDir, fmt.Sprintf(".atmos-plan-%d.tfplan", time.Now().UnixNano()))

	// Add -out flag to args.
	planOpts := *opts
	planOpts.Args = append(opts.Args, "-out="+planFile)

	// Run plan with TUI.
	if err := Execute(ctx, &planOpts); err != nil {
		// Clean up planfile on error.
		os.Remove(planFile)
		return err
	}

	// Display tree from planfile with badge summary.
	tree, treeErr := BuildDependencyTree(ctx, planFile, opts.Command, opts.WorkingDir, opts.Stack, opts.Component)
	if treeErr == nil {
		add, change, remove := tree.GetChangeSummary()
		// Only render tree if there are changes; otherwise just show badge.
		if add > 0 || change > 0 || remove > 0 {
			_ = ui.Writef("\n%s", tree.RenderTree())
		}
		_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
	}

	// Clean up temp planfile.
	os.Remove(planFile)

	return nil
}

// ExecuteInit runs terraform init with a spinner TUI that captures output.
// The output is shown in a viewport that clears when init completes.
func ExecuteInit(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteInit")()

	// Check TTY availability.
	if !term.IsTTYSupportForStdout() {
		return errUtils.ErrStreamingNotSupported
	}

	// Check if in CI environment.
	if telemetry.IsCI() {
		return errUtils.ErrStreamingNotSupported
	}

	if opts.DryRun {
		return nil
	}

	// Create command (no -json flag for init).
	cmd := exec.CommandContext(ctx, opts.Command, opts.Args...)
	cmd.Dir = opts.WorkingDir
	cmd.Env = opts.Env

	// Use a pipe to merge stdout and stderr into a single stream.
	// This ensures all terraform output is captured by the TUI.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	// Start command.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCommandStart, err)
	}

	// Capture exit code from the goroutine.
	var cmdErr error
	cmdDone := make(chan struct{})
	go func() {
		cmdErr = cmd.Wait()
		pw.Close()
		close(cmdDone)
	}()

	// Create and run TUI model.
	model := NewInitModel(opts.Component, opts.Stack, opts.SubCommand, opts.Workspace, pr)

	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	// Run TUI - this blocks until completion.
	finalModel, err := p.Run()
	if err != nil {
		// Kill the process if TUI failed.
		_ = cmd.Process.Kill()
		return fmt.Errorf("%w: %w", errUtils.ErrTUIRun, err)
	}

	// Wait for command to finish (should already be done since pipe closed).
	<-cmdDone

	// Get exit code.
	var exitCode int
	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return cmdErr
		}
	}

	// Check if model has an error.
	m := finalModel.(InitModel)
	if m.GetError() != nil {
		return m.GetError()
	}

	// If model captured an exit code, use it.
	if m.GetExitCode() != 0 {
		exitCode = m.GetExitCode()
	}

	// Return exit code error if non-zero.
	if exitCode != 0 {
		return errUtils.ExitCodeError{Code: exitCode}
	}

	return nil
}

// extractOutFlag checks if -out flag is present and returns its value.
func extractOutFlag(args []string) string {
	for i, arg := range args {
		if arg == "-out" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "-out=") {
			return strings.TrimPrefix(arg, "-out=")
		}
	}
	return ""
}

// ShouldUseStreamingUI determines if streaming UI should be used.
// This checks the flag, config, TTY availability, and CI environment.
func ShouldUseStreamingUI(uiFlagSet bool, uiFlag bool, configEnabled bool, subCommand string) bool {
	defer perf.Track(nil, "terraform.ui.ShouldUseStreamingUI")()

	// Check if explicitly disabled via --ui=false flag.
	if uiFlagSet && !uiFlag {
		return false
	}

	// Check if enabled via --ui flag OR config.
	enabled := (uiFlagSet && uiFlag) || configEnabled
	if !enabled {
		return false
	}

	// Auto-disable in CI environments.
	if telemetry.IsCI() {
		return false
	}

	// Auto-disable if stdout is not a TTY (piped output).
	if !term.IsTTYSupportForStdout() {
		return false
	}

	// Only enable for supported subcommands.
	switch subCommand {
	case "plan", "apply", "init", "destroy":
		return true
	case "refresh":
		// refresh doesn't have good -json streaming support.
		return false
	default:
		return false
	}
}

// ExecuteApply runs terraform apply with optional confirmation.
// If -auto-approve is not present and not using --from-plan, it runs:
// 1. terraform plan -json -out=<temp> (with TUI)
// 2. Display dependency tree
// 3. Confirmation prompt
// 4. terraform apply -json <temp> (with TUI)
func ExecuteApply(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteApply")()

	// Check if -auto-approve is in args - skip confirmation.
	if hasFlag(opts.Args, "-auto-approve") {
		return Execute(ctx, opts)
	}

	// Check if applying from existing plan - show tree and confirm.
	if planFile := extractPlanFile(opts.Args); planFile != "" {
		return executeWithPlanFile(ctx, opts, planFile)
	}

	// Two-phase: Plan → Tree → Confirm → Apply.
	return executeTwoPhaseApply(ctx, opts)
}

// executeTwoPhaseOperation executes a plan → confirm → apply workflow.
// isDestroy determines whether this is a destroy operation (affects planfile name and confirmation).
func executeTwoPhaseOperation(ctx context.Context, opts *ExecuteOptions, isDestroy bool) error {
	// Generate temp planfile path.
	planFilePattern := ".atmos-plan-%d.tfplan"
	if isDestroy {
		planFilePattern = ".atmos-destroy-%d.tfplan"
	}
	planFile := filepath.Join(opts.WorkingDir, fmt.Sprintf(planFilePattern, time.Now().UnixNano()))

	// Phase 1: Run plan with TUI.
	var planArgs []string
	if isDestroy {
		planArgs = buildDestroyPlanArgs(opts.Args, planFile)
	} else {
		planArgs = buildPlanArgs(opts.Args, planFile)
	}
	planOpts := *opts
	planOpts.Args = planArgs
	planOpts.SubCommand = "plan"

	if err := Execute(ctx, &planOpts); err != nil {
		// Clean up planfile on error.
		os.Remove(planFile)
		return err
	}

	// Phase 1.5: Parse planfile and check for changes.
	tree, err := BuildDependencyTree(ctx, planFile, opts.Command, opts.WorkingDir, opts.Stack, opts.Component)
	if err == nil {
		// Check if there are any changes.
		add, change, remove := tree.GetChangeSummary()
		if add == 0 && change == 0 && remove == 0 {
			// No changes to apply - show badge, outputs, and exit.
			os.Remove(planFile)
			_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
			fetchAndDisplayOutputs(opts.Command, opts.WorkingDir)
			return nil
		}
		// Display the dependency tree and badge summary.
		_ = ui.Writef("\n%s", tree.RenderTree())
		_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
	}

	// Phase 2: Confirmation.
	var confirmed bool
	if isDestroy {
		confirmed, err = ConfirmDestroy()
	} else {
		confirmed, err = ConfirmApply()
	}
	if err != nil {
		os.Remove(planFile)
		return err
	}
	if !confirmed {
		os.Remove(planFile)
		cancelMsg := "Apply cancelled"
		if isDestroy {
			cancelMsg = "Destroy cancelled"
		}
		_ = ui.Warning(cancelMsg)
		return nil
	}

	// Phase 3: Apply the planfile.
	applyArgs := buildApplyArgs(planFile)
	applyOpts := *opts
	applyOpts.Args = applyArgs
	applyOpts.SubCommand = "apply"

	err = Execute(ctx, &applyOpts)

	// Cleanup planfile.
	os.Remove(planFile)

	return err
}

func executeTwoPhaseApply(ctx context.Context, opts *ExecuteOptions) error {
	return executeTwoPhaseOperation(ctx, opts, false)
}

// ExecuteDestroy runs terraform destroy with confirmation.
// It runs:
// 1. terraform plan -destroy -json -out=<temp> (with TUI)
// 2. Display dependency tree
// 3. Confirmation prompt
// 4. terraform apply -json <temp> (with TUI)
func ExecuteDestroy(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteDestroy")()

	// Check if -auto-approve is in args - skip confirmation.
	if hasFlag(opts.Args, "-auto-approve") {
		return Execute(ctx, opts)
	}

	// Two-phase: Plan → Tree → Confirm → Apply.
	return executeTwoPhaseDestroy(ctx, opts)
}

func executeTwoPhaseDestroy(ctx context.Context, opts *ExecuteOptions) error {
	return executeTwoPhaseOperation(ctx, opts, true)
}

func executeWithPlanFile(ctx context.Context, opts *ExecuteOptions, planFile string) error {
	// Parse planfile and check for changes.
	tree, err := BuildDependencyTree(ctx, planFile, opts.Command, opts.WorkingDir, opts.Stack, opts.Component)
	if err == nil {
		// Check if there are any changes.
		add, change, remove := tree.GetChangeSummary()
		if add == 0 && change == 0 && remove == 0 {
			// No changes to apply - show badge, outputs, and exit.
			_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
			fetchAndDisplayOutputs(opts.Command, opts.WorkingDir)
			return nil
		}
		// Display the dependency tree and badge summary.
		_ = ui.Writef("\n%s", tree.RenderTree())
		_ = ui.Write(RenderChangeSummaryBadges(add, change, remove))
	}

	// Confirm.
	confirmed, err := ConfirmApply()
	if err != nil {
		return err
	}
	if !confirmed {
		_ = ui.Warning("Apply cancelled")
		return nil
	}

	// Execute apply.
	return Execute(ctx, opts)
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func extractPlanFile(args []string) string {
	// Look for a planfile argument (positional argument that's a file path).
	// Also check for --from-plan or --planfile flags.
	for i, arg := range args {
		if arg == "--from-plan" || arg == "--planfile" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if strings.HasPrefix(arg, "--from-plan=") {
			return strings.TrimPrefix(arg, "--from-plan=")
		}
		if strings.HasPrefix(arg, "--planfile=") {
			return strings.TrimPrefix(arg, "--planfile=")
		}
	}

	// Check for positional planfile (last arg that looks like a file).
	if len(args) > 1 {
		lastArg := args[len(args)-1]
		if !strings.HasPrefix(lastArg, "-") && strings.HasSuffix(lastArg, ".tfplan") {
			return lastArg
		}
	}

	return ""
}

func buildPlanArgs(args []string, planFile string) []string {
	// Convert apply args to plan args.
	// Let append handle capacity growth to avoid integer overflow concerns.
	var result []string

	// Replace "apply" with "plan".
	for i, arg := range args {
		if i == 0 && arg == "apply" {
			result = append(result, "plan")
		} else if arg == "-auto-approve" {
			// Skip -auto-approve for plan.
			continue
		} else {
			result = append(result, arg)
		}
	}

	// Add -out flag.
	result = append(result, "-out="+planFile)

	return result
}

func buildApplyArgs(planFile string) []string {
	// Simple apply with planfile - no -auto-approve needed since planfile is provided.
	return []string{"apply", planFile}
}

func buildDestroyPlanArgs(args []string, planFile string) []string {
	// Convert destroy args to plan -destroy args.
	// Let append handle capacity growth to avoid integer overflow concerns.
	var result []string

	// Replace "destroy" with "plan" and add -destroy flag.
	for i, arg := range args {
		if i == 0 && arg == "destroy" {
			result = append(result, "plan", "-destroy")
		} else if arg == "-auto-approve" {
			// Skip -auto-approve for plan.
			continue
		} else {
			result = append(result, arg)
		}
	}

	// Add -out flag.
	result = append(result, "-out="+planFile)

	return result
}

// displayOutputs renders terraform outputs after apply using a styled table.
func displayOutputs(tracker *ResourceTracker) {
	outputs := tracker.GetOutputs()
	if outputs == nil || len(outputs.Outputs) == 0 {
		return
	}

	// Build rows for the table.
	var rows [][]string
	for name, output := range outputs.Outputs {
		var value string
		if output.Sensitive {
			value = "<sensitive>"
		} else {
			value = formatOutputValue(output.Value)
		}
		rows = append(rows, []string{name, value})
	}

	// Sort rows by output name for consistent display.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	// Create and display the table with semantic cell styling.
	headers := []string{"Output", "Value"}
	tableStr := createOutputsTable(headers, rows)
	_ = ui.Writef("\n%s\n", tableStr)
}

// fetchAndDisplayOutputs fetches outputs using terraform output -json and displays them.
// This is used when there are no changes to apply but we still want to show current outputs.
func fetchAndDisplayOutputs(command, workingDir string) {
	// Run terraform output -json to get current outputs.
	cmd := exec.Command(command, "output", "-json")
	cmd.Dir = workingDir

	outputBytes, err := cmd.Output()
	if err != nil {
		// Silently ignore errors - outputs might not exist yet.
		return
	}

	// Parse the JSON output.
	var outputs map[string]OutputValue
	if err := json.Unmarshal(outputBytes, &outputs); err != nil {
		return
	}

	if len(outputs) == 0 {
		return
	}

	// Build rows for the table.
	var rows [][]string
	for name, output := range outputs {
		var value string
		if output.Sensitive {
			value = "<sensitive>"
		} else {
			value = formatOutputValue(output.Value)
		}
		rows = append(rows, []string{name, value})
	}

	// Sort rows by output name for consistent display.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][0] < rows[j][0]
	})

	// Create and display the table with semantic cell styling.
	headers := []string{"Output", "Value"}
	tableStr := createOutputsTable(headers, rows)
	_ = ui.Writef("\n%s\n", tableStr)
}

// formatOutputValue formats an output value for display in a table.
func formatOutputValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		// JSON numbers are float64.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case nil:
		return "null"
	default:
		// For complex types (maps, arrays), use JSON.
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

// createOutputsTable creates a table with semantic cell styling for terraform outputs.
func createOutputsTable(headers []string, rows [][]string) string {
	styles := theme.GetCurrentStyles()

	config := theme.TableConfig{
		Style:       theme.TableStyleMinimal,
		ShowBorders: false,
		ShowHeader:  true,
		Styles:      styles,
		StyleFunc:   createOutputsStyleFunc(rows, styles),
	}

	return theme.CreateTable(&config, headers, rows)
}

// createOutputsStyleFunc returns a styling function for the outputs table.
func createOutputsStyleFunc(rows [][]string, styles *theme.StyleSet) func(int, int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		baseStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)

		if styles == nil {
			return baseStyle
		}

		// Header row styling.
		if row == -1 {
			return baseStyle.Inherit(styles.TableHeader)
		}

		// First column (output name) uses standard row styling.
		if col == 0 {
			return baseStyle.Inherit(styles.TableRow)
		}

		// Value column (col 1) - apply semantic styling based on content.
		if row >= 0 && row < len(rows) && col < len(rows[row]) {
			value := rows[row][col]
			return getOutputCellStyle(value, baseStyle, styles)
		}

		return baseStyle.Inherit(styles.TableRow)
	}
}

// getOutputCellStyle returns the appropriate style for an output value cell.
func getOutputCellStyle(value string, baseStyle lipgloss.Style, styles *theme.StyleSet) lipgloss.Style {
	contentType := detectOutputContentType(value)

	switch contentType {
	case outputContentTypeBoolean:
		if value == "true" {
			return baseStyle.Foreground(styles.Success.GetForeground())
		}
		return baseStyle.Foreground(styles.Error.GetForeground())

	case outputContentTypeNumber:
		return baseStyle.Foreground(styles.Info.GetForeground())

	case outputContentTypeSensitive:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	case outputContentTypeNull:
		return baseStyle.Foreground(styles.Muted.GetForeground())

	default:
		return baseStyle.Inherit(styles.TableRow)
	}
}

// outputContentType represents the type of content in an output value cell.
type outputContentType int

const (
	outputContentTypeDefault outputContentType = iota
	outputContentTypeBoolean
	outputContentTypeNumber
	outputContentTypeSensitive
	outputContentTypeNull
)

// detectOutputContentType determines the content type of an output value.
func detectOutputContentType(value string) outputContentType {
	if value == "" {
		return outputContentTypeDefault
	}

	// Check for sensitive marker.
	if value == "<sensitive>" {
		return outputContentTypeSensitive
	}

	// Check for null.
	if value == "null" {
		return outputContentTypeNull
	}

	// Check for booleans.
	if value == "true" || value == "false" {
		return outputContentTypeBoolean
	}

	// Check for numbers (integers or floats).
	if isNumericString(value) {
		return outputContentTypeNumber
	}

	return outputContentTypeDefault
}

// isNumericString checks if a string represents a number.
func isNumericString(s string) bool {
	// Try to parse as float (covers both integers and floats).
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}
