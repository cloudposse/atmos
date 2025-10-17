package version

import (
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
)

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
)

type listModel struct {
	spinner  spinner.Model
	releases []*github.RepositoryRelease
	err      error
	done     bool
}

func (m *listModel) Init() tea.Cmd {
	return m.spinner.Tick
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

// fetchReleasesWithSpinner fetches releases with a spinner if TTY is available.
func fetchReleasesWithSpinner(client GitHubClient, opts ReleaseOptions) ([]*github.RepositoryRelease, error) {
	// Check if we have a TTY for the spinner.
	//nolint:nestif // Spinner logic requires nested conditions for TTY check.
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		// Create spinner model.
		s := spinner.New()
		s.Spinner = spinner.Dot

		// Fetch releases with spinner.
		m := &listModel{spinner: s}
		p := tea.NewProgram(m)

		// Fetch releases in background.
		go func() {
			releases, err := client.GetReleases("cloudposse", "atmos", opts)
			if err != nil {
				p.Send(err)
			} else {
				p.Send(releases)
			}
		}()

		// Run the spinner.
		finalModel, err := p.Run()
		if err != nil {
			return nil, err
		}

		// Get the final model.
		final := finalModel.(*listModel)
		if final.err != nil {
			return nil, final.err
		}

		return final.releases, nil
	}

	// No TTY - fetch without spinner.
	return client.GetReleases("cloudposse", "atmos", opts)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Atmos releases",
	Long:  `List available Atmos releases from GitHub with pagination and filtering options.`,
	Example: `  # List the last 10 releases (default)
  atmos version list

  # List the last 20 releases
  atmos version list --limit 20

  # List releases starting from offset 10
  atmos version list --offset 10

  # Include pre-releases
  atmos version list --include-prereleases

  # List releases since a specific date
  atmos version list --since 2025-01-01

  # Output as JSON
  atmos version list --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate limit.
		if listLimit < 1 || listLimit > listMaxLimit {
			return fmt.Errorf("%w: got %d", errUtils.ErrInvalidLimit, listLimit)
		}

		// Parse since date if provided.
		var sinceTime *time.Time
		if listSince != "" {
			parsed, err := time.Parse("2006-01-02", listSince)
			if err != nil {
				return fmt.Errorf("invalid date format for --since: %w (expected YYYY-MM-DD)", err)
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
		switch strings.ToLower(listFormat) {
		case "text":
			return formatReleaseListText(releases)
		case "json":
			return formatReleaseListJSON(releases)
		case "yaml":
			return formatReleaseListYAML(releases)
		default:
			return fmt.Errorf("%w: %s (supported: text, json, yaml)", errUtils.ErrUnsupportedOutputFormat, listFormat)
		}
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", listDefaultLimit, "Maximum number of releases to display (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of releases to skip")
	listCmd.Flags().StringVar(&listSince, "since", "", "Only show releases published after this date (ISO 8601 format: YYYY-MM-DD)")
	listCmd.Flags().BoolVar(&listIncludePrereleases, "include-prereleases", false, "Include pre-release versions")
	listCmd.Flags().StringVar(&listFormat, "format", "text", "Output format: text, json, yaml")

	versionCmd.AddCommand(listCmd)
}
