package artifacts

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newCatalogBuildCommand() *cobra.Command {
	parser := newParser(catalogBuildPrefix, repositoryFlag(), []flags.Option{
		flags.WithStringFlag(flagInput, "", "", "AtmosArtifactInventory manifest listing the releases to resolve"),
		flags.WithStringFlag(flagOutput, "", "", "File to write the catalog snapshot to (.json, .yaml, or .yml)"),
		flags.WithStringFlag(flagName, "", "", "Catalog manifest name (default: the configured catalog name)"),
		flags.WithEnvVars(flagInput, envInput),
		flags.WithEnvVars(flagOutput, envOutput),
		flags.WithEnvVars(flagName, envName),
	}, formatFlag(formatText, resultFormats))
	c := &catalogCommand{action: "catalog-build", parser: parser, prefix: catalogBuildPrefix, run: runCatalogBuild}
	return c.build("build", "Build a catalog snapshot from an inventory without publishing it",
		`Resolve the releases of an AtmosArtifactInventory in one configured repository and write an
AtmosArtifactCatalog snapshot. Build reads artifact metadata only: no payload is downloaded and nothing is
published. The snapshot records the revision of the catalog it was based on, so publishing it later fails
if the catalog changed in between.`,
		markdown.ArtifactsCatalogBuildUsageMarkdown, flagsOnly("atmos artifacts catalog build --repository=<alias> --input=<file> --output=<file>"))
}

func runCatalogBuild(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	input, err := in.required(flagInput, envInput)
	if err != nil {
		return nil, err
	}
	output, err := in.required(flagOutput, envOutput)
	if err != nil {
		return nil, err
	}
	refs, err := catalog.ReadInventoryFile(input)
	if err != nil {
		return nil, err
	}
	built, err := maintainer.Build(ctx, &catalog.BuildRequest{Repository: repository, Refs: refs, Name: in.str(flagName)})
	if err != nil {
		return nil, err
	}
	if err := catalog.WriteFile(output, built.File); err != nil {
		return nil, err
	}
	return &catalogResult{Output: output, BaseDigest: built.BaseDigest, Revision: built.File.Metadata.Revision, Listed: countListed(built.File)}, nil
}

// countListed counts the name:version pairs of a snapshot.
func countListed(file *catalog.File) int {
	total := 0
	for index := range file.Spec.Entries {
		total += len(file.Spec.Entries[index].Versions)
	}
	return total
}

func newCatalogPublishCommand() *cobra.Command {
	parser := newParser(catalogPublishPrefix, repositoryFlag(), []flags.Option{
		flags.WithStringFlag(flagExpectedDigest, "", "", "Require the published catalog to have this manifest digest"),
		flags.WithEnvVars(flagExpectedDigest, envExpectedDigest),
	}, formatFlag(formatText, resultFormats))
	c := &catalogCommand{action: "catalog-publish", parser: parser, prefix: catalogPublishPrefix, run: runCatalogPublish}
	return c.build("publish <file>", "Publish a catalog snapshot to the repository's configured catalog",
		`Publish an AtmosArtifactCatalog snapshot to the catalog configured on the repository. The command fails
before any write when the repository has no catalog, when the snapshot names a location outside the
repository, or when the catalog changed since the snapshot was built. Run only one catalog writer at a time:
the checks detect stale input but are not an atomic registry lock.`,
		markdown.ArtifactsCatalogPublishUsageMarkdown, exactArgs("atmos artifacts catalog publish <file> --repository=<alias>"))
}

func runCatalogPublish(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	file, err := catalog.ReadFile(in.args[0])
	if err != nil {
		return nil, err
	}
	res, err := maintainer.Publish(ctx, &catalog.PublishRequest{Repository: repository, File: file, ExpectedDigest: in.str(flagExpectedDigest)})
	return fromMutation(res), err
}

