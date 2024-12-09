package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
)

type pkgType int

const (
	pkgTypeRemote pkgType = iota
	pkgTypeOci
	pkgTypeLocal
)

func (p pkgType) String() string {
	names := [...]string{"remote", "oci", "local"}
	if p < pkgTypeRemote || p > pkgTypeLocal {
		return "unknown"
	}
	return names[p]
}

type pkgVendor struct {
	name             string
	version          string
	atmosPackage     *pkgAtmosVendor
	componentPackage *pkgComponentVendor
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

type modelVendor struct {
	packages  []pkgVendor
	index     int
	width     int
	height    int
	spinner   spinner.Model
	progress  progress.Model
	done      bool
	dryRun    bool
	failedPkg int
	cliConfig schema.CliConfiguration
	isTTY     bool
}

var (
	currentPkgNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("211"))
	doneStyle           = lipgloss.NewStyle().Margin(1, 2)
	checkMark           = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	xMark               = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
	grayColor           = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func newModelAtmosVendorInternal(pkgs []pkgAtmosVendor, dryRun bool, cliConfig schema.CliConfiguration) (modelVendor, error) {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(30),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	if len(pkgs) == 0 {
		return modelVendor{done: true}, nil
	}
	tty := CheckTTYSupport()
	var vendorPks []pkgVendor
	for _, pkg := range pkgs {
		p := pkgVendor{
			name:         pkg.name,
			version:      pkg.version,
			atmosPackage: &pkg,
		}
		vendorPks = append(vendorPks, p)
	}
	return modelVendor{
		packages:  vendorPks,
		spinner:   s,
		progress:  p,
		dryRun:    dryRun,
		cliConfig: cliConfig,
		isTTY:     tty,
	}, nil
}

func (m *modelVendor) Init() tea.Cmd {
	if len(m.packages) == 0 {
		m.done = true
		return nil
	}
	return tea.Batch(ExecuteInstall(m.packages[0], m.dryRun, m.cliConfig), m.spinner.Tick)
}
func (m *modelVendor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.width > 120 {
			m.width = 120
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}

	case installedPkgMsg:
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
				u.LogError(m.cliConfig, errors.New(errMsg))
			}
			mark = xMark
			m.failedPkg++
		}
		version := ""
		if pkg.version != "" {
			version = fmt.Sprintf("(%s)", pkg.version)
		}
		if m.index >= len(m.packages)-1 {
			// Everything's been installed. We're done!
			m.done = true
			if !m.isTTY {
				u.LogInfo(m.cliConfig, fmt.Sprintf("%s %s %s", mark, pkg.name, version))
				if m.dryRun {
					u.LogInfo(m.cliConfig, "Done! Dry run completed. No components vendored.\n")
				}
				if m.failedPkg > 0 {
					u.LogInfo(m.cliConfig, fmt.Sprintf("Vendored %d components. Failed to vendor %d components.\n", len(m.packages)-m.failedPkg, m.failedPkg))
				}
				u.LogInfo(m.cliConfig, fmt.Sprintf("Vendored %d components.\n", len(m.packages)))
			}
			version := grayColor.Render(version)
			return m, tea.Sequence(
				tea.Printf("%s %s %s", mark, pkg.name, version),
				tea.Quit,
			)
		}
		if !m.isTTY {
			u.LogInfo(m.cliConfig, fmt.Sprintf("%s %s %s", mark, pkg.name, version))
		}
		m.index++
		// Update progress bar
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.packages)))
		if !m.isTTY {
			u.LogInfo(m.cliConfig, fmt.Sprintf("Pulling %s", m.packages[m.index].name))
		}
		version = grayColor.Render(version)
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s %s %s %s", mark, pkg.name, version, errMsg), // print message above our program
			ExecuteInstall(m.packages[m.index], m.dryRun, m.cliConfig), // download the next package
		)
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

