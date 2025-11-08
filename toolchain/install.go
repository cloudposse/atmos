package toolchain

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
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
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	p := tea.NewProgram(initialSpinnerModel(message))
	return p
}

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
			return fmt.Errorf("invalid tool specification: %s. Expected format: owner/repo@version or tool@version, and failed to load .tool-versions: %w", toolSpec, err)
		}
		installer := NewInstaller()
		resolvedKey, foundVersion, found, usedLatest := LookupToolVersionOrLatest(tool, toolVersions, installer.resolver)
		if !found && !usedLatest {
			return fmt.Errorf("%w: %s. Expected format: owner/repo@version or tool@version, and tool not found in .tool-versions or as an alias", ErrInvalidToolSpec, toolSpec)
		}
		tool = resolvedKey
		version = foundVersion
	}
	if tool == "" || version == "" {
		return fmt.Errorf("%w: %s. Expected format: owner/repo@version or tool@version", ErrInvalidToolSpec, toolSpec)
	}

	// Use the enhanced parseToolSpec to handle both owner/repo and tool name formats.
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("%w: %s. Expected format: owner/repo or tool name", ErrInvalidToolSpec, tool)
	}

	// Handle "latest" keyword.
	isLatest := false
	if version == "latest" {
		registry := NewAquaRegistry()
		latestVersion, err := registry.GetLatestVersion(owner, repo)
		if err != nil {
			return fmt.Errorf("failed to get latest version for %s/%s: %w", owner, repo, err)
		}
		_ = ui.Toastf("📦", "Using latest version: %s", latestVersion)
		version = latestVersion
		isLatest = true
	}

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

	var p *tea.Program
	if showProgressBar {
		message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
		p = runBubbleTeaSpinner(message)
		go func() {
			// Run spinner in background, ignore error since program is managed by quit channel.
			_, _ = p.Run()
		}()
	}

	// Perform installation.
	binaryPath, err := installer.Install(owner, repo, version)
	if err != nil {
		if showProgressBar && p != nil {
			p.Send(installDoneMsg{})
		}
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
		_ = ui.Successf("Installed %s/%s@%s to %s", owner, repo, version, binaryPath)
	}
	if showProgressBar && p != nil {
		p.Send(installDoneMsg{})
		// Small delay to ensure spinner is cleared.
		time.Sleep(100 * time.Millisecond)
	}
	if err := AddToolToVersions(DefaultToolVersionsFilePath, repo, version); err == nil && showProgressBar {
		_ = ui.Successf("Registered %s %s in .tool-versions", repo, version)
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
	spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
		_ = ui.Successf("Skipped %s/%s@%s (already installed)", tool.owner, tool.repo, tool.version)
	case "installed":
		_ = ui.Successf("Installed %s/%s@%s", tool.owner, tool.repo, tool.version)
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
		_ = ui.Successf("Installed %d tools", installed)
	case failed == 0 && skipped > 0:
		_ = ui.Successf("Installed %d tools, skipped %d", installed, skipped)
	case failed > 0 && skipped == 0:
		_ = ui.Errorf("Installed %d tools, failed %d", installed, failed)
	default:
		_ = ui.Errorf("Installed %d tools, failed %d, skipped %d", installed, failed, skipped)
	}
}