func newCatalogAddCommand() *cobra.Command {
	parser := newParser(catalogAddPrefix, repositoryFlag(), nil, formatFlag(formatText, resultFormats))
	c := &catalogCommand{action: "catalog-add", parser: parser, prefix: catalogAddPrefix, run: runCatalogAdd}
	return c.build("add <ref>...", "List releases in the repository's published catalog",
		`Resolve the releases (name:version or name@sha256:<digest>) in the repository and list them in the latest
published catalog, creating the catalog when none exists. Only artifact metadata is read.`,
		markdown.ArtifactsCatalogAddUsageMarkdown, minArgs("atmos artifacts catalog add <ref>... --repository=<alias>"))
}

func runCatalogAdd(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	refs, err := parseRefs(in.args)
	if err != nil {
		return nil, err
	}
	res, err := maintainer.Add(ctx, &catalog.AddRequest{Repository: repository, Refs: refs})
	return fromMutation(res), err
}

func newCatalogRemoveCommand() *cobra.Command {
	parser := newParser(catalogRemovePrefix, repositoryFlag(), nil, formatFlag(formatText, resultFormats))
	c := &catalogCommand{action: "catalog-remove", parser: parser, prefix: catalogRemovePrefix, run: runCatalogRemove}
	return c.build("remove <ref>", "Unlist a release from the repository's published catalog",
		`Unlist a name:version from the latest published catalog. The artifact, its signatures, and every consumer
lock file are untouched: removal only stops the release from being discovered.`,
		markdown.ArtifactsCatalogRemoveUsageMarkdown, exactArgs("atmos artifacts catalog remove <name:version> --repository=<alias>"))
}

func runCatalogRemove(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	ref, err := artifact.ParseRef(in.args[0])
	if err != nil {
		return nil, err
	}
	res, err := maintainer.Remove(ctx, &catalog.RemoveRequest{Repository: repository, Ref: ref})
	return fromMutation(res), err
}

func newCatalogSyncCommand() *cobra.Command {
	parser := newParser(catalogSyncPrefix, repositoryFlag(), []flags.Option{
		flags.WithStringFlag(flagInput, "", "", "AtmosArtifactInventory manifest listing the releases to merge"),
		flags.WithEnvVars(flagInput, envInput),
	}, formatFlag(formatText, resultFormats))
	c := &catalogCommand{action: "catalog-sync", parser: parser, prefix: catalogSyncPrefix, run: runCatalogSync}
	return c.build("sync", "Merge an inventory's releases into the repository's published catalog",
		`Resolve the releases of an AtmosArtifactInventory and merge them into the latest published catalog. Entries
the inventory does not mention are kept. Only artifact metadata is read.`,
		markdown.ArtifactsCatalogSyncUsageMarkdown, flagsOnly("atmos artifacts catalog sync --repository=<alias> --input=<file>"))
}

func runCatalogSync(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error) {
	repository, err := in.repository()
	if err != nil {
		return nil, err
	}
	input, err := in.required(flagInput, envInput)
	if err != nil {
		return nil, err
	}
	refs, err := catalog.ReadInventoryFile(input)
	if err != nil {
		return nil, err
	}
	res, err := maintainer.Sync(ctx, &catalog.SyncRequest{Repository: repository, Refs: refs})
	return fromMutation(res), err
}

// parseRefs parses explicit references; there is no implicit latest.
func parseRefs(args []string) ([]artifact.Ref, error) {
	refs := make([]artifact.Ref, 0, len(args))
	for _, text := range args {
		ref, err := artifact.ParseRef(text)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// flagsOnly rejects positional arguments.
func flagsOnly(usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return errUtils.Build(fmt.Errorf("%w: expected no arguments, got %d", errUtils.ErrInvalidPositionalArgs, len(args))).
			WithHintf("Usage: `%s`.", usage).
			WithExitCode(2).
			Err()
	}
}

// minArgs requires at least one positional argument.
func minArgs(usage string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) >= 1 {
			return nil
		}
		return errUtils.Build(fmt.Errorf("%w: expected at least 1 argument", errUtils.ErrInvalidPositionalArgs)).
			WithHintf("Usage: `%s`.", usage).
			WithExitCode(2).
			Err()
	}
}
