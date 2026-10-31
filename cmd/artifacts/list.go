package artifacts

import (
	"context"
	"sort"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/markdown"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// collectionQuery runs a list or a search against the service.
type collectionQuery func(ctx context.Context, svc artifact.Service, in *input, repositories []string, labels []artifact.Label) (*artifact.ListResult, error)

// collection describes one collection-producing subcommand (list or search).
type collection struct {
	parser *flags.StandardParser
	prefix string
	query  collectionQuery
}

// collectionSpec describes how list and search differ; everything else is shared.
type collectionSpec struct {
	use     string
	short   string
	long    string
	example string
	usage   string
	prefix  string
	query   collectionQuery
}

func newCollectionCommand(spec *collectionSpec) *cobra.Command {
	parser := newListParser(spec.prefix)
	c := &collection{parser: parser, prefix: spec.prefix, query: spec.query}
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
		query: func(ctx context.Context, svc artifact.Service, in *input, repositories []string, labels []artifact.Label) (*artifact.ListResult, error) {
			return svc.List(ctx, artifact.ListRequest{Repositories: repositories, Name: firstArg(in), Labels: labels, Refresh: in.flag(flagRefresh)})
		},
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
		query: func(ctx context.Context, svc artifact.Service, in *input, repositories []string, labels []artifact.Label) (*artifact.ListResult, error) {
			return svc.Search(ctx, artifact.SearchRequest{Repositories: repositories, Query: firstArg(in), Labels: labels, Refresh: in.flag(flagRefresh)})
		},
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

	listing, err := c.run(commandContext(cmd), in)
	envelope := &listEnvelope{Artifacts: []listedArtifact{}}
	if listing != nil {
		envelope.Artifacts = listed(listing.Artifacts)
		envelope.Repositories = listing.Repositories
	}
	if err != nil {
		envelope.Error = err.Error()
	}
	if renderErr := renderCollection(outputFormat, envelope, err != nil); renderErr != nil && err == nil {
		return renderErr
	}
	return decorate(err, configuredAliases(atmosConfigPtr))
}

// listEnvelope is the structured result of list and search: the `artifacts` array plus
// per-repository outcomes, and the error when the query failed.
type listEnvelope struct {
	// Artifacts are the entries across the selected repositories, sorted lexically.
	Artifacts []listedArtifact `json:"artifacts"`

	// Repositories reports per-repository outcomes.
	Repositories []artifact.RepositoryListing `json:"repositories,omitempty"`

	// Error is set when the query failed.
	Error string `json:"error,omitempty"`
}

// listedArtifact is one entry of the `artifacts` array. It carries the named outputs of an
// operation result (ref, resolved_digest, record_digest, content_digest) next to the listing
// fields, plus the pinned reference a consumer lockfile persists.
type listedArtifact struct {
	Repository     string                   `json:"repository,omitempty"`
	Ref            string                   `json:"ref"`
	Name           string                   `json:"name"`
	Version        string                   `json:"version"`
	Kind           string                   `json:"kind"`
	Description    string                   `json:"description,omitempty"`
	Labels         map[string]string        `json:"labels,omitempty"`
	Documents      []artifact.Document      `json:"documents,omitempty"`
	ResolvedDigest digest.Digest            `json:"resolved_digest,omitempty"`
	RecordDigest   digest.Digest            `json:"record_digest,omitempty"`
	ContentDigest  digest.Digest            `json:"content_digest,omitempty"`
	Pinned         artifact.PinnedReference `json:"pinned"`
}

// listed converts service entries into envelope entries.
func listed(entries []artifact.ListEntry) []listedArtifact {
	out := make([]listedArtifact, 0, len(entries))
	for index := range entries {
		entry := &entries[index]
		out = append(out, listedArtifact{
			Repository:     entry.Repository,
			Ref:            artifact.Ref{Name: entry.Name, Version: entry.Version}.String(),
			Name:           entry.Name,
			Version:        entry.Version,
			Kind:           entry.Kind,
			Description:    entry.Description,
			Labels:         entry.Labels,
			Documents:      entry.Documents,
			ResolvedDigest: entry.Pinned.ResolvedDigest,
			RecordDigest:   entry.Pinned.RecordDigest,
			ContentDigest:  entry.Pinned.ContentDigest,
			Pinned:         entry.Pinned,
		})
	}
	return out
}

func (c *collection) run(ctx context.Context, in *input) (*artifact.ListResult, error) {
	if atmosConfigPtr == nil {
		return nil, errUtils.Build(errUtils.ErrArtifactConfigInvalid).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project directory that contains an `atmos.yaml`, or pass `--config`.").
			Err()
	}
	labels, err := selectorLabels(in)
	if err != nil {
		return nil, err
	}
	svc, err := newService(atmosConfigPtr, ServiceOptions{})
	if err != nil {
		return nil, err
	}
	var repositories []string
	if selected := in.v.GetStringSlice(in.key(flagRepository)); len(selected) > 0 {
		repositories = selected
	}
	return c.query(ctx, svc, in, repositories, labels)
}

// selectorLabels parses --labels into sorted exact key/value selectors.
func selectorLabels(in *input) ([]artifact.Label, error) {
	parsed, err := in.labels()
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(parsed))
	for key := range parsed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels := make([]artifact.Label, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, artifact.Label{Key: key, Value: parsed[key]})
	}
	return labels, nil
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
func reportListingDiagnostics(repositories []artifact.RepositoryListing) {
	for index := range repositories {
		reportDiagnostics(repositories[index].Diagnostics)
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
