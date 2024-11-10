package exec

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
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

type pkgAtmosVendor struct {
	uri               string
	name              string
	targetPath        string
	sourceIsLocalFile bool
	pkgType           pkgType
	version           string
	atmosVendorSource schema.AtmosVendorSource
}
type modelAtmosVendorInternal struct {
	packages  []pkgAtmosVendor
	index     int
	width     int
	height    int
	spinner   spinner.Model
	progress  progress.Model
	done      bool
	dryRun    bool
	failedPkg int
	cliConfig schema.CliConfiguration
}

var (
	currentPkgNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("211"))
	doneStyle           = lipgloss.NewStyle().Margin(1, 2)
	checkMark           = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	xMark               = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).SetString("x")
)

func newModelAtmosVendorInternal(pkg []pkgAtmosVendor, dryRun bool, cliConfig schema.CliConfiguration) (modelAtmosVendorInternal, error) {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(30),
		progress.WithoutPercentage(),
	)
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	if len(pkg) == 0 {
		return modelAtmosVendorInternal{}, nil
	}
	return modelAtmosVendorInternal{
		packages:  pkg,
		spinner:   s,
		progress:  p,
		dryRun:    dryRun,
		cliConfig: cliConfig,
	}, nil
}

func (m modelAtmosVendorInternal) Init() tea.Cmd {
	if len(m.packages) == 0 {
		m.done = true
		return nil
	}
	return tea.Batch(downloadAndInstall(m.packages[0], m.dryRun, m.cliConfig), m.spinner.Tick)
}
func (m modelAtmosVendorInternal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

		if msg.err != nil {
			u.LogDebug(m.cliConfig, fmt.Sprintf("Failed to vendor component %s error %s", pkg.name, msg.err))
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
			return m, tea.Sequence(
				tea.Printf("%s %s %s", mark, pkg.name, version),
				tea.Quit,
			)
		}

		// Update progress bar
		m.index++
		progressCmd := m.progress.SetPercent(float64(m.index) / float64(len(m.packages)))
		return m, tea.Batch(
			progressCmd,
			tea.Printf("%s %s %s", mark, pkg.name, version),                // print success message above our program
			downloadAndInstall(m.packages[m.index], m.dryRun, m.cliConfig), // download the next package
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

func (m modelAtmosVendorInternal) View() string {
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
func downloadAndInstall(p pkgAtmosVendor, dryRun bool, cliConfig schema.CliConfiguration) tea.Cmd {
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
		tempDir, err := os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
		if err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to create temp directory %s", err))
			return err
		}
		defer removeTempDir(cliConfig, tempDir)

		switch p.pkgType {
		case pkgTypeRemote:
			// Use go-getter to download remote packages
			client := &getter.Client{
				Ctx:  context.Background(),
				Dst:  tempDir,
				Src:  p.uri,
				Mode: getter.ClientModeAny,
			}
			if err := client.Get(); err != nil {
				u.LogTrace(cliConfig, fmt.Sprintf("Failed to download package %s error %s", p.name, err))
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}

		case pkgTypeOci:
			// Process OCI images
			if err := processOciImage(cliConfig, p.uri, tempDir); err != nil {
				u.LogTrace(cliConfig, fmt.Sprintf("Failed to process OCI image %s error %s", p.name, err))
				return installedPkgMsg{
					err:  err,
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
				u.LogTrace(cliConfig, fmt.Sprintf("Failed to copy package %s error %s", p.name, err))
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}
		default:
			u.LogTrace(cliConfig, fmt.Sprintf("Unknown package type %s", p.name))
			return installedPkgMsg{
				err:  fmt.Errorf("unknown package type"),
				name: p.name,
			}

		}
		if err := copyToTarget(cliConfig, tempDir, p.targetPath, p.atmosVendorSource, p.sourceIsLocalFile, p.uri); err != nil {
			u.LogTrace(cliConfig, fmt.Sprintf("Failed to copy package %s error %s", p.name, err))
			return installedPkgMsg{
				err:  err,
				name: p.name,
			}
		}
		return installedPkgMsg{
			err:  nil,
			name: p.name,
		}
	}
}
