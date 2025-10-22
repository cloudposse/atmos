package multiline_example

import "github.com/spf13/cobra"

// This should trigger the linter - multi-line Example field.
var badCmd = &cobra.Command{
	Use:   "bad",
	Short: "Bad command with multi-line example",
	// want +1 "multi-line markdown examples should use embedded markdown files from cmd/markdown/ instead of inline strings; see CLAUDE.md for the pattern"
	Example: `  # List available devcontainers
  atmos devcontainer list

  # Start a devcontainer
  atmos devcontainer start default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

// This should also trigger - using \n escape sequences.
var badCmd2 = &cobra.Command{
	Use:     "bad2",
	Short:   "Another bad command",
	Example: "atmos example\n\natmos example --flag", // want "multi-line markdown examples should use embedded markdown files from cmd/markdown/ instead of inline strings; see CLAUDE.md for the pattern"
}

// This should NOT trigger - single line example.
var goodCmd = &cobra.Command{
	Use:     "good",
	Short:   "Good command with single-line example",
	Example: "atmos good --flag",
}
