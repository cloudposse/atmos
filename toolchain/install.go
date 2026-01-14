package toolchain

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	spinnerAnimationFrames = 5 // Number of animation frames for progress spinner.

	// Install result status constants.
	resultInstalled = "installed"
	resultFailed    = "failed"
	resultSkipped   = "skipped"
)

// InstallOptions configures the behavior of InstallSingleTool.
type InstallOptions struct {
	IsLatest               bool // Whether this is a "latest" version install.
	ShowProgressBar        bool // Whether to show progress during install.
	ShowHint               bool // Whether to show PATH export hint after install.
	SkipToolVersionsUpdate bool // Skip .tool-versions update (caller handles it).
}

// Bubble Tea spinner model.
type spinnerModel struct {
	spinner bspinner.Model
	message string
	done    bool
}

func initialSpinnerModel(message string) *spinnerModel {
	s := bspinner.New()
	s.Spinner = bspinner.Dot
	styles := theme.GetCurrentStyles()
	s.Style = styles.Spinner
	return &spinnerModel{
		spinner: s,
		message: message,
	}
}

func (m *spinnerModel) Init() tea.Cmd {
	defer perf.Track(nil, "toolchain.spinnerModel.Init")()

	return m.spinner.Tick
}

func (m *spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "toolchain.spinnerModel.Update")()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case bspinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case installDoneMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *spinnerModel) View() string {
	defer perf.Track(nil, "toolchain.spinnerModel.View")()

	if m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.message)
}

// Custom message type for signaling installation completion.
type installDoneMsg struct{}

// Run spinner with Bubble Tea - proper way.
func runBubbleTeaSpinner(message string) *tea.Program {
	p := tea.NewProgram(initialSpinnerModel(message), tea.WithOutput(os.Stderr))
	return p
}

// RunInstall installs the specified tool (owner/repo@version or alias@version).
// If toolSpec is empty, installs all tools from .tool-versions file.
// The setAsDefault parameter controls whether to set the installed version as default.
// The reinstallFlag parameter forces reinstallation even if already installed.
// The showHint parameter controls whether to show the PATH export hint message.
// The showProgressBar parameter controls whether to show spinner and success messages.
func RunInstall(toolSpec string, setAsDefault, reinstallFlag, showHint, showProgressBar bool) error {
	defer perf.Track(nil, "toolchain.Install")()

	if toolSpec == "" {
		return installFromToolVersions(GetToolVersionsFilePath(), reinstallFlag, showHint)
	}

	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}

	// Resolve version if not specified.
	if version == "" {
		lookupResult, err := resolveVersionFromToolVersions(tool, toolSpec)
		if err != nil {
			return err
		}
		tool = lookupResult.tool
		version = lookupResult.version
	}

	// Validate tool and version.
	if err := validateToolAndVersion(tool, version, toolSpec); err != nil {
		return err
	}

	// Parse tool specification to get owner/repo.
	installer := NewInstaller()
	owner, repo, err := installer.ParseToolSpec(tool)
	if err != nil {
		// Check if this looks like a short name (no slash) - suggest adding an alias.
		if !strings.Contains(tool, "/") {
			return errUtils.Build(errUtils.ErrInvalidToolSpec).
				WithCause(err).
				WithExplanationf("Tool '%s' is not a valid tool specification", tool).
				WithHintf("Add an alias in atmos.yaml:\n```yaml\ntoolchain:\n  aliases:\n    %s: owner/repo\n```", tool).
				WithHint("Or use full format: owner/repo (e.g., hashicorp/terraform)").
				WithHint("See https://atmos.tools/cli/commands/toolchain/aliases for configuring aliases").
				Err()
		}
		return err
	}

	// Handle "latest" keyword - pass it to InstallSingleTool which will resolve it.
	// Skip .tool-versions update here; updateToolVersionsFile handles it below with the original tool name.
	err = InstallSingleTool(owner, repo, version, InstallOptions{
		IsLatest:               version == "latest",
		ShowProgressBar:        showProgressBar,
		ShowHint:               showHint,
		SkipToolVersionsUpdate: true,
	})
	if err != nil {
		return err
	}

	// Update .tool-versions file.
	return updateToolVersionsFile(tool, version, setAsDefault)
}

