package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type pkgType int

const (
	tempDirPermissions         = 0o700
	progressBarWidth           = 30
	maxWidth                   = 120
	pkgTypeRemote      pkgType = iota
	pkgTypeOci
	pkgTypeLocal
)

var (
	currentPkgNameStyle = theme.Styles.PackageName
	doneStyle           = lipgloss.NewStyle().Margin(1, 2)
	checkMark           = theme.Styles.Checkmark
	xMark               = theme.Styles.XMark
	grayColor           = theme.Styles.GrayText
)

type installedPkgMsg struct {
	err  error
	name string
}

func (p pkgType) String() string {
	names := [...]string{"remote", "oci", "local"}
	if p < pkgTypeRemote || p > pkgTypeLocal {
		return "unknown"
	}
	return names[p]
}

type pkgVendorDiff struct {
	name           string
	currentVersion string
	latestVersion  string // Add field to store latest version from pre-check
	source         schema.AtmosVendorSource
	outdatedOnly   bool
}

type pkgVendor struct {
	name             string
	version          string
	atmosPackage     *pkgAtmosVendor
	componentPackage *pkgComponentVendor
	diffPackage      *pkgVendorDiff // Add diff package type
}

type pkgAtmosVendor struct {
	uri               string
	name              string
	targetPath        string
	sourceIsLocalFile bool
	pkgType           pkgType
	version           string
	atmosVendorSource schema.AtmosVendorSource
}
type pkgComponentVendor struct {
	uri                 string
	name                string
	sourceIsLocalFile   bool
	pkgType             pkgType
	version             string
	vendorComponentSpec *schema.VendorComponentSpec
	componentPath       string
	IsComponent         bool
	IsMixins            bool
	mixinFilename       string
}

type modelVendor struct {
	packages    []pkgVendor
	index       int
	width       int
	height      int
	spinner     spinner.Model
	progress    progress.Model
	done        bool
	dryRun      bool
	failedPkg   int
	atmosConfig *schema.AtmosConfiguration
	isTTY       bool
}

func executeVendorModel[T pkgComponentVendor | pkgAtmosVendor | pkgVendorDiff](
	packages []T,
	dryRun bool,
	atmosConfig *schema.AtmosConfiguration,
) error {
	if len(packages) == 0 {
		return nil
	}
	// Initialize model based on package type
	model, err := newModelVendor(packages, dryRun, atmosConfig)
	if err != nil {
		return fmt.Errorf("%w: %v (verify terminal capabilities and permissions)", errUtils.ErrTUIModel, err)
	}

	var opts []tea.ProgramOption
	if !term.IsTTYSupportForStdout() {
		opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
		log.Debug("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
	}

	if _, err := tea.NewProgram(&model, opts...).Run(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	if model.failedPkg > 0 {
		return fmt.Errorf("%w: %d", ErrVendorComponents, model.failedPkg)
	}
	return nil
}

func newModelVendor[T pkgComponentVendor | pkgAtmosVendor | pkgVendorDiff](
	pkgs []T,
	dryRun bool,
	atmosConfig *schema.AtmosConfiguration,
) (modelVendor, error) {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = theme.Styles.Link

	if len(pkgs) == 0 {
		return modelVendor{done: true}, nil
	}

	vendorPks := make([]pkgVendor, len(pkgs))

	// Determine type once using first element
	switch any(pkgs[0]).(type) {
	case pkgComponentVendor:
		for i := range pkgs {
			// Get original element from slice
			cp := any(pkgs[i]).(pkgComponentVendor)
			vendorPks[i] = pkgVendor{
				name:             cp.name,
				version:          cp.version,
				componentPackage: &cp,
			}
		}
	case pkgAtmosVendor:
		for i := range pkgs {
			ap := any(pkgs[i]).(pkgAtmosVendor)
			vendorPks[i] = pkgVendor{
				name:         ap.name,
				version:      ap.version,
				atmosPackage: &ap,
			}
		}
	case pkgVendorDiff:
		for i := range pkgs {
			dp := any(pkgs[i]).(pkgVendorDiff)
			vendorPks[i] = pkgVendor{
				name:        dp.name,
				version:     dp.currentVersion,
				diffPackage: &dp,
			}
		}
	}

	return modelVendor{
		packages:    vendorPks,
		spinner:     s,
		progress:    p,
		dryRun:      dryRun,
		atmosConfig: atmosConfig,
		isTTY:       term.IsTTYSupportForStdout(),
	}, nil
}

func (m *modelVendor) Init() tea.Cmd {
	if len(m.packages) == 0 {
		m.done = true
		return nil
	}
	return tea.Batch(ExecuteInstall(m.packages[0], m.dryRun, m.atmosConfig), m.spinner.Tick)
}

func (m *modelVendor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.width > maxWidth {
			m.width = maxWidth
		}

	case tea.KeyMsg:
		if cmd := m.handleKeyPress(msg); cmd != nil {
			return m, cmd
		}

	case installedPkgMsg:
		return m.handleInstalledPkgMsg(&msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		newModel, cmd := m.progress.Update(msg)
		if newModel, ok := newModel.(progress.Model); ok {
			m.progress = newModel
		}
		return m, cmd
	}
	return m, nil
}

func (m *modelVendor) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return tea.Quit
	}
	return nil
}

