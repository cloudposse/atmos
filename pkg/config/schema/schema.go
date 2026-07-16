// Package configschema generates the JSON Schema for `atmos.yaml` (the Atmos CLI
// configuration) by reflecting over schema.AtmosConfiguration — the exact struct
// Atmos decodes `atmos.yaml` into — so the published schema can never drift from
// the code. Go doc comments on the configuration structs become schema
// descriptions.
//
// The committed artifact at pkg/datafetcher/schema/atmos/config/1.0.json is
// regenerated with `go generate ./pkg/config/schema` and guarded by a drift test
// that fails whenever the configuration structs change without a regeneration.
// The atmos binary never imports this package; generation happens at development
// and test time only.
package configschema

//go:generate go run ./generator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/invopop/jsonschema"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// SchemaID is the canonical `$id` of the generated schema — the floating
	// (always-latest) URL published on atmos.tools. Per-release pinned copies are
	// published under `.../atmos-config/<version>/atmos-config.json`.
	SchemaID = "https://atmos.tools/schemas/atmos/atmos-config/1.0/atmos-config.json"

	// EmbeddedSource is the `atmos://` source the embedded copy of the schema is
	// served from (see pkg/datafetcher).
	EmbeddedSource = "atmos://schema/atmos/config/1.0"

	// EmbeddedPath is the repository-relative path of the committed schema
	// artifact embedded into the binary by pkg/datafetcher.
	EmbeddedPath = "pkg/datafetcher/schema/atmos/config/1.0.json"

	// The Go module path, used to key extracted doc comments so the reflector
	// can match them to types by package import path.
	modulePath = "github.com/cloudposse/atmos"
)

// ErrRepoRootNotFound indicates the repository root could not be located from
// this source file, which means generation is running outside a source checkout.
var ErrRepoRootNotFound = errors.New("failed to locate the repository root from the configschema source file")

// Generate reflects schema.AtmosConfiguration into the atmos.yaml JSON Schema
// document. It requires the repository's Go source on disk (doc comments become
// schema descriptions), so it runs at generate/test time only — never inside the
// shipped binary.
func Generate() ([]byte, error) {
	defer perf.Track(nil, "configschema.Generate")()

	root, err := RepoRoot()
	if err != nil {
		return nil, err
	}

	r := &jsonschema.Reflector{
		// Property names come from the yaml struct tags — the keys users author
		// in atmos.yaml; fields tagged `yaml:"-"` drop out automatically.
		FieldNameTag: "yaml",
		// atmos.yaml has no required fields: partial configs (atmos.d fragments,
		// imports) must validate on their own.
		RequiredFromJSONSchemaTags: true,
		// Mirror the loader: Viper ignores unknown keys, so the schema must not
		// reject them.
		AllowAdditionalProperties: true,
		// Inline AtmosConfiguration at the document root instead of $ref-ing it.
		ExpandedStruct: true,
		// Types whose authored YAML forms differ from their Go representation
		// (durations, polymorphic conditions).
		Mapper: typeMapper,
	}
	if err := addGoComments(r, root); err != nil {
		return nil, err
	}

	s := r.Reflect(&schema.AtmosConfiguration{})
	applyOverrides(r, s)

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// RepoRoot locates the repository root relative to this source file. The
// generator and its tests always run from a source checkout (this package is
// never compiled into the atmos binary), so runtime.Caller is reliable here.
func RepoRoot() (string, error) {
	defer perf.Track(nil, "configschema.RepoRoot")()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", ErrRepoRootNotFound
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..")), nil
}

// addGoComments extracts Go doc comments from pkg/ so they become schema
// descriptions. AddGoComments builds its comment-map keys by joining the base
// import path with the walked path verbatim, so it must run from the repository
// root with a relative path.
func addGoComments(r *jsonschema.Reflector, repoRoot string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(repoRoot); err != nil {
		return err
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	// WithFullComment keeps complete type doc comments instead of go/doc
	// synopses, which truncate at abbreviations like "e.g.".
	return r.AddGoComments(modulePath, "pkg", jsonschema.WithFullComment())
}
