package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

// Color palette.
var (
	// Status colors.
	enabledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // Green
	disabledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")) // Gray

	// Header colors.
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")) // Purple
	countStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))           // Dim
	categoryStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")) // Cyan

	// Scene details.
	nameStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")) // White
	descStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))            // Light gray
	labelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))            // Dim
	valueStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("228"))            // Yellow
	tagStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))            // Pink
	outputStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))            // Light green
	noCategoryStyle = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("241"))
	separatorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238")) // Dark gray
	audioLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Orange
)

// listCmd returns the parent list command.
func listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List scenes, categories, and tags",
		Long:  `List demo scenes, categories, and tags defined in scenes.yaml.`,
	}

	cmd.AddCommand(listScenesSubCmd())
	cmd.AddCommand(listCategoriesCmd())
	cmd.AddCommand(listTagsCmd())

	return cmd
}

func listScenesSubCmd() *cobra.Command {
	var showAll bool
	var category string

	cmd := &cobra.Command{
		Use:   "scenes",
		Short: "List all demo scenes",
		Long: `List all demo scenes defined in scenes.yaml.

Shows scene names, descriptions, categories, and output formats.
Disabled scenes are shown in gray and marked with ⊘.`,
		Example: `
# List all scenes
director list scenes

# Show all details including requirements
director list scenes --all

# List scenes in a specific category
director list scenes --category terraform
`,
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			// Group scenes by category.
			categories := make(map[string][]*scene.Scene)
			var uncategorized []*scene.Scene
			enabledCount := 0
			disabledCount := 0

			for _, sc := range scenesList.Scenes {
				if sc.Enabled {
					enabledCount++
				} else {
					disabledCount++
				}

				cat := sc.GetCategory()
				if cat == "" {
					uncategorized = append(uncategorized, sc)
				} else {
					categories[cat] = append(categories[cat], sc)
				}
			}

			// Print header.
			fmt.Fprint(os.Stdout, headerStyle.Render("Atmos Demo Scenes"))
			fmt.Fprint(os.Stdout, " ")
			fmt.Fprintln(os.Stdout, countStyle.Render(fmt.Sprintf("(%d enabled, %d disabled)", enabledCount, disabledCount)))
			fmt.Fprintln(os.Stdout)

			// Get sorted category names.
			categoryOrder := []string{"flagship", "terraform", "demo-stacks", "auth", "dx"}
			seen := make(map[string]bool)
			for _, cat := range categoryOrder {
				seen[cat] = true
			}
			// Add any categories not in the predefined order.
			for cat := range categories {
				if !seen[cat] {
					categoryOrder = append(categoryOrder, cat)
				}
			}

			// If filtering by category, only show that category.
			if category != "" {
				scenes, ok := categories[category]
				if !ok {
					return fmt.Errorf("no scenes found in category: %s", category)
				}
				printCategory(category, scenes, showAll)
				return nil
			}

			// Print scenes by category.
			for _, cat := range categoryOrder {
				scenes, ok := categories[cat]
				if !ok {
					continue
				}
				printCategory(cat, scenes, showAll)
			}

			// Print uncategorized scenes.
			if len(uncategorized) > 0 {
				printCategory("uncategorized", uncategorized, showAll)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all details including requirements")
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter scenes by category")

	return cmd
}

func printCategory(category string, scenes []*scene.Scene, showAll bool) {
	// Category header.
	catDisplay := category
	if category == "uncategorized" {
		catDisplay = noCategoryStyle.Render("(no category)")
	} else {
		catDisplay = categoryStyle.Render(category)
	}
	fmt.Fprintf(os.Stdout, "%s %s\n", separatorStyle.Render("━━━"), catDisplay)

	for _, sc := range scenes {
		printScene(sc, showAll)
	}
	fmt.Fprintln(os.Stdout)
}

func printScene(sc *scene.Scene, showAll bool) {
	// Status indicator.
	var status string
	var nameStyled string
	var descStyled string

	if sc.Enabled {
		status = enabledStyle.Render("✓")
		nameStyled = nameStyle.Render(sc.Name)
		descStyled = descStyle.Render(sc.Description)
	} else {
		status = disabledStyle.Render("⊘")
		nameStyled = disabledStyle.Render(sc.Name)
		descStyled = disabledStyle.Render(sc.Description)
	}

	// First line: status, name, description.
	fmt.Fprintf(os.Stdout, "  %s %s", status, nameStyled)
	if sc.Description != "" {
		fmt.Fprintf(os.Stdout, "  %s", descStyled)
	}
	fmt.Fprintln(os.Stdout)

	// Second line: outputs, tags, audio.
	var details []string

	// Outputs.
	if len(sc.Outputs) > 0 {
		outputs := make([]string, len(sc.Outputs))
		for i, o := range sc.Outputs {
			outputs[i] = outputStyle.Render(o)
		}
		details = append(details, labelStyle.Render("outputs:")+strings.Join(outputs, separatorStyle.Render(",")))
	}

	// Tags.
	if len(sc.Tags) > 0 {
		tags := make([]string, len(sc.Tags))
		for i, t := range sc.Tags {
			tags[i] = tagStyle.Render(t)
		}
		details = append(details, labelStyle.Render("tags:")+strings.Join(tags, separatorStyle.Render(",")))
	}

	// Audio.
	if sc.Audio != nil {
		details = append(details, audioLabelStyle.Render("♪ audio"))
	}

	if len(details) > 0 {
		fmt.Fprintf(os.Stdout, "      %s\n", strings.Join(details, "  "))
	}

	// Additional details when --all is specified.
	if showAll {
		if sc.Tape != "" {
			fmt.Fprintf(os.Stdout, "      %s %s\n", labelStyle.Render("tape:"), valueStyle.Render(sc.Tape))
		}
		if len(sc.Requires) > 0 {
			fmt.Fprintf(os.Stdout, "      %s %s\n", labelStyle.Render("requires:"), valueStyle.Render(strings.Join(sc.Requires, ", ")))
		}
		if sc.Workdir != "" {
			fmt.Fprintf(os.Stdout, "      %s %s\n", labelStyle.Render("workdir:"), valueStyle.Render(sc.Workdir))
		}
	}
}

// listCategoriesCmd lists available scene categories.
func listCategoriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "categories",
		Short: "List available scene categories",
		Long: `List all categories defined in scenes.yaml.

Categories are used to group related scenes. Use the category name with
'director render --category <name>' to render all scenes in that category.`,
		Example: `
# List all categories
director list categories

# Then render all scenes in a category
director render --category terraform --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			// Collect categories with scene counts.
			categories := make(map[string]int)
			enabledByCategory := make(map[string]int)

			for _, sc := range scenesList.Scenes {
				cat := sc.GetCategory()
				if cat == "" {
					cat = "(uncategorized)"
				}
				categories[cat]++
				if sc.Enabled {
					enabledByCategory[cat]++
				}
			}

			// Print header.
			fmt.Fprint(os.Stdout, headerStyle.Render("Scene Categories"))
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout)

			// Sort categories.
			categoryOrder := []string{"flagship", "terraform", "demo-stacks", "list", "workflows", "auth", "dx", "config"}
			seen := make(map[string]bool)
			for _, cat := range categoryOrder {
				seen[cat] = true
			}
			for cat := range categories {
				if !seen[cat] {
					categoryOrder = append(categoryOrder, cat)
				}
			}

			// Print categories.
			for _, cat := range categoryOrder {
				count, ok := categories[cat]
				if !ok {
					continue
				}
				enabled := enabledByCategory[cat]

				catStyled := categoryStyle.Render(cat)
				countStr := countStyle.Render(fmt.Sprintf("(%d scenes, %d enabled)", count, enabled))

				fmt.Fprintf(os.Stdout, "  %s  %s\n", catStyled, countStr)
			}

			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, labelStyle.Render("Use 'director render --category <name>' to render all scenes in a category"))

			return nil
		},
	}
}

// listTagsCmd lists available scene tags.
func listTagsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tags",
		Short: "List available scene tags",
		Long: `List all tags used by scenes in scenes.yaml.

Tags are used to group related scenes for rendering. Use the tag name with
'director render --tag <name>' to render all scenes with that tag.`,
		Example: `
# List all tags
director list tags

# Then render all scenes with a tag
director render --tag version --force
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			scenesFile := filepath.Join(demosDir, "scenes.yaml")
			scenesList, err := scene.LoadScenes(scenesFile)
			if err != nil {
				return fmt.Errorf("failed to load scenes: %w", err)
			}

			// Collect tags with scene counts.
			tags := make(map[string]int)
			enabledByTag := make(map[string]int)

			for _, sc := range scenesList.Scenes {
				for _, t := range sc.Tags {
					tags[t]++
					if sc.Enabled {
						enabledByTag[t]++
					}
				}
			}

			// Print header.
			fmt.Fprint(os.Stdout, headerStyle.Render("Scene Tags"))
			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout)

			// Sort tags alphabetically.
			var tagNames []string
			for t := range tags {
				tagNames = append(tagNames, t)
			}
			// Sort with "featured" first, then alphabetically.
			sortedTags := make([]string, 0, len(tagNames))
			for _, t := range tagNames {
				if t == "featured" {
					sortedTags = append([]string{t}, sortedTags...)
				} else {
					sortedTags = append(sortedTags, t)
				}
			}

			// Print tags.
			for _, t := range sortedTags {
				count := tags[t]
				enabled := enabledByTag[t]

				tagStyled := tagStyle.Render(t)
				countStr := countStyle.Render(fmt.Sprintf("(%d scenes, %d enabled)", count, enabled))

				fmt.Fprintf(os.Stdout, "  %s  %s\n", tagStyled, countStr)
			}

			fmt.Fprintln(os.Stdout)
			fmt.Fprintln(os.Stdout, labelStyle.Render("Use 'director render --tag <name>' to render all scenes with a tag"))

			return nil
		},
	}
}

// catalogCmd is kept for backward compatibility (hidden).
func catalogCmd() *cobra.Command {
	cmd := listScenesSubCmd()
	cmd.Use = "catalog"
	cmd.Aliases = nil
	cmd.Hidden = true
	return cmd
}