func (m *modelVendor) handleInstalledPkgMsg(msg *installedPkgMsg) (tea.Model, tea.Cmd) {
	// ensure index is within bounds
	if m.index >= len(m.packages) {
		return m, nil
	}
	pkg := m.packages[m.index]

	mark := checkMark
	errMsg := ""
	if msg.err != nil {
		errMsg = fmt.Sprintf("Failed to vendor %s: error : %s", pkg.name, msg.err)
		if !m.isTTY {
			log.Error(errMsg)
		}
		mark = xMark
		m.failedPkg++
	}
	version := ""
	if pkg.version != "" {
		version = fmt.Sprintf("(%s)", pkg.version)
	}

	// Skip empty messages (for outdated filtered items)
	if msg.name == "" {
		if m.index >= len(m.packages)-1 {
			m.done = true
			return m, tea.Quit
		}
		m.index++
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.packages)))
		return m, tea.Batch(
			progressCmd,
			ExecuteInstall(m.packages[m.index], m.dryRun, m.atmosConfig),
		)
	}

	if m.index >= len(m.packages)-1 {
		// Everything's been installed. We're done!
		m.done = true
		m.logNonNTYFinalStatus(pkg, &mark)

		// For diff packages, use the exact message
		if pkg.diffPackage != nil {
			return m, tea.Sequence(
				tea.Printf("%s", msg.name),
				tea.Quit,
			)
		}

		version := grayColor.Render(version)
		return m, tea.Sequence(
			tea.Printf("%s %s %s %s", mark, pkg.name, version, errMsg),
			tea.Quit,
		)
	}

	if !m.isTTY {
		log.Info(mark, "package", pkg.name, "version", version)
	}

	m.index++
	// Update progress bar
	progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.packages)))

	// For diff packages, use the exact message
	if pkg.diffPackage != nil {
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s", msg.name),
			ExecuteInstall(m.packages[m.index], m.dryRun, m.atmosConfig),
		)
	}

	version = grayColor.Render(version)
	return m, tea.Batch(
		progressCmd,
		tea.Printf("%s %s %s %s", mark, pkg.name, version, errMsg),
		ExecuteInstall(m.packages[m.index], m.dryRun, m.atmosConfig),
	)
}

func (m *modelVendor) logNonNTYFinalStatus(pkg pkgVendor, mark *lipgloss.Style) {
	if m.isTTY {
		return
	}

	version := ""
	if pkg.version != "" {
		version = fmt.Sprintf("(%s)", pkg.version)
	}
	log.Info(mark, "package", pkg.name, "version", version)

	if m.dryRun {
		log.Info("Done! Dry run completed. No components vendored")
	}

	if m.failedPkg > 0 {
		log.Info("Vendored components", "success", len(m.packages)-m.failedPkg, "failed", m.failedPkg)
	} else {
		log.Info("Vendored components", "success", len(m.packages))
	}
}

