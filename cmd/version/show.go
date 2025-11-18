package version

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v59/github"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:embed markdown/atmos_version_show_usage.md
var showUsageMarkdown string

var showFormat string

type showModel struct {
	spinner    spinner.Model
	release    *github.RepositoryRelease
	err        error
	done       bool
	client     GitHubClient
	versionArg string
}

func (m *showModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchReleaseCmd(m.client, m.versionArg),
	)
}

func (m *showModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case *github.RepositoryRelease:
		m.release = msg
		m.done = true
		return m, tea.Quit
	case error:
		m.err = msg
		m.done = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m *showModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " Fetching release from GitHub..."
}

// fetchReleaseCmd returns a command that fetches a release from GitHub.
func fetchReleaseCmd(client GitHubClient, versionArg string) tea.Cmd {
	return func() tea.Msg {
		release, err := fetchRelease(client, versionArg)
		if err != nil {
			return err
		}
		return release
	}
}

// fetchRelease fetches a release from GitHub API.
func fetchRelease(client GitHubClient, versionArg string) (*github.RepositoryRelease, error) {
	if strings.ToLower(versionArg) == "latest" {
		return client.GetLatestRelease("cloudposse", "atmos")
	}
	return client.GetRelease("cloudposse", "atmos", versionArg)
}

// fetchReleaseWithSpinner fetches a release with a spinner if TTY is available.
func fetchReleaseWithSpinner(client GitHubClient, versionArg string) (*github.RepositoryRelease, error) {
	defer perf.Track(nil, "version.fetchReleaseWithSpinner")()

	// Check if we have a TTY for the spinner.
	if !isatty.IsTerminal(os.Stderr.Fd()) && !isatty.IsCygwinTerminal(os.Stderr.Fd()) {
		// No TTY - fetch without spinner.
		release, err := fetchRelease(client, versionArg)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch release: %w", err)
		}
		return release, nil
	}

	// Create spinner model.
	s := spinner.New()
	s.Spinner = spinner.Dot

	// Fetch release with spinner.
	m := &showModel{spinner: s, client: client, versionArg: versionArg}
	p := tea.NewProgram(m, tea.WithOutput(os.Stderr))

	// Run the spinner.
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("spinner execution failed: %w", err)
	}

	// Check for nil model.
	if finalModel == nil {
		return nil, fmt.Errorf("%w: spinner completed but returned nil model during release fetch", errUtils.ErrSpinnerReturnedNilModel)
	}

	// Get the final model with type assertion safety.
	final, ok := finalModel.(*showModel)
	if !ok {
		return nil, fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
	}

	if final.err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", final.err)
	}

	return final.release, nil
}

var showCmd = &cobra.Command{
	Use:     "show [version]",
	Short:   "Show details for a specific Atmos release",
	Long:    `Display detailed information about a specific Atmos release including release notes and download links.`,
	Example: showUsageMarkdown,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.show.RunE")()

		// Default to latest if no version specified.
		versionArg := "latest"
		if len(args) > 0 {
			versionArg = args[0]
		}

		// Create GitHub client.
		client := &RealGitHubClient{}

		// Fetch release (with or without spinner depending on TTY).
		release, err := fetchReleaseWithSpinner(client, versionArg)
		if err != nil {
			return err
		}

		// Format output.
		switch strings.ToLower(showFormat) {
		case "table":
			if err := formatReleaseDetailText(release); err != nil {
				return err
			}
			return nil
		case "json":
			return formatReleaseDetailJSON(release)
		case "yaml":
			return formatReleaseDetailYAML(release)
		default:
			return fmt.Errorf("%w: %s (supported: table, json, yaml)", errUtils.ErrInvalidFormat, showFormat)
		}
	},
}

func init() {
	showCmd.Flags().StringVar(&showFormat, "format", "table", "Output format: table, json, yaml")
	versionCmd.AddCommand(showCmd)
}
