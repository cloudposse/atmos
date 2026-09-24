package toolchain

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	bspinner "github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// DefaultInstallMaxConcurrency is the conservative default for independent
	// downloads and extraction work in a batch install.
	DefaultInstallMaxConcurrency = 4
	spinnerAnimationFrames       = 5 // Number of animation frames for progress spinner.

	// Install result status constants.
	resultInstalled = "installed"
	resultFailed    = "failed"
	resultSkipped   = "skipped"
)

// InstallOptions configures the behavior of InstallSingleTool.
type InstallOptions struct {
	IsLatest               bool // Whether this is a "latest" version install.
	ShowProgressBar        bool // Whether to show spinner during install.
	ShowInstallDetails     bool // Whether to show detailed install messages (path, size, registered).
	ShowHint               bool // Whether to show PATH export hint after install.
	SkipToolVersionsUpdate bool // Skip .tool-versions update (caller handles it).
}

// Bubble Tea spinner model.
type spinnerModel struct {
	spinner bspinner.Model
	message string
	done    bool
}

// initialSpinnerModel creates the themed installation spinner with its status message.
func initialSpinnerModel(message string) *spinnerModel {
	s := ui.NewSpinner()

	styles := theme.GetCurrentStyles()
	s.Style = styles.Spinner

	return &spinnerModel{
		spinner: s,
		message: message,
	}
}

// Init, Update, and View are TUI frame callbacks invoked on every tick.
// perf.Track is intentionally omitted to avoid hot-path overhead.

func (m *spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	if m.done {
		return ""
	}
	return fmt.Sprintf("%s %s", m.spinner.View(), m.message)
}

// Custom message type for signaling installation completion.
type installDoneMsg struct{}

// runBubbleTeaSpinner creates and returns a Bubble Tea program with a spinner.
func runBubbleTeaSpinner(message string) *tea.Program {
	return tea.NewProgram(initialSpinnerModel(message), tea.WithOutput(os.Stderr))
}

// RunInstall installs the specified tool (owner/repo@version or alias@version).
// If toolSpec is empty, installs all tools from .tool-versions file.
// The setAsDefault parameter controls whether to set the installed version as default.
// The reinstallFlag parameter forces reinstallation even if already installed.
// The showHint parameter controls whether to show the PATH export hint message.
// The showProgressBar parameter controls whether to show spinner and success messages.
//
// Special format: pr:NNNN - installs Atmos from a PR's build artifact.
// Example: atmos version install pr:2038.
func RunInstall(toolSpec string, setAsDefault, reinstallFlag, showHint, showProgressBar bool) error {
	return runInstall(toolSpec, installRequest{
		setAsDefault: setAsDefault, reinstall: reinstallFlag, showHint: showHint,
		showProgressBar: showProgressBar, updateVersions: true,
	})
}

// RunAutomaticInstall installs a dependency without changing declared tool versions.
// Verified artifacts still participate in the configured lockfile policy.
func RunAutomaticInstall(toolSpec string) error {
	defer perf.Track(nil, "toolchain.RunAutomaticInstall")()

	return runInstall(toolSpec, installRequest{showProgressBar: true})
}

type installRequest struct {
	setAsDefault, reinstall, showHint, showProgressBar, updateVersions bool
}

func runInstall(toolSpec string, opts installRequest) error {
	defer perf.Track(nil, "toolchain.Install")()

	if toolSpec == "" {
		return installFromToolVersions(GetToolVersionsFilePath(), opts.reinstall, opts.showHint)
	}
	if err := validateFrozenToolSpec(toolSpec); err != nil {
		return err
	}
	tool, version, err := ParseToolVersionArg(toolSpec)
	if err != nil {
		return err
	}
	if handled, err := installDevelopmentVersion(toolSpec, version, opts.showProgressBar); handled {
		return err
	}
	// Resolve version if not specified.
	if version == "" {
		lookupResult, err := resolveVersionFromToolVersions(tool, toolSpec)
		if err != nil {
			return err
		}
		tool, version = lookupResult.tool, lookupResult.version
	}
	// Validate tool and version.
	if err := validateToolAndVersion(tool, version, toolSpec); err != nil {
		return err
	}
	return installRelease(tool, version, opts)
}

