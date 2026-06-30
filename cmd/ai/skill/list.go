package skill

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ai "github.com/cloudposse/atmos/cmd/ai"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	listformat "github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/output"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// Status markers for the available-vs-installed view. A filled dot marks an
// installed skill; a hollow dot marks one that is available to install.
const (
	markerInstalled = "●"
	markerAvailable = "○"
)

// listParser handles flag parsing with Viper precedence for the list command.
var listParser *flags.StandardParser

//go:embed markdown/atmos_ai_skill_list.md
var listLongMarkdown string

//go:embed markdown/atmos_ai_skill_list_usage.md
var listUsageMarkdown string

// listEntry is the merged view of a skill: a catalog entry, an installed skill,
// or both. It powers the available-vs-installed listing.
type listEntry struct {
	name        string
	displayName string
	description string
	version     string
	source      string
	available   bool                        // True when part of the bundled catalog.
	installed   bool                        // True when installed locally.
	skill       *marketplace.InstalledSkill // Non-nil when installed.
}

// listCmd represents the 'atmos ai skill list' command.
var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List available and installed skills",
	Long:    listLongMarkdown,
	Example: listUsageMarkdown,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.aiSkillListCmd")()

		// Bind parsed flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flags from Viper (supports CLI > ENV > config > defaults).
		detailed := v.GetBool("detailed")
		installedOnly := v.GetBool("installed")

		// Create installer (which manages registry).
		installer, err := marketplace.NewInstaller(version.Version)
		if err != nil {
			return fmt.Errorf("failed to initialize installer: %w", err)
		}

		entries, err := buildListEntries(installer)
		if err != nil {
			return err
		}

		return renderSkillList(entries, installedOnly, detailed)
	},
}

func init() {
	// Create parser with list-specific flags using functional options.
	listParser = flags.NewStandardParser(
		flags.WithBoolFlag("detailed", "d", false, "Show detailed information for each skill"),
		flags.WithEnvVars("detailed", "ATMOS_AI_SKILL_DETAILED"),
		flags.WithBoolFlag("installed", "", false, "Show only installed skills"),
		flags.WithEnvVars("installed", "ATMOS_AI_SKILL_INSTALLED"),
	)

	// Register flags on the command.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper for environment variable support.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add 'list' subcommand to 'skill' command.
	ai.SkillCmd.AddCommand(listCmd)
}