func InstallSingleTool(owner, repo, version string, opts InstallOptions) error {
	defer perf.Track(nil, "toolchain.InstallSingleTool")()

	installer := NewInstaller()

	// Start spinner immediately before any potentially slow operations.
	spinner := &spinnerControl{showingSpinner: opts.ShowProgressBar}
	message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
	spinner.start(message)

	// Resolve "latest" version if needed (network call - can be slow).
	resolvedVersion, err := resolveLatestVersionWithSpinner(owner, repo, version, opts.IsLatest, spinner)
	if err != nil {
		spinner.stop()
		return err
	}
	version = resolvedVersion

	// Perform installation.
	binaryPath, err := installer.Install(owner, repo, version)

	// Stop spinner before printing any output.
	spinner.stop()

	if err != nil {
		if opts.ShowProgressBar {
			_ = ui.Errorf("Install failed %s/%s@%s: %v", owner, repo, version, err)
		}
		return err
	}

	// Handle post-installation tasks.
	handleInstallSuccess(installResult{
		owner:                  owner,
		repo:                   repo,
		version:                version,
		binaryPath:             binaryPath,
		isLatest:               opts.IsLatest,
		showMessage:            opts.ShowProgressBar,
		showHint:               opts.ShowHint,
		skipToolVersionsUpdate: opts.SkipToolVersionsUpdate,
	}, installer)
	return nil
}

func installFromToolVersions(toolVersionsPath string, reinstallFlag, showHint bool) error {
	installer := NewInstaller()

	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	toolList := buildToolList(installer, toolVersions)
	if len(toolList) == 0 {
		_ = ui.Writef("No tools found in %s\n", toolVersionsPath)
		return nil
	}

	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	styles := theme.GetCurrentStyles()
	spinner.Style = styles.Spinner
	progressBar := progress.New(progress.WithDefaultGradient())

	var installedCount, failedCount, alreadyInstalledCount int

	for i, tool := range toolList {
		result, err := installOrSkipTool(installer, tool, reinstallFlag, showHint)
		switch result {
		case resultInstalled:
			installedCount++
		case resultFailed:
			failedCount++
		case resultSkipped:
			alreadyInstalledCount++
		}
		showProgress(&spinner, &progressBar, tool, progressState{index: i, total: len(toolList), result: result, err: err})
	}

	printSummary(installedCount, failedCount, alreadyInstalledCount, len(toolList), showHint)
	return nil
}

func buildToolList(installer *Installer, toolVersions *ToolVersions) []toolInfo {
	var toolList []toolInfo
	for toolName, versions := range toolVersions.Tools {
		owner, repo, err := installer.ParseToolSpec(toolName)
		if err != nil {
			continue
		}
		for _, version := range versions {
			toolList = append(toolList, toolInfo{version, owner, repo})
		}
	}
	return toolList
}

func installOrSkipTool(installer *Installer, tool toolInfo, reinstallFlag, showHint bool) (string, error) {
	_, err := installer.FindBinaryPath(tool.owner, tool.repo, tool.version)
	if err == nil && !reinstallFlag {
		return resultSkipped, nil
	}

	err = InstallSingleTool(tool.owner, tool.repo, tool.version, InstallOptions{
		IsLatest:        tool.version == "latest",
		ShowProgressBar: true,
		ShowHint:        showHint,
	})
	if err != nil {
		return resultFailed, err
	}
	return resultInstalled, nil
}

type toolInfo struct {
	version, owner, repo string
}

type progressState struct {
	index, total int
	result       string
	err          error
}