// installDevelopmentVersion handles both bare specs (pr:2038, sha:ceb7526)
// and named specs (atmos@pr:2038, atmos@sha:ceb7526).
func installDevelopmentVersion(toolSpec, version string, showProgress bool) (bool, error) {
	if version != "" {
		toolSpec = version
	}
	if prNumber, isPR := IsPRVersion(toolSpec); isPR {
		_, err := InstallFromPR(prNumber, showProgress)
		return true, err
	}
	if sha, isSHA := IsSHAVersion(toolSpec); isSHA {
		_, err := InstallFromSHA(sha, showProgress)
		return true, err
	}
	return false, nil
}

func installRelease(tool, version string, opts installRequest) error {
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
		ShowProgressBar:        opts.showProgressBar,
		ShowInstallDetails:     opts.showProgressBar, // Single-tool mode shows verbose output.
		ShowHint:               opts.showHint,
		SkipToolVersionsUpdate: true,
	})
	if err != nil {
		return err
	}

	if !opts.updateVersions || (GetAtmosConfig() != nil && GetAtmosConfig().Toolchain.FrozenLockFile) {
		return nil
	}

	// Update .tool-versions file only for explicit management commands.
	return updateToolVersionsFile(tool, version, opts.setAsDefault)
}

func validateFrozenToolSpec(toolSpec string) error {
	if config := GetAtmosConfig(); config != nil && config.Toolchain.FrozenLockFile {
		_, version, err := ParseToolVersionArg(toolSpec)
		if err != nil {
			return err
		}
		if version == "" {
			version = toolSpec
		}
		if kind, _, err := ParseVersionSpec(version); err == nil && kind != VersionTypeSemver {
			return fmt.Errorf("%w: development artifacts do not have toolchain lock entries", errUtils.ErrFrozenLockfile)
		}
	}

	return nil
}

func InstallSingleTool(owner, repo, version string, opts InstallOptions) error {
	defer perf.Track(nil, "toolchain.InstallSingleTool")()

	return installSingleToolWithInstaller(NewInstaller(), owner, repo, version, opts)
}

// installSingleToolWithInstaller preserves the complete single-install lifecycle
// while allowing concurrent batches to retain their worker-specific callbacks.
func installSingleToolWithInstaller(installer *Installer, owner, repo, version string, opts InstallOptions) error {
	if config := GetAtmosConfig(); config != nil && config.Toolchain.FrozenLockFile && version == "latest" {
		return fmt.Errorf("%w: latest is not an exact version", errUtils.ErrFrozenLockfile)
	}

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
			ui.Errorf("Install failed `%s/%s@%s`: %v", owner, repo, version, err)
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
		showMessage:            opts.ShowInstallDetails,
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
		ui.Writef("No tools found in %s\n", toolVersionsPath)
		return nil
	}
	return installToolList(toolList, reinstallFlag, showHint, DefaultInstallMaxConcurrency)
}

// RunInstallFromToolVersions installs every configured tool with bounded
// concurrency. It is used by the command path; RunInstall retains its legacy
// signature for callers that need the default behavior.
func RunInstallFromToolVersions(reinstallFlag, showHint bool, maxConcurrency int) error {
	defer perf.Track(nil, "toolchain.RunInstallFromToolVersions")()

	toolVersionsPath := GetToolVersionsFilePath()
	installer := NewInstaller()
	toolVersions, err := LoadToolVersions(toolVersionsPath)
	if err != nil {
		return fmt.Errorf("failed to load .tool-versions: %w", err)
	}
	toolList := buildToolList(installer, toolVersions)
	if len(toolList) == 0 {
		ui.Writef("No tools found in %s\n", toolVersionsPath)
		return nil
	}
	return installToolList(toolList, reinstallFlag, showHint, maxConcurrency)
}

// installToolList installs or skips requested tools, respecting concurrency limits and reporting results.
func installToolList(toolList []toolInfo, reinstallFlag, showHint bool, maxConcurrency int) error {
	if maxConcurrency < 1 {
		return fmt.Errorf("%w: max concurrency must be at least 1", errUtils.ErrInvalidFlagValue)
	}
	if maxConcurrency > 1 && len(toolList) > 1 {
		return installToolListConcurrently(toolList, reinstallFlag, showHint, maxConcurrency)
	}

	spinner := ui.NewSpinner()

	styles := theme.GetCurrentStyles()
	spinner.Style = styles.Spinner
	progressBar := ui.NewProgress()

	var installedCount, failedCount, alreadyInstalledCount int

	installer := NewInstaller()
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
	if failedCount > 0 {
		return fmt.Errorf("%w: %d tool installation(s) failed", errUtils.ErrToolInstall, failedCount)
	}
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
	return installOrSkipToolWithProgress(installer, tool, reinstallFlag, showHint, true)
}

