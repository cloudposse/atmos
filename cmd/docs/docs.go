package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/internal/exec"
	termUtils "github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/browser"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	l "github.com/cloudposse/atmos/pkg/list"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	atmosDocsURL = "https://atmos.tools"

	// Prefix of a long flag in shell completion input.
	longFlagPrefix = "--"
)

var openDocsURL = func(url string) error {
	return browser.New().Open(url)
}

// docsCmd opens the Atmos docs and can display component documentation
var docsCmd = &cobra.Command{
	Use:                "docs",
	Short:              "Open Atmos documentation or display component-specific docs",
	Long:               `This command opens the Atmos docs or displays the documentation for a specified Atmos component.`,
	Example:            "atmos docs vpc",
	Args:               cobra.MaximumNArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  componentsArgCompletion,
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

			styleOption := glamour.WithAutoStyle()
			if lipgloss.ColorProfile() == termenv.Ascii {
				style, err := markdown.GetPlainTextStyle()
				if err != nil {
					return err
				}
				styleOption = glamour.WithStylesFromJSONBytes(style)
			}
			r, err := glamour.NewTermRenderer(
				glamour.WithColorProfile(lipgloss.ColorProfile()),
				styleOption,
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
		if err := openDocsURL(atmosDocsURL); err != nil {
			return fmt.Errorf("open Atmos docs: %w", err)
		}

		ui.Infof("Opening default browser to '%v'.", atmosDocsURL)
		return nil
	},
}

// componentsArgCompletion provides shell completion for the component positional argument.
// It mirrors the root command's component completion: directory completion for path-like input,
// otherwise component names from the stack configurations.
func componentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Enable directory completion for paths (platform-specific separator and forward slash).
		if toComplete == "." || strings.Contains(toComplete, string(filepath.Separator)) || strings.Contains(toComplete, "/") {
			log.Trace("Enabling directory completion for path input", "toComplete", toComplete)
			return nil, cobra.ShellCompDirectiveFilterDirs
		}

		// Otherwise, suggest component names from stack configurations.
		output, err := listComponents(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	flagName := args[len(args)-1]
	if strings.HasPrefix(flagName, longFlagPrefix) {
		flagName = strings.ReplaceAll(flagName, longFlagPrefix, "")
	}
	if strings.HasPrefix(toComplete, longFlagPrefix) {
		flagName = strings.ReplaceAll(toComplete, longFlagPrefix, "")
	}
	flagName = strings.ReplaceAll(flagName, "=", "")
	if option, ok := cmd.GetFlagCompletionFunc(flagName); ok {
		return option(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// listComponents returns the component names found in the stack configurations,
// filtered by the --stack flag when the command defines one.
func listComponents(cmd *cobra.Command) ([]string, error) {
	stackFlag, err := cmd.Flags().GetString("stack")
	if err != nil {
		stackFlag = ""
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil, fmt.Errorf("error initializing CLI config: %w", err)
	}

	stacksMap, err := exec.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error describing stacks: %w", err)
	}

	return l.FilterAndListComponents(stackFlag, stacksMap)
}

func init() {
	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(
		&DocsCommandProvider{},
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/docs/usage"),
	)
}

// DocsCommandProvider implements the CommandProvider interface.
type DocsCommandProvider struct{}

// GetCommand returns the docs command.
func (d *DocsCommandProvider) GetCommand() *cobra.Command {
	return docsCmd
}

// GetName returns the command name.
func (d *DocsCommandProvider) GetName() string {
	return "docs"
}

// GetGroup returns the command group for help organization.
func (d *DocsCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
// The docs command has no flags of its own; docs generate registers its own via its parser.
func (d *DocsCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// The docs command validates its optional component argument via cobra.MaximumNArgs(1).
func (d *DocsCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// The docs command has no compatibility flags.
func (d *DocsCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// The docs command has no aliases.
func (d *DocsCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (d *DocsCommandProvider) IsExperimental() bool {
	return false
}
