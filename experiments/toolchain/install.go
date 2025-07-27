package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Bubble Tea spinner model
type spinnerModel struct {
	spinner bspinner.Model
	message string
	done    bool
}

func initialSpinnerModel(message string) spinnerModel {
	s := bspinner.New()
	s.Spinner = bspinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	return spinnerModel{
		spinner: s,
		message: message,
	}
}

func (m spinnerModel) Init() tea.Cmd {
	return bspinner.Tick
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.message)
}

// Run spinner with Bubble Tea
func runBubbleTeaSpinner(message string, done *atomic.Bool) {
	p := tea.NewProgram(initialSpinnerModel(message))

	go func() {
		for !done.Load() {
			time.Sleep(100 * time.Millisecond)
		}
		p.Quit()
	}()

	p.Run()
}

var installCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install a CLI binary from the registry",
	Long: `Install a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version
Examples:
  toolchain install suzuki-shunsuke/github-comment@v3.5.0
  toolchain install hashicorp/terraform@v1.5.0
  toolchain install                    # Install from .tool-versions file`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInstall,
}

var reinstallFlag bool

func init() {
	installCmd.Flags().Bool("default", false, "Set installed version as default (front of .tool-versions)")
	installCmd.Flags().BoolVar(&reinstallFlag, "reinstall", false, "Reinstall even if already installed")
}