func (m modelVendor) View() string {

	n := len(m.packages)
	w := lipgloss.Width(fmt.Sprintf("%d", n))
	if m.done {
		if m.dryRun {
			return doneStyle.Render("Done! Dry run completed. No components vendored.\n")
		}
		if m.failedPkg > 0 {
			return doneStyle.Render(fmt.Sprintf("Vendored %d components. Failed to vendor %d components.\n", n-m.failedPkg, m.failedPkg))
		}
		return doneStyle.Render(fmt.Sprintf("Vendored %d components.\n", n))
	}

	pkgCount := fmt.Sprintf(" %*d/%*d", w, m.index, w, n)
	spin := m.spinner.View() + " "
	prog := m.progress.View()
	cellsAvail := max(0, m.width-lipgloss.Width(spin+prog+pkgCount))
	if m.index >= len(m.packages) {
		return ""
	}
	pkgName := currentPkgNameStyle.Render(m.packages[m.index].name)

	info := lipgloss.NewStyle().MaxWidth(cellsAvail).Render("Pulling " + pkgName)

	cellsRemaining := max(0, m.width-lipgloss.Width(spin+info+prog+pkgCount))
	gap := strings.Repeat(" ", cellsRemaining)

	return spin + info + gap + prog + pkgCount
}

type installedPkgMsg struct {
	err  error
	name string
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func downloadAndInstall(p *pkgAtmosVendor, dryRun bool, cliConfig schema.CliConfiguration) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			// Simulate the action
			time.Sleep(500 * time.Millisecond)
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}

		// Create temp directory
		tempDir, err := os.MkdirTemp("", fmt.Sprintf("atmos-vendor-%d-*", time.Now().Unix()))
		if err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("failed to create temp directory: %w", err),
				name: p.name,
			}
		}
		// Ensure directory permissions are restricted
		if err := os.Chmod(tempDir, 0700); err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("failed to set temp directory permissions: %w", err),
				name: p.name,
			}
		}

		defer removeTempDir(cliConfig, tempDir)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		switch p.pkgType {
		case pkgTypeRemote:
			// Use go-getter to download remote packages
			client := &getter.Client{
				Ctx:  ctx,
				Dst:  tempDir,
				Src:  p.uri,
				Mode: getter.ClientModeAny,
			}
			if err := client.Get(); err != nil {
				return installedPkgMsg{
					err:  fmt.Errorf("failed to download package: %w", err),
					name: p.name,
				}
			}

		case pkgTypeOci:
			// Process OCI images
			if err := processOciImage(cliConfig, p.uri, tempDir); err != nil {
				return installedPkgMsg{
					err:  fmt.Errorf("failed to process OCI image: %w", err),
					name: p.name,
				}
			}

		case pkgTypeLocal:
			// Copy from local file system
			copyOptions := cp.Options{
				PreserveTimes: false,
				PreserveOwner: false,
				OnSymlink:     func(src string) cp.SymlinkAction { return cp.Deep },
			}
			if p.sourceIsLocalFile {
				tempDir = path.Join(tempDir, filepath.Base(p.uri))
			}
			if err := cp.Copy(p.uri, tempDir, copyOptions); err != nil {
				return installedPkgMsg{
					err:  fmt.Errorf("failed to copy package: %w", err),
					name: p.name,
				}
			}
		default:
			return installedPkgMsg{
				err:  fmt.Errorf("unknown package type %s for package %s", p.pkgType.String(), p.name),
				name: p.name,
			}

		}
		if err := copyToTarget(cliConfig, tempDir, p.targetPath, &p.atmosVendorSource, p.sourceIsLocalFile, p.uri); err != nil {
			return installedPkgMsg{
				err:  fmt.Errorf("failed to copy package: %w", err),
				name: p.name,
			}
		}
		return installedPkgMsg{
			err:  nil,
			name: p.name,
		}
	}
}

func ExecuteInstall(installer pkgVendor, dryRun bool, cliConfig schema.CliConfiguration) tea.Cmd {
	if installer.atmosPackage != nil {
		return downloadAndInstall(installer.atmosPackage, dryRun, cliConfig)
	}

	if installer.componentPackage != nil {
		return downloadComponentAndInstall(installer.componentPackage, dryRun, cliConfig)
	}

	// No valid package provided
	return func() tea.Msg {
		err := fmt.Errorf("no valid installer package provided for %s", installer.name)
		return installedPkgMsg{
			err:  err,
			name: installer.name,
		}
	}
}
