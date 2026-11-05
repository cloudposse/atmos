package invoke

import (
	"slices"
	"sort"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Field names an input of an artifact operation. The values are the artifact step field names,
// which are also the CLI flag names where one exists (`--name` is artifact_name, `--kind` is
// artifact_kind, `--output` is destination).
type Field string

// Operation inputs.
const (
	FieldSource       Field = "source"
	FieldRepository   Field = "repository"
	FieldArtifactName Field = "artifact_name"
	FieldVersion      Field = "version"
	FieldArtifactKind Field = "artifact_kind"
	FieldForce        Field = "force"
	FieldDryRun       Field = "dry_run"
	FieldLabels       Field = "labels"
	FieldMetadata     Field = "metadata.labels"
	FieldReadme       Field = "readme"
	FieldChangelog    Field = "changelog"
	FieldRef          Field = "ref"
	FieldDestination  Field = "destination"
	FieldFrom         Field = "from"
	FieldTo           Field = "to"
	FieldDocument     Field = "document"
	FieldQuery        Field = "query"
	FieldRefresh      Field = "refresh"
)

// Spec lists the inputs an action requires and the optional inputs it also accepts. Any other
// input is invalid for the action.
type Spec struct {
	// Required inputs must be present and non-empty.
	Required []Field

	// Optional inputs may be present.
	Optional []Field
}

// specs is the contract table: "Public command and step mapping" in the artifacts PRD.
var specs = map[artifact.Action]Spec{
	artifact.ActionPublish: {
		Required: []Field{FieldSource, FieldRepository},
		Optional: []Field{FieldArtifactName, FieldVersion, FieldArtifactKind, FieldForce, FieldDryRun, FieldMetadata, FieldReadme, FieldChangelog},
	},
	artifact.ActionPull: {
		Required: []Field{FieldRef, FieldRepository, FieldDestination},
		Optional: []Field{FieldForce},
	},
	artifact.ActionCopy: {
		Required: []Field{FieldRef, FieldFrom, FieldTo},
		Optional: []Field{FieldForce},
	},
	artifact.ActionInspect: {Required: []Field{FieldRef, FieldRepository}},
	artifact.ActionSign:    {Required: []Field{FieldRef, FieldRepository}},
	artifact.ActionVerify:  {Required: []Field{FieldRef, FieldRepository}},
	artifact.ActionDelete:  {Required: []Field{FieldRef, FieldRepository}},
	artifact.ActionShow:    {Required: []Field{FieldDocument, FieldRef, FieldRepository}},
	artifact.ActionList:    {Optional: []Field{FieldArtifactName, FieldRepository, FieldLabels, FieldRefresh}},
	artifact.ActionSearch:  {Optional: []Field{FieldQuery, FieldRepository, FieldLabels, FieldRefresh}},
}

// SpecFor returns the input contract of an action. The second result is false for an action that
// is not available as a CLI command or step action.
func SpecFor(action artifact.Action) (Spec, bool) {
	defer perf.Track(nil, "invoke.SpecFor")()

	spec, ok := specs[action]
	return spec, ok
}

// Actions lists the actions in a stable order.
func Actions() []artifact.Action {
	defer perf.Track(nil, "invoke.Actions")()

	actions := make([]artifact.Action, 0, len(specs))
	for action := range specs {
		actions = append(actions, action)
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i] < actions[j] })
	return actions
}

// Accepts reports whether the field is required or optional for the action.
func (s Spec) Accepts(field Field) bool {
	defer perf.Track(nil, "invoke.Spec.Accepts")()

	return slices.Contains(s.Required, field) || slices.Contains(s.Optional, field)
}

// Fields lists every input the action accepts, required first.
func (s Spec) Fields() []Field {
	defer perf.Track(nil, "invoke.Spec.Fields")()

	return append(slices.Clone(s.Required), s.Optional...)
}

// AllFields lists every input of every action in a stable order.
func AllFields() []Field {
	defer perf.Track(nil, "invoke.AllFields")()

	seen := make(map[Field]bool)
	var all []Field
	for _, action := range Actions() {
		for _, field := range specs[action].Fields() {
			if !seen[field] {
				seen[field] = true
				all = append(all, field)
			}
		}
	}
	return all
}

// Params is the transport-neutral input of one artifact operation: the union of the CLI flags and
// the artifact step fields. Which fields an action reads is described by [SpecFor].
type Params struct {
	// Action selects the operation.
	Action artifact.Action

	// Repository is the configured repository alias for single-repository actions.
	Repository string

	// Repositories restricts list and search; empty means all configured repositories.
	Repositories []string

	// Source is the publish source file or directory, already resolved to a path.
	Source string

	// Name is the artifact name (--name; artifact_name). List uses it to restrict to one name.
	Name string

	// Version is the explicit publish version.
	Version string

	// Kind is the artifact kind (--kind; artifact_kind). Empty means the files kind.
	Kind string

	// Force permits replacing conflicting state where the repository allows it.
	Force bool

	// DryRun validates and plans a publication without writing.
	DryRun bool

	// Refresh bypasses cached discovery snapshots for list and search.
	Refresh bool

	// Labels are the publish metadata labels, or the exact list and search selectors.
	Labels map[string]string

	// Readme and Changelog override the document source paths, relative to the source directory.
	Readme, Changelog string

	// Ref is the artifact reference exactly as the caller typed it.
	Ref string

	// Destination is the pull output directory (--output).
	Destination string

	// From and To are the copy source and destination repository aliases.
	From, To string

	// Document is readme or changelog for show.
	Document string

	// Query is the search text.
	Query string

	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution artifact.Execution
}