// buildListEntries merges the bundled catalog with installed skills into a single
// alphabetically-sorted view. Catalog skills carry their installed version/source
// when installed; community skills not in the catalog are appended.
func buildListEntries(installer *marketplace.Installer) ([]listEntry, error) {
	catalog, err := marketplace.Catalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load skill catalog: %w", err)
	}

	installed := installer.List()
	byName := make(map[string]*marketplace.InstalledSkill, len(installed))
	for _, s := range installed {
		byName[s.Name] = s
	}

	seen := make(map[string]bool, len(catalog))
	entries := make([]listEntry, 0, len(catalog)+len(installed))

	for _, c := range catalog {
		seen[c.Name] = true
		e := listEntry{
			name:        c.Name,
			displayName: c.DisplayName,
			description: c.Description,
			version:     c.Version,
			source:      c.Source,
			available:   true,
		}
		if s, ok := byName[c.Name]; ok {
			e.installed = true
			e.version = s.Version
			e.source = s.Source
			e.skill = s
		}
		entries = append(entries, e)
	}

	// Append installed community skills that are not part of the bundled catalog.
	for _, s := range installed {
		if seen[s.Name] {
			continue
		}
		entries = append(entries, listEntry{
			name:        s.Name,
			displayName: s.DisplayName,
			description: "",
			version:     s.Version,
			source:      s.Source,
			installed:   true,
			skill:       s,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	return entries, nil
}

// renderSkillList renders the merged skill view honoring the --installed and
// --detailed flags. Counts are computed from the full catalog before filtering
// so the header is accurate regardless of which rows are shown.
func renderSkillList(entries []listEntry, installedOnly, detailed bool) error {
	available, installed := countEntries(entries)

	display := entries
	if installedOnly {
		display = filterInstalled(entries)
	}

	if len(display) == 0 {
		// Only reachable with --installed (the catalog is never empty).
		return writeSkillListOutput("No skills installed.\n\nBrowse available skills with:\n  atmos ai skill list\n")
	}

	var rendered string
	var err error
	if detailed {
		rendered = renderEntryDetails(display) + installHint()
	} else {
		rendered, err = renderEntrySummaries(display)
		if err != nil {
			return err
		}
		rendered += legend() + installHint()
	}

	return writeSkillListOutput(listHeader(installedOnly, available, installed) + rendered)
}

// listHeader returns the count header, worded for the active view.
func listHeader(installedOnly bool, available, installed int) string {
	if installedOnly {
		return fmt.Sprintf("Installed skills (%d):\n\n", installed)
	}
	return fmt.Sprintf("Atmos skills (%d available, %d installed):\n\n", available, installed)
}

// filterInstalled returns only the installed entries.
func filterInstalled(entries []listEntry) []listEntry {
	out := make([]listEntry, 0, len(entries))
	for _, e := range entries {
		if e.installed {
			out = append(out, e)
		}
	}
	return out
}

// renderEntrySummaries renders a one-line summary per skill using the shared
// list renderer pipeline.
func renderEntrySummaries(entries []listEntry) (string, error) {
	selector, err := column.NewSelector(skillListColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return "", fmt.Errorf("error creating skill list column selector: %w", err)
	}

	r := renderer.New(nil, selector, nil, listformat.FormatTable, "")
	return r.RenderToString(skillListRows(entries))
}

// countEntries returns the number of available (uninstalled catalog) and installed entries.
// The available count includes only catalog entries that are not yet installed,
// so the header legend matches the hollow-dot rows shown in the listing.
func countEntries(entries []listEntry) (available, installed int) {
	for _, e := range entries {
		if e.available && !e.installed {
			available++
		}
		if e.installed {
			installed++
		}
	}
	return available, installed
}

func skillListColumns() []column.Config {
	return []column.Config{
		{Name: "Status", Value: "{{ .status_marker }}"},
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "State", Value: "{{ .state }}"},
	}
}

func skillListRows(entries []listEntry) []map[string]any {
	rows := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		marker := markerAvailable
		state := "available"
		if e.installed {
			marker = markerInstalled
			state = "installed (v" + e.version + ")"
		}
		rows = append(rows, map[string]any{
			"status_marker": marker,
			"name":          e.name,
			"state":         state,
		})
	}
	return rows
}

func legend() string {
	return fmt.Sprintf("\n  %s installed   %s available\n", markerInstalled, markerAvailable)
}

func installHint() string {
	return "\nInstall a skill with:\n  atmos ai skill install <name>\n"
}

// renderEntryDetails renders detailed blocks, including
// install details when installed.
func renderEntryDetails(entries []listEntry) string {
	var b strings.Builder
	for i := range entries {
		writeEntryDetail(&b, &entries[i])
	}
	return b.String()
}

func writeEntryDetail(b *strings.Builder, e *listEntry) {
	status := "Available"
	if e.installed {
		status = "Installed"
	}

	fmt.Fprintf(b, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(b, "%s (%s)\n", e.displayName, status)
	fmt.Fprintf(b, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	fmt.Fprintf(b, "Name:         %s\n", e.name)
	fmt.Fprintf(b, "Version:      %s\n", e.version)
	fmt.Fprintf(b, "Source:       %s\n", e.source)
	if e.description != "" {
		fmt.Fprintf(b, "Description:  %s\n", e.description)
	}

	if e.installed && e.skill != nil {
		fmt.Fprintf(b, "Installed:    %s\n", formatTime(e.skill.InstalledAt))
		fmt.Fprintf(b, "Last Updated: %s\n", formatTime(e.skill.UpdatedAt))
		fmt.Fprintf(b, "Location:     %s\n", e.skill.Path)
	}

	fmt.Fprintf(b, "\n")
}

func writeSkillListOutput(content string) error {
	return output.New(listformat.FormatTable).Write(content)
}

// formatTime formats a time in a human-readable way.
func formatTime(t time.Time) string {
	const (
		hoursPerDay = 24
		daysPerWeek = 7
	)

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < hoursPerDay*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < daysPerWeek*hoursPerDay*time.Hour:
		days := int(diff.Hours() / hoursPerDay)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		// More than a week ago, show date.
		return strings.TrimSpace(t.Format("Jan 2, 2006"))
	}
}