func (m *modelVendor) View() string {
	n := len(m.packages)
	w := lipgloss.Width(fmt.Sprintf("%d", n))
	if m.done {
		// Special handling for diff packages
		if len(m.packages) > 0 && m.packages[0].diffPackage != nil {
			// For vendor diff, show more appropriate message
			updateCount := 0
			for _, pkg := range m.packages {
				if pkg.diffPackage != nil && pkg.diffPackage.latestVersion != "" {
					updateCount++
				}
			}

			if updateCount > 0 {
				return doneStyle.Render(fmt.Sprintf("Found %d updates. Use --update flag to update the vendor configuration file.\n", updateCount))
			} else if m.packages[0].diffPackage.outdatedOnly {
				// For --outdated mode with no updates, this shouldn't normally be shown
				// (pre-filter should have caught it), but just in case
				return doneStyle.Render("No outdated vendor dependencies found.\n")
			} else {
				return doneStyle.Render("All vendor dependencies are up to date!\n")
			}
		}

		// Standard completion messages for non-diff operations
		if m.dryRun {
			return doneStyle.Render("Done! Dry run completed. No components vendored.\n")
		}
		if m.failedPkg > 0 {
			return doneStyle.Render(fmt.Sprintf("Vendored %d components. Failed to vendor %d components.\n", n-m.failedPkg, m.failedPkg))
		}
		return doneStyle.Render(fmt.Sprintf("Vendored %d components.\n", n))
	}

	pkgCount := fmt.Sprintf(" %*d/%*d", w, m.index+1, w, n)
	spin := m.spinner.View() + " "

	// Calculate progress percentage
	progressPercent := float64(m.index) / float64(n)
	prog := m.progress.ViewAs(progressPercent)

	cellsAvail := max(0, m.width-lipgloss.Width(spin+prog+pkgCount))

	if m.index >= len(m.packages) {
		return ""
	}
	pkgName := currentPkgNameStyle.Render(m.packages[m.index].name)

	// Determine action text based on package type
	actionText := "Pulling"
	if m.packages[m.index].diffPackage != nil {
		actionText = "Checking"
	}

	info := lipgloss.NewStyle().MaxWidth(cellsAvail).Render(actionText + " " + pkgName)

	cellsRemaining := max(0, m.width-lipgloss.Width(spin+info+prog+pkgCount))
	gap := strings.Repeat(" ", cellsRemaining)

	return spin + info + gap + prog + pkgCount
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func downloadAndInstall(p *pkgAtmosVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		log.Debug("Downloading and installing package", "package", p.name)
		if dryRun {
			return handleDryRunInstall(p, atmosConfig)
		}
		tempDir, err := createTempDir()
		if err != nil {
			return newInstallError(err, p.name)
		}

		defer removeTempDir(tempDir)
		if err := p.installer(&tempDir, atmosConfig); err != nil {
			return newInstallError(err, p.name)
		}
		if err := copyToTargetWithPatterns(tempDir, p.targetPath, &p.atmosVendorSource, p.sourceIsLocalFile); err != nil {
			return newInstallError(fmt.Errorf("failed to copy package: %w", err), p.name)
		}
		return installedPkgMsg{
			err:  nil,
			name: p.name,
		}
	}
}

func (p *pkgAtmosVendor) installer(tempDir *string, atmosConfig *schema.AtmosConfiguration) error {
	switch p.pkgType {
	case pkgTypeRemote:
		// Use go-getter to download remote packages

		if err := downloader.NewGoGetterDownloader(atmosConfig).Fetch(p.uri, *tempDir, downloader.ClientModeAny, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package: %w", err)
		}

	case pkgTypeOci:
		// Process OCI images
		if err := processOciImage(atmosConfig, p.uri, *tempDir); err != nil {
			return fmt.Errorf("failed to process OCI image: %w", err)
		}

	case pkgTypeLocal:
		// Copy from local file system
		copyOptions := cp.Options{
			PreserveTimes: false,
			PreserveOwner: false,
			OnSymlink:     func(src string) cp.SymlinkAction { return cp.Deep },
		}
		if p.sourceIsLocalFile {
			*tempDir = filepath.Join(*tempDir, SanitizeFileName(p.uri))
		}
		if err := cp.Copy(p.uri, *tempDir, copyOptions); err != nil {
			return fmt.Errorf("failed to copy package: %w", err)
		}
	default:
		return fmt.Errorf("%w %s for package %s", errUtils.ErrUnknownPackageType, p.pkgType.String(), p.name)
	}
	return nil
}

