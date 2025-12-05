package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const atmosDocsURL = "https://atmos.tools"

// docsCmd opens the Atmos docs and can display component documentation.
var docsCmd = &cobra.Command{
	Use:                "docs",
	Short:              "Open Atmos documentation or display component-specific docs",
	Long:               `This command opens the Atmos docs or displays the documentation for a specified Atmos component.`,
	Example:            "atmos docs vpc",
	Args:               cobra.MaximumNArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			info := schema.ConfigAndStacksInfo{
				Component:             args[0],
				ComponentFolderPrefix: "terraform",
			}

			atmosConfig, err := cfg.InitCliConfig(info, false)
			if err != nil {
				return err
			}

			// Detect terminal width if not specified in `atmos.yaml`
			// The default screen width is 120 characters, but uses maxWidth if set and greater than zero
			maxWidth := atmosConfig.Settings.Terminal.MaxWidth
			if maxWidth == 0 && atmosConfig.Settings.Docs.MaxWidth > 0 {
				maxWidth = atmosConfig.Settings.Docs.MaxWidth
				log.Warn("'settings.docs.max-width' is deprecated and will be removed in a future version. Please use 'settings.terminal.max_width' instead")
			}
			defaultWidth := 120
			screenWidth := defaultWidth

			// Detect terminal width and use it by default if available
			if termUtils.IsTTYSupportForStdout() {
				termWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil && termWidth > 0 {
					// Adjusted for subtle padding effect at the terminal boundaries
					screenWidth = termWidth - 2
				}
			}

			if maxWidth > 0 {
				screenWidth = min(maxWidth, screenWidth)
			}

			// Construct the full path to the Terraform component by combining the Atmos base path, Terraform base path, and component name
			componentPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, info.Component)
			componentPathExists, err := u.IsDirectory(componentPath)
			if err != nil {
				return err
			}

			if !componentPathExists {
				er := fmt.Errorf("component `%s` not found in path: `%s`", info.Component, componentPath)
				return er
			}

			readmePath := filepath.Join(componentPath, "README.md")
			if _, err := os.Stat(readmePath); err != nil {
				if os.IsNotExist(err) {
					er := fmt.Errorf("No README found for component: %s", info.Component)
					return er
				} else {
					return err
				}
			}

			readmeContent, err := os.ReadFile(readmePath)
			if err != nil {
				return err
			}

			r, err := glamour.NewTermRenderer(
				glamour.WithColorProfile(lipgloss.ColorProfile()),
				glamour.WithAutoStyle(),
				glamour.WithPreservedNewLines(),
				glamour.WithWordWrap(screenWidth),
			)
			if err != nil {
				return err
			}

			componentDocs, err := r.Render(string(readmeContent))
			if err != nil {
				return err
			}

			pager := atmosConfig.Settings.Terminal.IsPagerEnabled()
			if !pager && atmosConfig.Settings.Docs.Pagination {
				pager = atmosConfig.Settings.Docs.Pagination
				log.Warn("'settings.docs.pagination' is deprecated and will be removed in a future version. Please use 'settings.terminal.pager' instead")
			}

			if err := u.DisplayDocs(componentDocs, pager); err != nil {
				return err
			}

			return nil
		}

		// Opens atmos.tools docs if no component argument is provided
		if err := u.OpenUrl(atmosDocsURL); err != nil {
			return fmt.Errorf("open Atmos docs: %w", err)
		}

		// UI messages should go to stderr; stdout is for data/results.
		fmt.Fprintf(os.Stderr, "Opening default browser to '%v'.\n", atmosDocsURL)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(docsCmd)
}
