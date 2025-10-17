package version

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v59/github"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
)

var showFormat string

type showModel struct {
	spinner spinner.Model
	release *github.RepositoryRelease
	err     error
	done    bool
}

func (m *showModel) Init() tea.Cmd {
	return m.spinner.Tick
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

// fetchReleaseWithSpinner fetches a release with a spinner if TTY is available.
func fetchReleaseWithSpinner(client GitHubClient, versionArg string) (*github.RepositoryRelease, error) {
	// Check if we have a TTY for the spinner.
	//nolint:nestif // Spinner logic requires nested conditions for TTY check.
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		// Create spinner model.
		s := spinner.New()
		s.Spinner = spinner.Dot

		// Fetch release with spinner.
		m := &showModel{spinner: s}
		p := tea.NewProgram(m)

		// Fetch release in background.
		go func() {
			var release *github.RepositoryRelease
			var err error

			// Handle "latest" keyword.
			if strings.ToLower(versionArg) == "latest" {
				release, err = client.GetLatestRelease("cloudposse", "atmos")
			} else {
				release, err = client.GetRelease("cloudposse", "atmos", versionArg)
			}

			if err != nil {
				p.Send(err)
			} else {
				p.Send(release)
			}
		}()

		// Run the spinner.
		finalModel, err := p.Run()
		if err != nil {
			return nil, err
		}

		// Get the final model.
		final := finalModel.(*showModel)
		if final.err != nil {
			return nil, final.err
		}

		return final.release, nil
	}

	// No TTY - fetch without spinner.
	var release *github.RepositoryRelease
	var err error

	if strings.ToLower(versionArg) == "latest" {
		release, err = client.GetLatestRelease("cloudposse", "atmos")
	} else {
		release, err = client.GetRelease("cloudposse", "atmos", versionArg)
	}

	return release, err
}

var showCmd = &cobra.Command{
	Use:   "show <version>",
	Short: "Show details for a specific Atmos release",
	Long:  `Display detailed information about a specific Atmos release including release notes and download links.`,
	Example: `  # Show details for a specific version
  atmos version show v1.95.0

  # Show details for the latest release
  atmos version show latest

  # Output as JSON
  atmos version show v1.95.0 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		versionArg := args[0]

		// Create GitHub client.
		client := &RealGitHubClient{}

		// Fetch release (with or without spinner depending on TTY).
		release, err := fetchReleaseWithSpinner(client, versionArg)
		if err != nil {
			return err
		}

		// Format output.
		switch strings.ToLower(showFormat) {
		case "text":
			formatReleaseDetailText(release)
			return nil
		case "json":
			return formatReleaseDetailJSON(release)
		case "yaml":
			return formatReleaseDetailYAML(release)
		default:
			return fmt.Errorf("%w: %s (supported: text, json, yaml)", errUtils.ErrUnsupportedOutputFormat, showFormat)
		}
	},
}

func init() {
	showCmd.Flags().StringVar(&showFormat, "format", "text", "Output format: text, json, yaml")
	versionCmd.AddCommand(showCmd)
}
