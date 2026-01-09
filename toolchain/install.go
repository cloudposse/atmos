package toolchain

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	spinnerAnimationFrames = 5 // Number of animation frames for progress spinner.
)

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

// RunInstall installs the specified tools (owner/repo@version or alias@version).
// If toolSpecs is empty, installs all tools from .tool-versions file.
// If toolSpecs has one element, installs that single tool with progress.
// If toolSpecs has multiple elements, installs all with batch progress and summary.
// The setAsDefault parameter controls whether to set the installed version as default.
// The reinstallFlag parameter forces reinstallation even if already installed.
func RunInstall(toolSpecs []string, setAsDefault, reinstallFlag bool) error {
	defer perf.Track(nil, "toolchain.Install")()

	// No args: install from .tool-versions file.
	if len(toolSpecs) == 0 {
		return installFromToolVersions(GetToolVersionsFilePath(), reinstallFlag)
	}

	// Single tool: use original single-tool flow with full progress.
	if len(toolSpecs) == 1 {
		return installSingleToolSpec(toolSpecs[0], setAsDefault, reinstallFlag)
	}

	// Multiple tools: use batch installation with progress bar and summary.
	return installMultipleTools(toolSpecs, setAsDefault, reinstallFlag)
}

// installSingleToolSpec installs a single tool specification with full progress display.
// Note: reinstallFlag is accepted for API consistency but handled by InstallSingleTool internally.
func installSingleToolSpec(toolSpec string, setAsDefault, _ bool) error {
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
	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("%w: %s. Expected format: owner/repo or tool name", ErrInvalidToolSpec, tool)
	}

	// Handle "latest" keyword - pass it to InstallSingleTool which will resolve it.
	isLatest := version == "latest"

	err = InstallSingleTool(owner, repo, version, isLatest, true)
	if err != nil {
		return err
	}

	// Update .tool-versions file.
	return updateToolVersionsFile(tool, version, setAsDefault)
}

// parseToolSpecs parses command-line tool specs into toolInfo structs.
func parseToolSpecs(installer *Installer, toolSpecs []string) []toolInfo {
	var toolList []toolInfo
	for _, spec := range toolSpecs {
		info, ok := parseToolSpec(installer, spec)
		if ok {
			toolList = append(toolList, info)
		}
	}
	return toolList
}

// parseToolSpec parses a single tool specification into toolInfo.
func parseToolSpec(installer *Installer, spec string) (toolInfo, bool) {
	tool, version, err := ParseToolVersionArg(spec)
	if err != nil {
		_ = ui.Errorf("Invalid tool spec `%s`: %v", spec, err)
		return toolInfo{}, false
	}

	// Resolve version if not specified.
	if version == "" {
		lookupResult, err := resolveVersionFromToolVersions(tool, spec)
		if err != nil {
			_ = ui.Errorf("Failed to resolve version for `%s`: %v", spec, err)
			return toolInfo{}, false
		}
		tool = lookupResult.tool
		version = lookupResult.version
	}

	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		_ = ui.Errorf("Invalid tool `%s`: %v", tool, err)
		return toolInfo{}, false
	}

	return toolInfo{version: version, owner: owner, repo: repo}, true
}

// installMultipleTools installs multiple tool specifications with batch progress.
func installMultipleTools(toolSpecs []string, setAsDefault, reinstallFlag bool) error {
	installer := NewInstaller()
	toolList := parseToolSpecs(installer, toolSpecs)

	if len(toolList) == 0 {
		return ErrNoValidTools
	}

	// Use existing batch installation infrastructure.
	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	styles := theme.GetCurrentStyles()
	spinner.Style = styles.Spinner
	progressBar := progress.New(progress.WithDefaultGradient())

	var installedCount, failedCount, alreadyInstalledCount int

	for i, tool := range toolList {
		result, err := installOrSkipTool(installer, tool, reinstallFlag)
		switch result {
		case "installed":
			installedCount++
			// Update .tool-versions for each successfully installed tool.
			toolName := fmt.Sprintf("%s/%s", tool.owner, tool.repo)
			_ = updateToolVersionsFile(toolName, tool.version, setAsDefault)
		case "failed":
			failedCount++
		case "skipped":
			alreadyInstalledCount++
		}
		showProgress(&spinner, &progressBar, tool, progressState{index: i, total: len(toolList), result: result, err: err})
	}

	printSummary(installedCount, failedCount, alreadyInstalledCount, len(toolList))
	return nil
}