func handleDryRunInstall(p *pkgAtmosVendor, atmosConfig *schema.AtmosConfiguration) tea.Msg {
	log.Debug("Entering dry-run flow for generic (non component/mixin) vendoring ", "package", p.name)

	if needsCustomDetection(p.uri) {
		log.Debug("Custom detection required for URI", "uri", p.uri)
		detector := downloader.NewCustomGitDetector(atmosConfig, "")
		_, _, err := detector.Detect(p.uri, "")
		if err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("dry-run: detection failed: %w", err),
				name: p.name,
			}
		}
	} else {
		log.Debug("Skipping custom detection; URI already supported by go getter", "uri", p.uri)
	}

	time.Sleep(500 * time.Millisecond)
	return installedPkgMsg{
		err:  nil,
		name: p.name,
	}
}

// This is a replica of getForce method from go getter library, had to make it as it is not exported.
// The idea is to call Detect method in dry run only for those links where go getter does this.
// Otherwise, Detect is run for every link being vendored which isn't correct.
func needsCustomDetection(src string) bool {
	_, getSrc := "", src
	if idx := strings.Index(src, "::"); idx >= 0 {
		_, getSrc = src[:idx], src[idx+2:]
	}

	getSrc, _ = getter.SourceDirSubdir(getSrc)

	if absPath, err := filepath.Abs(getSrc); err == nil {
		if u.FileExists(absPath) {
			return false
		}
		isDir, err := u.IsDirectory(absPath)
		if err == nil && isDir {
			return false
		}
	}

	parsed, err := url.Parse(getSrc)
	if err != nil || parsed.Scheme == "" {
		return true
	}

	supportedSchemes := map[string]bool{
		"http":      true,
		"https":     true,
		"git":       true,
		"hg":        true,
		"s3":        true,
		"gcs":       true,
		"file":      true,
		"oci":       true,
		"ssh":       true,
		"git+ssh":   true,
		"git+https": true,
	}

	if _, ok := supportedSchemes[parsed.Scheme]; ok {
		return false
	}

	return true
}

func createTempDir() (string, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "atmos-vendor")
	if err != nil {
		return "", err
	}

	// Ensure directory permissions are restricted
	if err := os.Chmod(tempDir, tempDirPermissions); err != nil {
		return "", err
	}

	return tempDir, nil
}

func newInstallError(err error, name string) installedPkgMsg {
	return installedPkgMsg{
		err:  fmt.Errorf("%s: %w", name, err),
		name: name,
	}
}

func ExecuteInstall(installer pkgVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	if installer.atmosPackage != nil {
		return downloadAndInstall(installer.atmosPackage, dryRun, atmosConfig)
	}

	if installer.componentPackage != nil {
		return downloadComponentAndInstall(installer.componentPackage, dryRun, atmosConfig)
	}

	if installer.diffPackage != nil {
		return executeDiffCheck(installer.diffPackage, atmosConfig)
	}

	// No valid package provided
	return func() tea.Msg {
		err := fmt.Errorf("%w: %s", errUtils.ErrValidPackage, installer.name)
		return installedPkgMsg{
			err:  err,
			name: installer.name,
		}
	}
}

// executeDiffCheck performs the version diff check for a component
func executeDiffCheck(p *pkgVendorDiff, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		// If we already have the latest version from pre-check, use that directly
		if p.outdatedOnly && p.latestVersion != "" {
			// Format the result to show the update with pre-filled latest version
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("📦 %s: %s → %s", p.name, p.currentVersion, p.latestVersion),
			}
		}

		// Otherwise, check for updates using the existing logic
		updateAvailable, latestInfo, err := checkForVendorUpdates(p.source, true)

		if err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("Error checking for updates - %v", err),
				name: p.name,
			}
		}

		if updateAvailable && latestInfo != "" {
			p.latestVersion = latestInfo
			// Format the result to show the update
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("📦 %s: %s → %s", p.name, p.currentVersion, latestInfo),
			}
		} else if !p.outdatedOnly {
			return installedPkgMsg{
				err:  nil,
				name: fmt.Sprintf("%s %s: %s (up to date)", checkMark, p.name, p.currentVersion),
			}
		}

		// For outdatedOnly mode, don't show up-to-date components
		return installedPkgMsg{
			err:  nil,
			name: "", // Empty name means don't display
		}
	}
}
