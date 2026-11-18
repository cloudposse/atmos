// Package artifacts implements the `atmos artifacts` command space: publish, pull, copy, inspect,
// list, search, delete, sign, verify, and show.
//
// The commands are thin wiring. They parse flags with the standard flag parser, build the artifact
// service through the composition root in pkg/artifact/compose, call one service method, and render
// the typed result. Packaging, repository access, signatures, locks, and conflict policy all live in
// the artifact service and its repositories, so the commands, workflow steps, and hooks share one
// implementation.
package artifacts

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// docsBase is the documentation route of the command space.
const docsBase = "https://atmos.tools/cli/commands/artifacts/"

// atmosConfigPtr holds the Atmos configuration injected by root.go before any subcommand runs.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig is called from root.go after the configuration is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// artifactsCmd is the `atmos artifacts` parent command.
var artifactsCmd = NewCommand()

// NewCommand builds the `atmos artifacts` command tree with fresh flag parsers. The registered
// command is built once at package initialization; tests build their own to start from default flags.
func NewCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "artifacts",
		Short: "Publish, pull, copy, inspect, sign, and verify versioned artifacts across repositories",
		Long: `Publish files and directories as named, versioned artifacts in local, S3, and OCI repositories, then
pull, copy, inspect, list, search, sign, and verify them with one command space. Repositories are
configured under artifacts.repositories in atmos.yaml and addressed by alias; no command picks a
default repository or an implicit latest version.`,
		Example: markdown.ArtifactsUsageMarkdown,
	}
	command.AddCommand(
		newPublishCommand(),
		newPullCommand(),
		newCopyCommand(),
		newInspectCommand(),
		newListCommand(),
		newSearchCommand(),
		newDeleteCommand(),
		newSignCommand(),
		newVerifyCommand(),
		newShowCommand(),
		newCatalogCommand(),
	)
	registerDocumentation(command, "")
	return command
}

// documentationPages maps a command path to its page below the artifacts docs route.
var documentationPages = map[string]string{
	"":               "usage",
	"publish":        "publish",
	"pull":           "pull",
	"copy":           "copy",
	"inspect":        "inspect",
	"list":           "list",
	"search":         "search",
	"delete":         "delete",
	"sign":           "sign",
	"verify":         "verify",
	"show":           "show",
	"show readme":    "show",
	"show changelog": "show",

	"catalog":         "catalog",
	"catalog build":   "catalog-build",
	"catalog publish": "catalog-publish",
	"catalog add":     "catalog-add",
	"catalog remove":  "catalog-remove",
	"catalog sync":    "catalog-sync",
}

// registerDocumentation links every command to its documentation page.
func registerDocumentation(command *cobra.Command, path string) {
	if page, exists := documentationPages[path]; exists {
		internal.RegisterCommandMetadata(command, internal.WithDocumentationURL(docsBase+page))
	}
	for _, child := range command.Commands() {
		next := child.Name()
		if path != "" {
			next = path + " " + child.Name()
		}
		registerDocumentation(child, next)
	}
}

func init() {
	defer perf.Track(nil, "artifacts.init")()

	internal.Register(&CommandProvider{})
}

// CommandProvider registers the artifacts commands with the standard command registry.
type CommandProvider struct{}

// GetCommand returns the artifacts parent command with every subcommand attached.
func (*CommandProvider) GetCommand() *cobra.Command { return artifactsCmd }

// GetName returns the command name.
func (*CommandProvider) GetName() string { return "artifacts" }

// GetGroup returns the command group for help organization.
func (*CommandProvider) GetGroup() string { return "Other Commands" }

// GetFlagsBuilder returns nil: the subcommands own their flags.
func (*CommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns nil: the parent command takes no positional arguments.
func (*CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

// GetCompatibilityFlags returns nil: the commands use native Cobra flags only.
func (*CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

// GetAliases returns nil: the commands have no aliases.
func (*CommandProvider) GetAliases() []internal.CommandAlias { return nil }

// IsExperimental reports that the artifacts commands are experimental in V1.
func (*CommandProvider) IsExperimental() bool { return true }
