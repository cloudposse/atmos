package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const atmosDocsURL = "https://atmos.tools"

// docsCmd opens the Atmos docs and can display component documentation
var docsCmd = &cobra.Command{
	Use:                "docs",
	Short:              "Open Atmos documentation or display component-specific docs",
	Long:               `This command opens the Atmos docs or displays the documentation for a specified Atmos component.`,
	Example:            "atmos docs vpc",
	Args:               cobra.MaximumNArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			info := schema.ConfigAndStacksInfo{
				Component:             args[0],
				ComponentFolderPrefix: "terraform",
			}

			atmosConfig, err := cfg.InitCliConfig(info, true)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
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
			if term.IsTerminal(int(os.Stdout.Fd())) {
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
				log.Debug(err)
				u.PrintErrorMarkdownAndExit("", fmt.Errorf("Component not found"), "")
			}
			if !componentPathExists {
				u.PrintErrorMarkdownAndExit("", fmt.Errorf("Component not found"), "")
			}

			readmePath := filepath.Join(componentPath, "README.md")
			if _, err := os.Stat(readmePath); err != nil {
				u.PrintErrorMarkdownAndExit("", fmt.Errorf("Documentation is missing for the component `%s`. Consider adding a README.md to provide more context and details.", info.Component), "")
			}

			readmeContent, err := os.ReadFile(readmePath)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}

			r, err := glamour.NewTermRenderer(
				glamour.WithColorProfile(lipgloss.ColorProfile()),
				glamour.WithAutoStyle(),
				glamour.WithPreservedNewLines(),
				glamour.WithWordWrap(screenWidth),
			)
			if err != nil {
				u.PrintErrorMarkdownAndExit("", fmt.Errorf("failed to initialize markdown renderer: %w", err), "")
			}

			componentDocs, err := r.Render(string(readmeContent))
			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}

			pager := atmosConfig.Settings.Terminal.Pager
			if !pager && atmosConfig.Settings.Docs.Pagination {
				pager = atmosConfig.Settings.Docs.Pagination
				log.Warn("'settings.docs.pagination' is deprecated and will be removed in a future version. Please use 'settings.terminal.pager' instead")
			}

			if err := u.DisplayDocs(componentDocs, pager); err != nil {
				u.PrintErrorMarkdownAndExit("", fmt.Errorf("failed to display documentation: %w", err), "")
			}

			return
		}

		// Opens atmos.tools docs if no component argument is provided
		var err error

		if os.Getenv("GO_TEST") == "1" {
			log.Debug("Skipping browser launch in test environment")
		} else {
			switch runtime.GOOS {
			case "linux":
				err = exec.Command("xdg-open", atmosDocsURL).Start()
			case "windows":
				err = exec.Command("rundll32", "url.dll,FileProtocolHandler", atmosDocsURL).Start()
			case "darwin":
				err = exec.Command("open", atmosDocsURL).Start()
			default:
				err = fmt.Errorf("unsupported platform: %s", runtime.GOOS)
			}

			if err != nil {
				u.PrintErrorMarkdownAndExit("", err, "")
			}
		}

		fmt.Printf("Opening default browser to '%v'.\n", atmosDocsURL)
	},
}

func init() {
	RootCmd.AddCommand(docsCmd)
}
