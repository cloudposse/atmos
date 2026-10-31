package scaffold

import (
	artifactscmd "github.com/cloudposse/atmos/cmd/artifacts"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/markdown"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	artifactscaffold "github.com/cloudposse/atmos/pkg/artifact/scaffold"
)

// scaffoldPublishSpec describes `atmos scaffold publish`. The command shares the flag set,
// environment variables, service wiring, and result rendering of `atmos artifacts publish` and only
// supplies the scaffold kind; the scaffold producer validates the scaffold configuration before it
// packages anything, without generating output or running scaffold hooks.
var scaffoldPublishSpec = artifactscmd.KindPublishSpec{
	Use:   "publish <source>",
	Short: "Validate and publish a scaffold as a versioned artifact",
	Long: `Validate a scaffold directory and publish it to one configured artifact repository.

The scaffold configuration (scaffold.yaml at the source root) is validated first, without generating output and without running scaffold hooks. The artifact name and version default to the scaffold manifest's metadata.name and metadata.version; --name and --version override them.

The root README is ordinary scaffold content. The files .atmos/artifact/README.md and .atmos/artifact/CHANGELOG.md are published as separate package documents and never appear in generated output.`,
	Example: markdown.ScaffoldPublishUsageMarkdown,
	Kind:    produce.KindScaffold,
	Prefix:  "scaffold.publish",
	Usage:   "atmos scaffold publish <source> --repository=<alias> [--name=<name>] [--version=<version>]",
}

var scaffoldPublishCmd = artifactscmd.NewKindPublishCommand(&scaffoldPublishSpec)

func init() {
	// The scaffold producer validates through the scaffold manifest loader, which sits above the
	// artifact packages in the dependency graph, so the binary registers it here.
	artifactscaffold.Register()

	scaffoldCmd.AddCommand(scaffoldPublishCmd)
	internal.RegisterCommandMetadata(
		scaffoldPublishCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/scaffold/publish"),
	)
}