func showProgress(
	spinner *bspinner.Model,
	progressBar *progress.Model,
	tool toolInfo,
	state progressState,
) {
	// Clear progress bar line before printing status (same pattern as uninstall).
	resetLine()

	switch state.result {
	case resultSkipped:
		_ = ui.Successf("Skipped `%s/%s@%s` (already installed)", tool.owner, tool.repo, tool.version)
	case resultInstalled:
		_ = ui.Successf("Installed `%s/%s@%s`", tool.owner, tool.repo, tool.version)
	case resultFailed:
		_ = ui.Errorf("Install failed %s/%s@%s: %v", tool.owner, tool.repo, tool.version, state.err)
	}

	percent := float64(state.index+1) / float64(state.total)
	bar := progressBar.ViewAs(percent)

	// Show animated progress bar using helper (handles TTY detection).
	for j := 0; j < spinnerAnimationFrames; j++ {
		printProgressBar(fmt.Sprintf("%s %s", spinner.View(), bar))
		spin, _ := spinner.Update(bspinner.TickMsg{})
		spinner = &spin
		time.Sleep(50 * time.Millisecond)
	}
}

func printSummary(installed, failed, skipped, total int, showHint bool) {
	// Clear progress bar line before printing summary.
	resetLine()
	_ = ui.Writeln("")

	if total == 0 {
		_ = ui.Success("No tools to install")
		return
	}

	// Print result message based on failure/skip counts.
	if failed > 0 {
		printFailureSummary(installed, failed, skipped)
		return
	}

	printSuccessSummary(installed, skipped, showHint)
}

func printFailureSummary(installed, failed, skipped int) {
	if skipped == 0 {
		_ = ui.Errorf("Installed %d tools, failed %d", installed, failed)
	} else {
		_ = ui.Errorf("Installed %d tools, failed %d, skipped %d", installed, failed, skipped)
	}
}

func printSuccessSummary(installed, skipped int, showHint bool) {
	if skipped == 0 {
		_ = ui.Successf("Installed **%d** tools", installed)
	} else {
		_ = ui.Successf("Installed **%d** tools, skipped **%d**", installed, skipped)
	}
	if showHint {
		_ = ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
	}
}

// RunInstallBatch installs multiple tools with batch progress display.
// Shows status messages scrolling up with a single progress bar at bottom.
// For single tools, delegates to RunInstall for simpler output.
func RunInstallBatch(toolSpecs []string, reinstallFlag bool) error {
	defer perf.Track(nil, "toolchain.RunInstallBatch")()

	if len(toolSpecs) == 0 {
		return nil
	}

	// Single tool: delegate to single-tool flow for simpler output.
	if len(toolSpecs) == 1 {
		return RunInstall(toolSpecs[0], false, reinstallFlag, false, true)
	}

	// Multiple tools: batch install with progress bar.
	return installMultipleTools(toolSpecs, reinstallFlag)
}

// installMultipleTools handles batch installation with progress bar.
func installMultipleTools(toolSpecs []string, reinstallFlag bool) error {
	defer perf.Track(nil, "toolchain.installMultipleTools")()

	installer := NewInstaller()

	// Parse all tool specs first.
	var toolList []toolInfo
	for _, spec := range toolSpecs {
		tool, version, err := ParseToolVersionArg(spec)
		if err != nil {
			_ = ui.Errorf("Invalid tool spec `%s`: %v", spec, err)
			continue
		}

		owner, repo, err := installer.ParseToolSpec(tool)
		if err != nil {
			_ = ui.Errorf("Invalid tool `%s`: %v", tool, err)
			continue
		}

		toolList = append(toolList, toolInfo{
			version: version,
			owner:   owner,
			repo:    repo,
		})
	}

	if len(toolList) == 0 {
		return nil
	}

	// Set up progress display.
	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	styles := theme.GetCurrentStyles()
	spinner.Style = styles.Spinner
	progressBar := progress.New(progress.WithDefaultGradient())

	var installedCount, failedCount, skippedCount int

	for i, tool := range toolList {
		result, err := installOrSkipTool(installer, tool, reinstallFlag, false)
		switch result {
		case resultInstalled:
			installedCount++
		case resultFailed:
			failedCount++
		case resultSkipped:
			skippedCount++
		}
		showProgress(&spinner, &progressBar, tool, progressState{
			index:  i,
			total:  len(toolList),
			result: result,
			err:    err,
		})
	}

	printSummary(installedCount, failedCount, skippedCount, len(toolList), false)
	return nil
}
