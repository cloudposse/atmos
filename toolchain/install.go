package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
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

// RunInstall installs the specified tool (owner/repo@version or alias@version).
// If toolSpec is empty, installs all tools from .tool-versions file.
// The setAsDefault parameter controls whether to set the installed version as default.
// The reinstallFlag parameter forces reinstallation even if already installed.
func RunInstall(toolSpec string, setAsDefault, reinstallFlag bool) error {
	defer perf.Track(nil, "toolchain.Install")()

	if toolSpec == "" {
		return installFromToolVersions(GetToolVersionsFilePath(), reinstallFlag)
	}
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if version == "" {
		// Try to look up version in .tool-versions or fallback to alias/latest.
		toolVersions, err := LoadToolVersions(DefaultToolVersionsFilePath)
		if err != nil {
			return errUtils.Build(errUtils.ErrInvalidToolSpec).
				WithExplanationf("Invalid tool specification: `%s`", toolSpec).
				WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
				WithHint("Or use alias: `terraform@1.5.0` (requires `.tool-versions` or registry alias)").
				WithHint("File `.tool-versions` could not be loaded").
				WithContext("tool_spec", toolSpec).
				WithContext("tool_versions_file", DefaultToolVersionsFilePath).
				WithContext("error", err.Error()).
				WithExitCode(2).
				Err()
		}
		installer := NewInstaller()
		resolvedKey, foundVersion, found, usedLatest := LookupToolVersionOrLatest(tool, toolVersions, installer.resolver)
		if !found && !usedLatest {
			return errUtils.Build(errUtils.ErrInvalidToolSpec).
				WithExplanationf("Invalid tool specification: `%s`", toolSpec).
				WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
				WithHint("Or add tool to `.tool-versions` file").
				WithContext("tool_spec", toolSpec).
				WithExitCode(2).
				Err()
		}
		tool = resolvedKey
		version = foundVersion
	}
	if tool == "" || version == "" {
		return errUtils.Build(errUtils.ErrInvalidToolSpec).
			WithExplanationf("Invalid tool specification: `%s`", toolSpec).
			WithHint("Use format: `owner/repo@version` (e.g., `hashicorp/terraform@1.5.0`)").
			WithHint("Or use alias: `terraform@1.5.0`").
			WithContext("tool_spec", toolSpec).
			WithExitCode(2).
			Err()
	}

	// Use the enhanced parseToolSpec to handle both owner/repo and tool name formats.
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

	// Update .tool-versions: add version, set as default if requested.
	if setAsDefault {
		if err := AddToolToVersionsAsDefault(DefaultToolVersionsFilePath, tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	} else {
		if err := AddToolToVersions(DefaultToolVersionsFilePath, tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	}

	return nil
}

func InstallSingleTool(owner, repo, version string, isLatest bool, showProgressBar bool) error {
	defer perf.Track(nil, "toolchain.InstallSingleTool")()

	installer := NewInstaller()

	// Start spinner immediately before any potentially slow operations.
	var p *tea.Program
	if showProgressBar {
		message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
		p = runBubbleTeaSpinner(message)
		go func() {
			// Run spinner in background, ignore error since program is managed by quit channel.
			_, _ = p.Run()
		}()
	}

	// Resolve "latest" version if needed (network call - can be slow).
	if isLatest && version == "latest" {
		registry := NewAquaRegistry()
		latestVersion, err := registry.GetLatestVersion(owner, repo)
		if err != nil {
			// Stop spinner before showing error.
			if showProgressBar && p != nil {
				p.Send(installDoneMsg{})
				time.Sleep(100 * time.Millisecond)
			}
			return fmt.Errorf("failed to get latest version for %s/%s: %w", owner, repo, err)
		}
		// Update version to the resolved latest version.
		version = latestVersion
		// Update spinner message to show actual version.
		if showProgressBar && p != nil {
			p.Send(installDoneMsg{})
			time.Sleep(50 * time.Millisecond)
			_ = ui.Toastf("📦", "Using latest version `%s`", latestVersion)
			// Restart spinner with updated message.
			message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
			p = runBubbleTeaSpinner(message)
			go func() {
				_, _ = p.Run()
			}()
		}
	}

	// Perform installation.
	binaryPath, err := installer.Install(owner, repo, version)

	// Stop spinner before printing any output.
	if showProgressBar && p != nil {
		p.Send(installDoneMsg{})
		// Small delay to ensure spinner is cleared before printing output.
		time.Sleep(100 * time.Millisecond)
	}

	if err != nil {
		if showProgressBar {
			_ = ui.Errorf("Install failed %s/%s@%s: %v", owner, repo, version, err)
		}
		return err
	}
	if isLatest {
		if err := installer.CreateLatestFile(owner, repo, version); err != nil {
			if showProgressBar {
				_ = ui.Errorf("Failed to create latest file for %s/%s: %v", owner, repo, err)
			}
		}
	}
	if showProgressBar {
		// Calculate directory size for the installed version.
		versionDir := filepath.Dir(binaryPath)
		sizeStr := ""
		if dirSize, err := calculateDirectorySize(versionDir); err == nil {
			sizeStr = fmt.Sprintf(" (%s)", formatBytes(dirSize))
		}
		_ = ui.Successf("Installed `%s/%s@%s` to `%s`%s", owner, repo, version, binaryPath, sizeStr)
	}
	if err := AddToolToVersions(DefaultToolVersionsFilePath, repo, version); err == nil && showProgressBar {
		_ = ui.Successf("Registered `%s %s` in `.tool-versions`", repo, version)
	}
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
		showProgress(&spinner, progressBar, tool, progressState{index: i, total: len(toolList), result: result, err: err})
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
	progressBar progress.Model,
	tool toolInfo,
	state progressState,
) {
	switch state.result {
	case "skipped":
		_ = ui.Successf("Skipped `%s/%s@%s` (already installed)", tool.owner, tool.repo, tool.version)
	case "installed":
		_ = ui.Successf("Installed `%s/%s@%s`", tool.owner, tool.repo, tool.version)
	case "failed":
		_ = ui.Errorf("Install failed %s/%s@%s: %v", tool.owner, tool.repo, tool.version, state.err)
	}

	percent := float64(state.index+1) / float64(state.total)
	bar := progressBar.ViewAs(percent)

	// Show animated progress bar
	for j := 0; j < 5; j++ {
		_ = ui.Writef("\r%s %s", spinner.View(), bar)
		spin, _ := spinner.Update(bspinner.TickMsg{})
		spinner = &spin
		time.Sleep(50 * time.Millisecond)
	}
}

func printSummary(installed, failed, skipped, total int) {
	_ = ui.Writeln("")

	switch {
	case total == 0:
		_ = ui.Success("No tools to install")
	case failed == 0 && skipped == 0:
		_ = ui.Successf("Installed **%d** tools", installed)
	case failed == 0 && skipped > 0:
		_ = ui.Successf("Installed **%d** tools, skipped **%d**", installed, skipped)
	case failed > 0 && skipped == 0:
		_ = ui.Errorf("Installed %d tools, failed %d", installed, failed)
	default:
		_ = ui.Errorf("Installed %d tools, failed %d, skipped %d", installed, failed, skipped)
	}
}