func runInstall(cmd *cobra.Command, args []string) error {
	// If no arguments, install from tool-versions file
	if len(args) == 0 {
		return installFromToolVersions(GetToolVersionsFilePath())
	}

	setDefault, _ := cmd.Flags().GetBool("default")

	toolSpec := args[0]
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if version == "" {
		// Try to look up version in .tool-versions or fallback to alias/latest
		toolVersions, err := LoadToolVersions(".tool-versions")
		if err != nil {
			return fmt.Errorf("invalid tool specification: %s. Expected format: owner/repo@version or tool@version, and failed to load .tool-versions: %w", toolSpec, err)
		}
		installer := NewInstaller()
		resolvedKey, foundVersion, found, usedLatest := LookupToolVersionOrLatest(tool, toolVersions, installer.resolver)
		if !found && !usedLatest {
			return fmt.Errorf("invalid tool specification: %s. Expected format: owner/repo@version or tool@version, and tool not found in .tool-versions or as an alias", toolSpec)
		}
		tool = resolvedKey
		version = foundVersion
	}
	if tool == "" || version == "" {
		return fmt.Errorf("invalid tool specification: %s. Expected format: owner/repo@version or tool@version", toolSpec)
	}

	// Use the enhanced parseToolSpec to handle both owner/repo and tool name formats
	installer := NewInstaller()
	owner, repo, err := installer.parseToolSpec(tool)
	if err != nil {
		return fmt.Errorf("invalid repository path: %s. Expected format: owner/repo or tool name", tool)
	}

	// Handle "latest" keyword
	isLatest := false
	if version == "latest" {
		registry := NewAquaRegistry()
		latestVersion, err := registry.GetLatestVersion(owner, repo)
		if err != nil {
			return fmt.Errorf("failed to get latest version for %s/%s: %w", owner, repo, err)
		}
		fmt.Printf("📦 Using latest version: %s\n", latestVersion)
		version = latestVersion
		isLatest = true
	}

	err = InstallSingleTool(owner, repo, version, isLatest, true)
	if err != nil {
		return err
	}

	// Update .tool-versions: add version, set as default if requested
	if setDefault {
		if err := AddToolToVersionsAsDefault(".tool-versions", tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	} else {
		if err := AddToolToVersions(".tool-versions", tool, version); err != nil {
			return fmt.Errorf("failed to update .tool-versions: %w", err)
		}
	}

	return nil
}

func InstallSingleTool(owner, repo, version string, isLatest bool, showProgressBar bool) error {
	restoreLogger := SuppressLogger()
	defer restoreLogger()
	installer := NewInstaller()
	if showProgressBar {
		var done atomic.Bool
		message := fmt.Sprintf("Installing %s/%s@%s", owner, repo, version)
		go runBubbleTeaSpinner(message, &done)
		// Simulate install work (replace with real install logic below)
		time.Sleep(2 * time.Second)
		done.Store(true)
	}
	// Perform installation
	binaryPath, err := installer.Install(owner, repo, version)
	if err != nil {
		if showProgressBar {
			fmt.Fprintf(os.Stderr, "\r%s Install failed %s/%s@%s: %v\n", xMark.Render(), owner, repo, version, err)
		}
		return err
	}
	if isLatest {
		if err := installer.createLatestFile(owner, repo, version); err != nil {
			if showProgressBar {
				fmt.Fprintf(os.Stderr, "\r%s Failed to create latest file for %s/%s: %v\n", xMark.Render(), owner, repo, err)
			}
		}
	}
	if showProgressBar {
		fmt.Fprintf(os.Stderr, "%s Installed %s/%s@%s to %s\n", checkMark.Render(), owner, repo, version, binaryPath)
	}
	if err := AddToolToVersions(".tool-versions", repo, version); err == nil && showProgressBar {
		fmt.Fprintf(os.Stderr, "%s Registered %s %s in .tool-versions\n", checkMark.Render(), repo, version)
	}
	return nil
}

func installFromToolVersions(toolVersionsPath string) error {
	restoreLogger := SuppressLogger()
	defer restoreLogger()

	installer := NewInstaller()

	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}

	totalTools := len(toolVersions.Tools)
	if totalTools == 0 {
		fmt.Fprintf(os.Stderr, "No tools found in %s\n", toolVersionsPath)
		return nil
	}

	installedCount := 0
	failedCount := 0
	alreadyInstalledCount := 0

	toolList := make([]struct {
		tool    string
		version string
		owner   string
		repo    string
	}, 0, totalTools)
	for tool, versions := range toolVersions.Tools {
		owner, repo, err := installer.parseToolSpec(tool)
		if err != nil {
			continue
		}
		for _, version := range versions {
			toolList = append(toolList, struct {
				tool    string
				version string
				owner   string
				repo    string
			}{tool, version, owner, repo})
		}
	}

	spinner := bspinner.New()
	spinner.Spinner = bspinner.Dot
	spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	progressBar := progress.New(progress.WithDefaultGradient())

	for i, tool := range toolList {
		_, err := installer.findBinaryPath(tool.owner, tool.repo, tool.version)
		if err == nil && !reinstallFlag {
			msg := fmt.Sprintf("%s Skipped %s/%s@%s (already installed)", checkMark.Render(), tool.owner, tool.repo, tool.version)
			percent := float64(i+1) / float64(len(toolList))
			bar := progressBar.ViewAs(percent)
			resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
			fmt.Fprintln(os.Stderr, msg)
			// Show animated progress for a moment
			for j := 0; j < 5; j++ {
				printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
				spinner, _ = spinner.Update(bspinner.TickMsg{})
				time.Sleep(100 * time.Millisecond)
			}
			alreadyInstalledCount++
			continue
		}

		err = InstallSingleTool(tool.owner, tool.repo, tool.version, tool.version == "latest", false)
		var msg string
		if err == nil {
			msg = fmt.Sprintf("%s Installed %s/%s@%s", checkMark.Render(), tool.owner, tool.repo, tool.version)
			installedCount++
		} else {
			msg = fmt.Sprintf("%s Install failed %s/%s@%s: %v", xMark.Render(), tool.owner, tool.repo, tool.version, err)
			failedCount++
		}
		percent := float64(i+1) / float64(len(toolList))
		bar := progressBar.ViewAs(percent)
		resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
		fmt.Fprintln(os.Stderr, msg)
		// Show animated progress for a moment
		for j := 0; j < 5; j++ {
			printProgressBar(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s %s", spinner.View(), bar))
			spinner, _ = spinner.Update(bspinner.TickMsg{})
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Stop spinner animation
	// spinnerDone.Store(true) // This line is removed as per the new_code

	// At the end, clear the progress bar line before printing the summary
	resetLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())))
	fmt.Fprintln(os.Stderr)
	if totalTools == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s No tools to install", checkMark.Render()))
	} else if failedCount == 0 && alreadyInstalledCount == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools", checkMark.Render(), installedCount))
	} else if failedCount == 0 && alreadyInstalledCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, skipped %d", checkMark.Render(), installedCount, alreadyInstalledCount))
	} else if failedCount > 0 && alreadyInstalledCount == 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, failed %d", xMark.Render(), installedCount, failedCount))
	} else if failedCount > 0 && alreadyInstalledCount > 0 {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, failed %d, skipped %d", xMark.Render(), installedCount, failedCount, alreadyInstalledCount))
	} else {
		printStatusLine(os.Stderr, term.IsTerminal(int(os.Stderr.Fd())), fmt.Sprintf("%s Installed %d tools, failed %d, skipped %d", checkMark.Render(), installedCount, failedCount, alreadyInstalledCount))
	}

	return nil
}
