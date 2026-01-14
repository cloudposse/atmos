package version

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v59/github"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
	"github.com/cloudposse/atmos/toolchain"
)

//go:embed markdown/atmos_version_list_usage.md
var listUsageMarkdown string

const (
	listDefaultLimit = 10
	listMaxLimit     = 100
)

var (
	listLimit              int
	listOffset             int
	listSince              string
	listIncludePrereleases bool
	listFormat             string
	listInstalled          bool
)

type listModel struct {
	spinner  spinner.Model
	releases []*github.RepositoryRelease
	err      error
	done     bool
	client   GitHubClient
	opts     ReleaseOptions
}

func (m *listModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchReleasesCmd(m.client, m.opts),
	)
}

func (m *listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case []*github.RepositoryRelease:
		m.releases = msg
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

func (m *listModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " Fetching releases from GitHub..."
}

// fetchReleasesCmd returns a command that fetches releases from GitHub.
func fetchReleasesCmd(client GitHubClient, opts ReleaseOptions) tea.Cmd {
	return func() tea.Msg {
		releases, err := client.GetReleases("cloudposse", "atmos", opts)
		if err != nil {
			return err
		}
		return releases
	}
}

// fetchReleasesWithSpinner fetches releases for the cloudposse/atmos repository and displays a spinner on stderr when a TTY is available.
// If a TTY is present, an interactive spinner is shown while fetching; any error from the spinner runtime or the fetch is returned.
// If no TTY is present, releases are fetched directly via the provided client and any client error is returned.
func fetchReleasesWithSpinner(client GitHubClient, opts ReleaseOptions) ([]*github.RepositoryRelease, error) {
	defer perf.Track(nil, "version.fetchReleasesWithSpinner")()

	// Check if we have a TTY for the spinner.
	//nolint:nestif // Spinner logic requires nested conditions for TTY check.
	if isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()) {
		// Create spinner model.
		s := spinner.New()
		s.Spinner = spinner.Dot
		s.Style = theme.GetCurrentStyles().Spinner

		// Fetch releases with spinner.
		m := &listModel{spinner: s, client: client, opts: opts}
		p := tea.NewProgram(m, tea.WithOutput(os.Stderr))

		// Run the spinner.
		finalModel, err := p.Run()
		if err != nil {
			return nil, fmt.Errorf("spinner execution failed: %w", err)
		}

		// Check for nil model.
		if finalModel == nil {
			return nil, fmt.Errorf("%w: spinner completed but returned nil model during releases fetch", errUtils.ErrSpinnerReturnedNilModel)
		}

		// Get the final model with type assertion safety.
		final, ok := finalModel.(*listModel)
		if !ok {
			return nil, fmt.Errorf("%w: got %T", errUtils.ErrSpinnerUnexpectedModelType, finalModel)
		}

		if final.err != nil {
			return nil, fmt.Errorf("failed to fetch releases: %w", final.err)
		}

		return final.releases, nil
	}

	// No TTY - fetch without spinner.
	return client.GetReleases("cloudposse", "atmos", opts)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List Atmos releases",
	Long:    `List available Atmos releases from GitHub with pagination and filtering options, or list locally installed versions with --installed.`,
	Example: listUsageMarkdown,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "version.list.RunE")()

		// Handle --installed flag.
		if listInstalled {
			return listInstalledVersions()
		}

		// Validate limit.
		if listLimit < 1 || listLimit > listMaxLimit {
			return fmt.Errorf("%w: got %d", errUtils.ErrInvalidLimit, listLimit)
		}

		// Validate offset.
		if listOffset < 0 {
			return fmt.Errorf("%w: got %d", errUtils.ErrInvalidOffset, listOffset)
		}

		// Validate format.
		normalizedFormat := strings.ToLower(listFormat)
		if normalizedFormat != "table" && normalizedFormat != "json" && normalizedFormat != "yaml" {
			return fmt.Errorf("%w: %s (supported: table, json, yaml)", errUtils.ErrInvalidFormat, listFormat)
		}

		// Parse since date if provided.
		var sinceTime *time.Time
		if listSince != "" {
			parsed, err := time.Parse("2006-01-02", listSince)
			if err != nil {
				return fmt.Errorf("%w: %q (expected YYYY-MM-DD): %v", errUtils.ErrInvalidSinceDate, listSince, err)
			}
			sinceTime = &parsed
		}

		// Create GitHub client.
		client := &RealGitHubClient{}

		// Fetch releases (with or without spinner depending on TTY).
		releases, err := fetchReleasesWithSpinner(client, ReleaseOptions{
			Limit:              listLimit,
			Offset:             listOffset,
			IncludePrereleases: listIncludePrereleases,
			Since:              sinceTime,
		})
		if err != nil {
			return err
		}

		// Format output.
		switch normalizedFormat {
		case "table":
			return formatReleaseListText(releases)
		case "json":
			return formatReleaseListJSON(releases)
		case "yaml":
			return formatReleaseListYAML(releases)
		default:
			return fmt.Errorf("%w: %s (supported: table, json, yaml)", errUtils.ErrInvalidFormat, listFormat)
		}
	},
}

// listInstalledVersions lists locally installed versions of Atmos using the toolchain table renderer.
func listInstalledVersions() error {
	defer perf.Track(nil, "version.listInstalledVersions")()

	return toolchain.RunListInstalledAtmosVersions(pkgversion.Version)
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", listDefaultLimit, "Maximum number of releases to display (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of releases to skip")
	listCmd.Flags().StringVar(&listSince, "since", "", "Only show releases published after this date (ISO 8601 format: YYYY-MM-DD)")
	listCmd.Flags().BoolVar(&listIncludePrereleases, "include-prereleases", false, "Include pre-release versions")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format: table, json, yaml")
	listCmd.Flags().BoolVar(&listInstalled, "installed", false, "List locally installed versions instead of GitHub releases")

	versionCmd.AddCommand(listCmd)
}
