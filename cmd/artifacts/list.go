package artifacts

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// collection describes one collection-producing subcommand (list or search).
type collection struct {
	action artifact.Action
	parser *flags.StandardParser
	prefix string
}

// collectionSpec describes how list and search differ; everything else is shared.
type collectionSpec struct {
	use     string
	short   string
	long    string
	example string
	usage   string
	prefix  string
	action  artifact.Action
}

func newCollectionCommand(spec *collectionSpec) *cobra.Command {
	parser := newListParser(spec.prefix)
	c := &collection{action: spec.action, parser: parser, prefix: spec.prefix}
	cmd := &cobra.Command{
		Use:     spec.use,
		Short:   spec.short,
		Long:    spec.long,
		Example: spec.example,
		Args:    optionalArg(spec.usage),
		RunE:    c.execute,
	}
	parser.RegisterFlags(cmd)
	registerCompletions(cmd, listFormats)
	return cmd
}

// firstArg returns the optional positional argument.
func firstArg(in *input) string {
	if len(in.args) == 0 {
		return ""
	}
	return in.args[0]
}

func newListCommand() *cobra.Command {
	return newCollectionCommand(&collectionSpec{
		use:   "list [name]",
		short: "List artifacts and versions across the configured repositories",
		long: `List the artifacts and versions in one or more repositories. Without --repository, every configured
repository is listed, and each entry names the repository alias it came from. Pass an artifact name to
list only its versions. Labels are exact key=value selectors; every selector must match.`,
		example: markdown.ArtifactsListUsageMarkdown,
		usage:   "atmos artifacts list [name] [--repository=<alias>] [--labels=key=value]",
		prefix:  listPrefix,
		action:  artifact.ActionList,
	})
}

func newSearchCommand() *cobra.Command {
	return newCollectionCommand(&collectionSpec{
		use:   "search [text]",
		short: "Search artifacts by name, kind, description, and labels",
		long: `Search the artifacts in one or more repositories. The text matches names, kinds, descriptions, and
labels. Without --repository, every configured repository is searched. Searching reads repositories
and caches; it never mutates a catalog.`,
		example: markdown.ArtifactsSearchUsageMarkdown,
		usage:   "atmos artifacts search [text] [--repository=<alias>] [--labels=key=value]",
		prefix:  searchPrefix,
		action:  artifact.ActionSearch,
	})
}

// execute is the collection pipeline. Structured formats emit the `artifacts` array envelope, and
// a failed query still emits it (empty, with the error) while the exit code stays nonzero.
func (c *collection) execute(cmd *cobra.Command, args []string) error {
	defer perf.Track(atmosConfigPtr, "artifacts.collection.execute")()

	values := viper.New()
	if err := c.parser.BindFlagsToViper(cmd, values); err != nil {
		return err
	}
	in := &input{v: values, prefix: c.prefix, args: args}
	outputFormat := in.str(flagFormat)
	if err := validateFormat(outputFormat, listFormats); err != nil {
		return err
	}

	envelope, err := c.run(commandContext(cmd), in)
	if renderErr := renderCollection(outputFormat, envelope, err != nil); renderErr != nil && err == nil {
		return renderErr
	}
	return decorate(err, configuredAliases(atmosConfigPtr))
}

// listEnvelope is the structured result of list and search: the `artifacts` array plus
// per-repository outcomes, and the error when the query failed. The type is shared with the
// artifact step (see pkg/artifact/invoke), so the field names are identical everywhere.
type listEnvelope = invoke.Collection

// listedArtifact is one entry of the `artifacts` array.
type listedArtifact = invoke.Listed

// run maps the flags onto operation parameters and runs the query through the shared
// implementation. The envelope is never nil: a failed query still yields an empty array and the
// error text, while the returned error keeps the exit code nonzero.
func (c *collection) run(ctx context.Context, in *input) (*listEnvelope, error) {
	failed := func(err error) (*listEnvelope, error) {
		return &listEnvelope{Artifacts: []listedArtifact{}, Error: err.Error()}, err
	}
	if atmosConfigPtr == nil {
		return failed(errUtils.Build(errUtils.ErrArtifactConfigInvalid).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project directory that contains an `atmos.yaml`, or pass `--config`.").
			Err())
	}
	labels, err := in.labels()
	if err != nil {
		return failed(err)
	}
	svc, err := newService(atmosConfigPtr, ServiceOptions{})
	if err != nil {
		return failed(err)
	}
	params := &invoke.Params{Action: c.action, Labels: labels, Refresh: in.flag(flagRefresh)}
	if selected := in.v.GetStringSlice(in.key(flagRepository)); len(selected) > 0 {
		params.Repositories = selected
	}
	if c.action == artifact.ActionSearch {
		params.Query = firstArg(in)
	} else {
		params.Name = firstArg(in)
	}
	outcome, err := invoke.Run(ctx, svc, params)
	return outcome.Collection, err
}

// renderCollection writes a listing. Structured formats always render; table-like formats render
// only a successful listing, with an empty result reported as a UI message instead of an empty table.
func renderCollection(outputFormat string, envelope *listEnvelope, failed bool) error {
	if isStructured(outputFormat) {
		return writeStructured(outputFormat, envelope)
	}
	if failed {
		return nil
	}
	reportListingDiagnostics(envelope.Repositories)
	if len(envelope.Artifacts) == 0 {
		if len(configuredAliases(atmosConfigPtr)) == 0 {
			ui.Info("No artifact repositories are configured. Add repositories under `artifacts.repositories` in `atmos.yaml`.")
			return nil
		}
		ui.Info("No artifacts found.")
		return nil
	}
	return renderTable(outputFormat, envelope.Artifacts)
}

// reportListingDiagnostics sends per-repository notes (for example a capability that is missing) to stderr.
// A repository that could not be listed is reported by name, because the command still succeeds
// when other repositories were listed and the result must never read as complete.
func reportListingDiagnostics(repositories []artifact.RepositoryListing) {
	for index := range repositories {
		reportDiagnostics(repositories[index].Diagnostics)
		for _, diagnostic := range repositories[index].Diagnostics {
			if diagnostic.Severity == artifact.SeverityError {
				ui.Warningf("The listing is incomplete: repository `%s` could not be listed: %s", repositories[index].Repository, diagnostic.Message)
			}
		}
	}
}

// listColumns are the default table columns.
func listColumns() []column.Config {
	return []column.Config{
		{Name: "Repository", Value: "{{ .repository }}"},
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Version", Value: "{{ .version }}"},
		{Name: "Kind", Value: "{{ .kind }}"},
		{Name: "Description", Value: "{{ .description }}"},
		{Name: "Labels", Value: "{{ .labels }}"},
	}
}

// renderTable renders entries through the shared list renderer.
func renderTable(outputFormat string, entries []listedArtifact) error {
	selector, err := column.NewSelector(listColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(entries))
	for index := range entries {
		entry := &entries[index]
		rows = append(rows, map[string]any{
			"repository":  entry.Repository,
			"name":        entry.Name,
			"version":     entry.Version,
			"kind":        entry.Kind,
			"description": entry.Description,
			"labels":      formatLabels(entry.Labels),
		})
	}
	return renderer.New(nil, selector, nil, format.Format(outputFormat), "").Render(rows)
}