func InstallSingleTool(owner, repo, version string, isLatest bool, showProgressBar bool) error {
	defer perf.Track(nil, "toolchain.InstallSingleTool")()

	installer := NewInstaller()

	// Start spinner immediately before any potentially slow operations.
	spinner := &spinnerControl{showingSpinner: showProgressBar}
	message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
	spinner.start(message)

	// Resolve "latest" version if needed (network call - can be slow).
	resolvedVersion, err := resolveLatestVersionWithSpinner(owner, repo, version, isLatest, spinner)
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
		if showProgressBar {
			_ = ui.Errorf("Install failed %s/%s@%s: %v", owner, repo, version, err)
		}
		return err
	}

	// Handle post-installation tasks.
	handleInstallSuccess(installResult{
		owner:       owner,
		repo:        repo,
		version:     version,
		binaryPath:  binaryPath,
		isLatest:    isLatest,
		showMessage: showProgressBar,
	}, installer)
	return nil
}

func installFromToolVersions(toolVersionsPath string, reinstallFlag bool) error {
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
		result, err := installOrSkipTool(installer, tool, reinstallFlag)
		switch result {
		case "installed":
			installedCount++
		case "failed":
			failedCount++
		case "skipped":
			alreadyInstalledCount++
		}
		showProgress(&spinner, &progressBar, tool, progressState{index: i, total: len(toolList), result: result, err: err})
	}

	printSummary(installedCount, failedCount, alreadyInstalledCount, len(toolList))
	return nil
}

func buildToolList(installer *Installer, toolVersions *ToolVersions) []toolInfo {
	var toolList []toolInfo
	for toolName, versions := range toolVersions.Tools {
		owner, repo, err := installer.parseToolSpec(toolName)
		if err != nil {
			continue
		}
		for _, version := range versions {
			toolList = append(toolList, toolInfo{version, owner, repo})
		}
	}
	return toolList
}

func installOrSkipTool(installer *Installer, tool toolInfo, reinstallFlag bool) (string, error) {
	_, err := installer.FindBinaryPath(tool.owner, tool.repo, tool.version)
	if err == nil && !reinstallFlag {
		return "skipped", nil
	}

	err = InstallSingleTool(tool.owner, tool.repo, tool.version, tool.version == "latest", false)
	if err != nil {
		return "failed", err
	}
	return "installed", nil
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
	// Strategy: Status messages scroll up normally, progress bar updates in place at bottom.
	// 1. Clear current line (where progress bar was)
	// 2. Print status message with newline (scrolls up, cursor on new line)
	// 3. Print progress bar on current line (no newline, overwrites itself each frame)

	// Clear the progress bar line first.
	_ = ui.Write(terminal.EscResetLine)

	// Print status message (with newline - this becomes a permanent line that scrolls up).
	switch state.result {
	case "skipped":
		_ = ui.Successf("Skipped `%s/%s@%s` (already installed)", tool.owner, tool.repo, tool.version)
	case "installed":
		_ = ui.Successf("Installed `%s/%s@%s`", tool.owner, tool.repo, tool.version)
	case "failed":
		_ = ui.Errorf("Install failed %s/%s@%s: %v", tool.owner, tool.repo, tool.version, state.err)
	}

	// Show animated progress bar on current line (EscResetLine overwrites each frame).
	percent := float64(state.index+1) / float64(state.total)
	bar := progressBar.ViewAs(percent)

	for j := 0; j < spinnerAnimationFrames; j++ {
		_ = ui.Writef("%s%s %s", terminal.EscResetLine, spinner.View(), bar)
		spin, _ := spinner.Update(bspinner.TickMsg{})
		spinner = &spin
		time.Sleep(50 * time.Millisecond)
	}
}

func printSummary(installed, failed, skipped, total int) {
	// Clear the progress bar line before printing summary.
	_ = ui.Write(terminal.EscResetLine)
	_ = ui.Writeln("")

	switch {
	case total == 0:
		_ = ui.Success("No tools to install")
	case failed == 0 && skipped == 0:
		_ = ui.Successf("Installed **%d** tools", installed)
		_ = ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
	case failed == 0 && skipped > 0:
		_ = ui.Successf("Installed **%d** tools, skipped **%d**", installed, skipped)
		_ = ui.Hintf("Export the `PATH` environment variable for your toolchain tools using `eval \"$(atmos --chdir /path/to/project toolchain env)\"`")
	case failed > 0 && skipped == 0:
		_ = ui.Errorf("Installed %d tools, failed %d", installed, failed)
	default:
		_ = ui.Errorf("Installed %d tools, failed %d, skipped %d", installed, failed, skipped)
	}
}
