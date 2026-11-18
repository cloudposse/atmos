package artifacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifactwire"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Catalog flag names and environment variables. A catalog is the optional, maintained discovery
// snapshot of one configured repository.
const (
	flagInput          = "input"
	flagExpectedDigest = "expected-digest"

	envInput          = "ATMOS_ARTIFACTS_INPUT"
	envExpectedDigest = "ATMOS_ARTIFACTS_EXPECTED_DIGEST"

	catalogBuildPrefix   = "artifacts.catalog.build"
	catalogPublishPrefix = "artifacts.catalog.publish"
	catalogAddPrefix     = "artifacts.catalog.add"
	catalogRemovePrefix  = "artifacts.catalog.remove"
	catalogSyncPrefix    = "artifacts.catalog.sync"
)

// CatalogFactory builds the catalog maintenance service for the loaded configuration. Tests replace
// it to inject a fake; production composes the real repositories lazily, so building the service
// performs no network access, credential resolution, or directory creation.
type CatalogFactory func(config *schema.AtmosConfiguration) (catalog.Maintainer, error)

var newCatalogs CatalogFactory = artifactwire.NewCatalogs

// UseCatalogFactory replaces the catalog service factory and returns a function that restores the
// previous one. It is the seam command tests use; production code never calls it.
func UseCatalogFactory(factory CatalogFactory) (restore func()) {
	previous := newCatalogs
	newCatalogs = factory
	return func() { newCatalogs = previous }
}

func init() {
	hintRules = append(catalogHintRules(), hintRules...)
}

// newCatalogCommand builds the `atmos artifacts catalog` command group.
func newCatalogCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "catalog",
		Short: "Build and maintain the optional discovery catalog of a repository",
		Long: `Build, publish, and maintain the optional catalog of a configured repository. A catalog is a
maintained discovery snapshot published as an artifact record of kind catalog. It lets a registry that
cannot enumerate repositories be listed and searched. Catalogs are discoverable data, never a trust grant:
publishing, signing, or deleting an artifact never edits a catalog, and removing an entry only unlists it.
Catalog writes need one externally serialized publisher.`,
		Example: markdown.ArtifactsCatalogUsageMarkdown,
	}
	command.AddCommand(newCatalogBuildCommand(), newCatalogPublishCommand(), newCatalogAddCommand(), newCatalogRemoveCommand(), newCatalogSyncCommand())
	return command
}

// catalogRun is the operation of one catalog subcommand.
type catalogRun func(ctx context.Context, maintainer catalog.Maintainer, in *input) (*catalogResult, error)

// catalogCommand describes one catalog subcommand; the pipeline around it is shared.
type catalogCommand struct {
	action string
	parser *flags.StandardParser
	prefix string
	run    catalogRun
}

func (c *catalogCommand) build(use, short, long, example string, args cobra.PositionalArgs) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Long: long, Example: example, Args: args, RunE: c.execute}
	c.parser.RegisterFlags(cmd)
	registerCompletions(cmd, resultFormats)
	return cmd
}

// execute binds flags, builds the service, runs the operation, and renders the result. A structured
// format emits its envelope even when the operation fails, and the returned error keeps the exit
// code nonzero.
func (c *catalogCommand) execute(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "artifacts.catalogCommand.execute")()

	values := viper.New()
	if err := c.parser.BindFlagsToViper(cmd, values); err != nil {
		return err
	}
	in := &input{v: values, prefix: c.prefix, args: args}
	format := in.str(flagFormat)
	if err := validateFormat(format, resultFormats); err != nil {
		return err
	}
	res, err := c.invoke(commandContext(cmd), in)
	if res == nil {
		res = &catalogResult{}
	}
	res.Action, res.Repository = c.action, in.str(flagRepository)
	if err != nil {
		res.Error = err.Error()
	}
	if isStructured(format) {
		if emitErr := writeStructured(format, res); emitErr != nil && err == nil {
			return emitErr
		}
	} else if err == nil {
		if textErr := writeText(res.pairs()); textErr != nil {
			return textErr
		}
	}
	return decorate(err, configuredAliases(atmosConfigPtr))
}

func (c *catalogCommand) invoke(ctx context.Context, in *input) (*catalogResult, error) {
	if atmosConfigPtr == nil {
		return nil, errUtils.Build(errUtils.ErrArtifactConfigInvalid).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project directory that contains an `atmos.yaml`, or pass `--config`.").
			Err()
	}
	if _, err := in.repository(); err != nil {
		return nil, err
	}
	maintainer, err := newCatalogs(atmosConfigPtr)
	if err != nil {
		return nil, err
	}
	return c.run(ctx, maintainer, in)
}

// catalogResult is the structured result of every catalog subcommand.
type catalogResult struct {
	Action         string   `json:"action"`
	Repository     string   `json:"repository,omitempty"`
	Catalog        string   `json:"catalog,omitempty"`
	Output         string   `json:"output,omitempty"`
	Digest         string   `json:"digest,omitempty"`
	PreviousDigest string   `json:"previous_digest,omitempty"`
	BaseDigest     string   `json:"base_digest,omitempty"`
	Revision       int      `json:"revision"`
	Listed         int      `json:"listed"`
	Added          []string `json:"added,omitempty"`
	Updated        []string `json:"updated,omitempty"`
	Removed        []string `json:"removed,omitempty"`
	LockScope      string   `json:"lock_scope,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// pairs renders the text view; empty values are omitted, never filled in.
func (r *catalogResult) pairs() []keyValue {
	return []keyValue{
		{"repository", r.Repository},
		{"catalog", r.Catalog},
		{"output", r.Output},
		{"digest", r.Digest},
		{"previous_digest", r.PreviousDigest},
		{"base_digest", r.BaseDigest},
		{"revision", fmt.Sprint(r.Revision)},
		{"listed", fmt.Sprint(r.Listed)},
		{"added", strings.Join(r.Added, ", ")},
		{"updated", strings.Join(r.Updated, ", ")},
		{"removed", strings.Join(r.Removed, ", ")},
		{"lock_scope", r.LockScope},
	}
}

// fromMutation maps a service result onto the command result.
func fromMutation(res *catalog.Result) *catalogResult {
	if res == nil {
		return nil
	}
	return &catalogResult{
		Catalog: res.Catalog, Digest: res.Digest, PreviousDigest: res.PreviousDigest, Revision: res.Revision, Listed: res.Listed,
		Added: res.Added, Updated: res.Updated, Removed: res.Removed, LockScope: string(res.LockScope),
	}
}

// catalogHintRules attach actionable hints to catalog failures.
func catalogHintRules() []hintRule {
	return []hintRule{
		{errUtils.ErrArtifactCatalogNotConfigured, "", func([]string) []string {
			return []string{"Set `catalog: oci://<registry>/<path>:<tag>` on the repository in `artifacts.repositories`. The catalog must live under the repository's registry and prefix."}
		}},
		{errUtils.ErrArtifactCatalogConflict, "", func([]string) []string {
			return []string{"The catalog changed since it was read. Rebuild the snapshot from the latest catalog, or run the command again; run only one catalog writer at a time."}
		}},
		{errUtils.ErrArtifactCatalogInvalid, "", func([]string) []string {
			return []string{"A catalog lists only artifacts of the repository it is configured on. Check the inventory refs and the catalog's entries."}
		}},
		{errUtils.ErrArtifactDiscoveryOffline, "", func([]string) []string {
			return []string{"Offline discovery serves only existing snapshots. Run the command online once, or turn off `plugins.policy.offline`."}
		}},
	}
}
