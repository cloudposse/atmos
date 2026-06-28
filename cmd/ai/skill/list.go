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

		renderSkillList(entries, installedOnly, detailed)
		return nil
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

// renderSkillList prints the merged skill view honoring the --installed and
// --detailed flags. Counts are computed from the full catalog before filtering
// so the header is accurate regardless of which rows are shown.
func renderSkillList(entries []listEntry, installedOnly, detailed bool) {
	available, installed := countEntries(entries)

	display := entries
	if installedOnly {
		display = filterInstalled(entries)
	}

	if len(display) == 0 {
		// Only reachable with --installed (the catalog is never empty).
		fmt.Println("No skills installed.")
		fmt.Println("\nBrowse available skills with:")
		fmt.Println("  atmos ai skill list")
		return
	}

	printListHeader(installedOnly, available, installed)

	if detailed {
		for i := range display {
			printEntryDetailed(&display[i])
		}
		printInstallHint()
		return
	}

	printEntrySummaries(display)
}

// printListHeader prints the count header, worded for the active view.
func printListHeader(installedOnly bool, available, installed int) {
	if installedOnly {
		fmt.Printf("Installed skills (%d):\n\n", installed)
		return
	}
	fmt.Printf("Atmos skills (%d available, %d installed):\n\n", available, installed)
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

// printEntrySummaries prints a one-line summary per skill with a status marker,
// padded to align names into columns.
func printEntrySummaries(entries []listEntry) {
	nameWidth := 0
	for _, e := range entries {
		if len(e.name) > nameWidth {
			nameWidth = len(e.name)
		}
	}

	for _, e := range entries {
		marker := markerAvailable
		status := "available"
		if e.installed {
			marker = markerInstalled
			status = "installed (v" + e.version + ")"
		}
		fmt.Printf("  %s %-*s  %s\n", marker, nameWidth, e.name, status)
	}

	fmt.Printf("\n  %s installed   %s available\n", markerInstalled, markerAvailable)
	printInstallHint()
}

// countEntries returns the number of available (catalog) and installed entries.
func countEntries(entries []listEntry) (available, installed int) {
	for _, e := range entries {
		if e.available {
			available++
		}
		if e.installed {
			installed++
		}
	}
	return available, installed
}

// printInstallHint prints the install command hint shown after the listing.
func printInstallHint() {
	fmt.Println("\nInstall a skill with:")
	fmt.Println("  atmos ai skill install <name>")
}

// printEntryDetailed prints a detailed block for a single skill, including
// install details when installed.
func printEntryDetailed(e *listEntry) {
	status := "Available"
	if e.installed {
		status = "Installed"
	}

	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("%s (%s)\n", e.displayName, status)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	fmt.Printf("Name:         %s\n", e.name)
	fmt.Printf("Version:      %s\n", e.version)
	fmt.Printf("Source:       %s\n", e.source)
	if e.description != "" {
		fmt.Printf("Description:  %s\n", e.description)
	}

	if e.installed && e.skill != nil {
		fmt.Printf("Installed:    %s\n", formatTime(e.skill.InstalledAt))
		fmt.Printf("Last Updated: %s\n", formatTime(e.skill.UpdatedAt))
		fmt.Printf("Location:     %s\n", e.skill.Path)
	}

	fmt.Printf("\n")
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