func installOrSkipToolWithProgress(installer *Installer, tool toolInfo, reinstallFlag, showHint, showProgress bool) (string, error) {
	// Reject unsupported version formats (e.g. a hand-edited range/constraint in
	// .tool-versions) before any lookup or network call, with a clear per-tool error
	// instead of a raw HTTP 404 from treating the value as a literal release tag.
	if err := ValidateVersionSpec(tool.version); err != nil {
		return resultFailed, err
	}

	_, err := installer.FindBinaryPath(tool.owner, tool.repo, tool.version)
	if err == nil && !reinstallFlag {
		return resultSkipped, nil
	}
	err = installSingleToolWithInstaller(installer, tool.owner, tool.repo, tool.version, InstallOptions{
		IsLatest:               tool.version == "latest",
		ShowProgressBar:        showProgress,
		ShowInstallDetails:     false, // Batch mode - showProgress handles the simple message.
		SkipToolVersionsUpdate: true,  // Dependencies and manifest installs never declare new tools.
		ShowHint:               showHint,
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

	printToolResult(tool, state.result, state.err)

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

func printToolResult(tool toolInfo, result string, err error) {
	switch result {
	case resultSkipped:
		ui.Successf("Skipped `%s/%s@%s` (already installed)", tool.owner, tool.repo, tool.version)
	case resultInstalled:
		ui.Successf("Installed `%s/%s@%s`", tool.owner, tool.repo, tool.version)
	case resultFailed:
		ui.Errorf("Install failed `%s/%s@%s`: %v", tool.owner, tool.repo, tool.version, err)
	}
}

func printSummary(installed, failed, skipped, total int, showHint bool) {
	// Clear progress bar line before printing summary.
	resetLine()
	ui.Writeln("")

	if total == 0 {
		ui.Success("No tools to install")
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
		ui.Errorf("Installed %d tools, failed %d", installed, failed)
	} else {
		ui.Errorf("Installed %d tools, failed %d, skipped %d", installed, failed, skipped)
	}
}

func printSuccessSummary(installed, skipped int, showHint bool) {
	if skipped == 0 {
		ui.Successf("Installed **%d** tools", installed)
	} else {
		ui.Successf("Installed **%d** tools, skipped **%d**", installed, skipped)
	}
	if showHint {
		ui.Hint(getPlatformPathHint())
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
	return RunInstallBatchWithOptions(toolSpecs, BatchInstallOptions{
		Reinstall:      reinstallFlag,
		MaxConcurrency: DefaultInstallMaxConcurrency,
	})
}

// BatchInstallOptions controls batch installation without changing the
// long-standing RunInstallBatch API used by dependencies and integrations.
type BatchInstallOptions struct {
	SkipToolVersionsUpdate bool
	Reinstall              bool
	ShowHint               bool
	MaxConcurrency         int
}

// RunAutomaticInstallBatch installs execution dependencies without editing declarations.
func RunAutomaticInstallBatch(toolSpecs []string, reinstall bool) error {
	defer perf.Track(nil, "toolchain.RunAutomaticInstallBatch")()

	return RunInstallBatchWithOptions(toolSpecs, BatchInstallOptions{
		Reinstall:              reinstall,
		SkipToolVersionsUpdate: true,
	})
}

// RunInstallBatchWithOptions installs an explicit tool list with the supplied
// bounded concurrency configuration.
func RunInstallBatchWithOptions(toolSpecs []string, opts BatchInstallOptions) error {
	defer perf.Track(nil, "toolchain.RunInstallBatchWithOptions")()

	if len(toolSpecs) == 0 {
		return nil
	}
	if opts.MaxConcurrency == 0 {
		opts.MaxConcurrency = DefaultInstallMaxConcurrency
	}
	if opts.MaxConcurrency < 1 {
		return fmt.Errorf("%w: max concurrency must be at least 1", errUtils.ErrInvalidFlagValue)
	}
	if len(toolSpecs) == 1 {
		return runInstall(toolSpecs[0], installRequest{
			reinstall: opts.Reinstall, showHint: opts.ShowHint, showProgressBar: true,
			updateVersions: !opts.SkipToolVersionsUpdate,
		})
	}
	return installMultipleToolsWithOptions(toolSpecs, opts)
}

// installMultipleTools handles batch installation with progress bar.
func installMultipleTools(toolSpecs []string, reinstallFlag bool) error {
	return installMultipleToolsWithOptions(toolSpecs, BatchInstallOptions{
		Reinstall:      reinstallFlag,
		MaxConcurrency: DefaultInstallMaxConcurrency,
	})
}

func installMultipleToolsWithOptions(toolSpecs []string, opts BatchInstallOptions) error {
	defer perf.Track(nil, "toolchain.installMultipleTools")()

	installer := NewInstaller()

	// Parse all tool specs first.
	var toolList []toolInfo
	for _, spec := range toolSpecs {
		tool, version, err := ParseToolVersionArg(spec)
		if err != nil {
			ui.Errorf("Invalid tool spec `%s`: %v", spec, err)
			continue
		}

		owner, repo, err := installer.ParseToolSpec(tool)
		if err != nil {
			ui.Errorf("Invalid tool `%s`: %v", tool, err)
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

	return installToolList(toolList, opts.Reinstall, opts.ShowHint, opts.MaxConcurrency)
}

type batchEvent struct {
	tool     toolInfo
	started  bool
	progress *downloadProgress
	result   string
	err      error
}

type downloadProgress struct {
	downloaded int64
	total      int64
}

type activeInstall struct {
	id int
	toolInfo
	downloadProgress
	phase string
}

// batchRenderer is the sole writer for a parallel batch. It keeps the existing
// result-toast style while reserving a small, redrawn area for active spinners.
// Worker goroutines never touch the terminal.
type batchRenderer struct {
	display *batch.Renderer
	active  []activeInstall
	total   int
	nextID  int
}

// newBatchRenderer creates the themed display for concurrent tool downloads and installation progress.
func newBatchRenderer(total int) *batchRenderer {
	return &batchRenderer{total: total, display: batch.New(total, true, batch.WithToolchainStyle())}
}

func (r *batchRenderer) ensureDisplay() {
	if r.display == nil {
		r.display = batch.New(r.total, true, batch.WithToolchainStyle())
	}
}

func (r *batchRenderer) start(tool toolInfo) {
	r.ensureDisplay()
	id := r.nextID
	r.nextID++
	r.active = append(r.active, activeInstall{id: id, toolInfo: tool, downloadProgress: downloadProgress{total: -1}, phase: "Downloading"})
	r.display.Update(&batch.Event{ID: id, Label: fmt.Sprintf("%s/%s@%s", tool.owner, tool.repo, tool.version), Phase: "Downloading"})
}

func (r *batchRenderer) complete(tool toolInfo, result string, err error) {
	r.ensureDisplay()
	for i, active := range r.active {
		if active.toolInfo == tool {
			r.active = append(r.active[:i], r.active[i+1:]...)
			r.display.Complete(active.id, func() { printToolResult(tool, result, err) })
			return
		}
	}
	id := r.nextID
	r.nextID++
	r.display.Complete(id, func() { printToolResult(tool, result, err) })
}

func (r *batchRenderer) updateProgress(tool toolInfo, download downloadProgress) {
	for i := range r.active {
		active := &r.active[i]
		if active.toolInfo != tool {
			continue
		}
		active.downloadProgress = download
		oldPhase := active.phase
		if download.total > 0 && download.downloaded >= download.total {
			active.phase = "Verifying"
		}
		if r.display != nil {
			if oldPhase != active.phase {
				r.display.Update(&batch.Event{ID: active.id, Label: fmt.Sprintf("%s/%s@%s", tool.owner, tool.repo, tool.version), Phase: active.phase, Downloaded: download.downloaded, Total: download.total})
			} else {
				r.display.Update(&batch.Event{ID: active.id, Bytes: true, Downloaded: download.downloaded, Total: download.total})
			}
		}
		return
	}
}

func (r *batchRenderer) tick() {
	if r.display != nil {
		r.display.Tick()
	}
}

func (r *batchRenderer) clear() {
	if r.display != nil {
		r.display.Clear()
	}
}

func rightAlignInstallProgress(left string, downloaded, total int64, terminalWidth int) string {
	return batch.AlignProgress(left, downloaded, total, terminalWidth)
}

func installToolListConcurrently(toolList []toolInfo, reinstallFlag, showHint bool, maxConcurrency int) error {
	events := startBatchInstallWorkers(toolList, reinstallFlag, maxConcurrency)
	display := newBatchDisplay(len(toolList))
	counts := collectBatchInstallEvents(events, display, len(toolList))
	display.clear()
	return counts.finish(len(toolList), showHint)
}

func collectBatchInstallEvents(events <-chan batchEvent, display *batchDisplay, total int) batchInstallCounts {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	var counts batchInstallCounts
	for counts.completed < total {
		select {
		case event, ok := <-events:
			if !ok {
				counts.failed += total - counts.completed
				counts.completed = total
				break
			}
			counts.handle(&event, display, total)
		case <-ticker.C:
			display.tick()
		}
	}
	return counts
}

func startBatchInstallWorkers(toolList []toolInfo, reinstallFlag bool, maxConcurrency int) <-chan batchEvent {
	jobs := make(chan toolInfo)
	events := make(chan batchEvent, len(toolList)*2)
	var workers sync.WaitGroup
	for range min(maxConcurrency, len(toolList)) {
		workers.Add(1)
		go runBatchInstallWorker(jobs, events, reinstallFlag, &workers)
	}
	go func() {
		for _, tool := range toolList {
			jobs <- tool
		}
		close(jobs)
		workers.Wait()
		close(events)
	}()
	return events
}

func runBatchInstallWorker(jobs <-chan toolInfo, events chan<- batchEvent, reinstallFlag bool, workers *sync.WaitGroup) {
	defer workers.Done()
	for tool := range jobs {
		events <- batchEvent{tool: tool, started: true}
		installer := NewInstaller(WithDownloadProgress(func(downloaded, total int64) {
			select {
			case events <- batchEvent{tool: tool, progress: &downloadProgress{downloaded: downloaded, total: total}}:
			default:
			}
		}))
		result, err := installOrSkipToolWithProgress(installer, tool, reinstallFlag, false, false)
		events <- batchEvent{tool: tool, result: result, err: err}
	}
}

type batchDisplay struct {
	renderer    *batchRenderer
	spinner     bspinner.Model
	progressBar progress.Model
}

// newBatchDisplay selects a live terminal renderer or a simpler display for non-TTY and debug output.
func newBatchDisplay(total int) *batchDisplay {
	display := &batchDisplay{}
	if isTTY() && log.GetLevel() > log.DebugLevel {
		display.renderer = newBatchRenderer(total)
		return display
	}
	display.spinner = ui.NewSpinner()

	display.progressBar = ui.NewProgress()
	return display
}

func (d *batchDisplay) start(tool toolInfo) {
	if d.renderer != nil {
		d.renderer.start(tool)
	}
}

func (d *batchDisplay) update(tool toolInfo, download downloadProgress) {
	if d.renderer != nil {
		d.renderer.updateProgress(tool, download)
	}
}

func (d *batchDisplay) complete(tool toolInfo, result string, err error, index, total int) {
	if d.renderer != nil {
		d.renderer.complete(tool, result, err)
		return
	}
	showProgress(&d.spinner, &d.progressBar, tool, progressState{index: index, total: total, result: result, err: err})
}

func (d *batchDisplay) tick() {
	if d.renderer != nil {
		d.renderer.tick()
	}
}

func (d *batchDisplay) clear() {
	if d.renderer != nil {
		d.renderer.clear()
	}
}

type batchInstallCounts struct {
	installed int
	failed    int
	skipped   int
	completed int
}

func (c *batchInstallCounts) handle(event *batchEvent, display *batchDisplay, total int) {
	if event.started {
		display.start(event.tool)
		return
	}
	if event.progress != nil {
		display.update(event.tool, *event.progress)
		return
	}
	c.completed++
	switch event.result {
	case resultInstalled:
		c.installed++
	case resultFailed:
		c.failed++
	case resultSkipped:
		c.skipped++
	}
	display.complete(event.tool, event.result, event.err, c.completed-1, total)
}

func (c *batchInstallCounts) finish(total int, showHint bool) error {
	printSummary(c.installed, c.failed, c.skipped, total, showHint)
	if c.failed > 0 {
		return fmt.Errorf("%w: %d tool installation(s) failed", errUtils.ErrToolInstall, c.failed)
	}
	return nil
}
