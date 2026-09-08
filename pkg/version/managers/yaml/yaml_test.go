package yaml

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

var testRefs = map[string]manager.VersionRef{
	"opentofu":     {Version: "1.10.6"},
	"nginx":        {Version: "1.29.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Pin: manager.PinDigest},
	"cli":          {Version: "v2.5.0"},
	"bare-numeric": {Version: "5"},
}

func setOptions(entries ...setEntry) map[string]any {
	raw := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		raw = append(raw, map[string]any{"path": e.Path, "from": e.From, "format": e.Format})
	}
	return map[string]any{"set": raw}
}

func planFixture(t *testing.T, name, content string, options map[string]any) (string, []managers.FileChange) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{name},
		Refs:    testRefs,
		Options: options,
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	return path, changes
}

// planFixtureErr is planFixture's error-path counterpart: it returns Plan's
// error instead of failing the test, for cases that assert Plan fails.
func planFixtureErr(t *testing.T, name, content string, options map[string]any) error {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{name},
		Refs:    testRefs,
		Options: options,
	})
	return err
}

func TestYAMLSetsValueAtPath(t *testing.T) {
	path, changes := planFixture(t, "values.yaml",
		"name: atmos\nversion: 1.0.0\n",
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 1.10.6`)) {
		t.Fatalf("expected version rewrite, got:\n%s", changes[0].New)
	}
	if changes[0].Path != path {
		t.Fatalf("expected changed path %q, got %q", path, changes[0].Path)
	}
}

// fixtureWithCommentsAndAnchor exercises the whole point of building the yaml
// manager on Atmos's own format-preserving editor (pkg/yaml) instead of a
// naive unmarshal/remarshal: comments, an anchor/alias pair, and unrelated
// key order must all survive a single-field edit untouched.
const fixtureWithCommentsAndAnchor = `# Header comment.
defaults: &defaults
  region: us-east-1
vars:
  # Region used everywhere.
  region: us-east-1  # inline
  <<: *defaults
version: 1.0.0
`

// TestYAMLPreservesCommentsAndAnchors guards against a regression to a plain
// unmarshal-into-map/remarshal approach, which would silently drop every
// comment and collapse the anchor/alias into a duplicated literal.
func TestYAMLPreservesCommentsAndAnchors(t *testing.T) {
	_, changes := planFixture(t, "values.yaml", fixtureWithCommentsAndAnchor,
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	for _, want := range []string{"# Header comment.", "# Region used everywhere.", "# inline", "&defaults", "*defaults"} {
		if !bytes.Contains(changes[0].New, []byte(want)) {
			t.Fatalf("expected %q preserved, got:\n%s", want, changes[0].New)
		}
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 1.10.6`)) {
		t.Fatalf("expected version rewrite, got:\n%s", changes[0].New)
	}
}

func TestYAMLNoOpWhenValueMatches(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		`version: "1.10.6"`+"\n",
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

// TestYAMLCreatesMissingKey mirrors the json manager's behavior: a simple
// path that doesn't exist yet in the document is created rather than
// rejected.
func TestYAMLCreatesMissingKey(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"name: atmos\n",
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 1.10.6`)) {
		t.Fatalf("expected version created, got:\n%s", changes[0].New)
	}
}

// TestYAMLDigestPinnedEntryUsesDigest guards against regressing to
// ref.Version for a digest-pinned entry: the manager must call ref.String(),
// which resolves to the locked digest, not the plain version.
func TestYAMLDigestPinnedEntryUsesDigest(t *testing.T) {
	_, changes := planFixture(t, "deploy.yaml",
		"image:\n  digest: sha256:old\n",
		setOptions(setEntry{Path: "image.digest", From: "nginx"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	want := `digest: ` + testRefs["nginx"].Digest
	if !bytes.Contains(changes[0].New, []byte(want)) {
		t.Fatalf("expected digest rewrite %q, got:\n%s", want, changes[0].New)
	}
	if bytes.Contains(changes[0].New, []byte(testRefs["nginx"].Version)) {
		t.Fatalf("expected plain version not to appear for a digest-pinned entry, got:\n%s", changes[0].New)
	}
}

// TestYAMLFormatUnsetIsVerbatim guards against a regression where the Format
// field changes behavior for entries that don't set it: an empty Format must
// still write ref.String() verbatim.
func TestYAMLFormatUnsetIsVerbatim(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"version: 1.0.0\n",
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 1.10.6`)) {
		t.Fatalf("expected verbatim version rewrite, got:\n%s", changes[0].New)
	}
}

// TestYAMLFormatTrimsVersionPrefix exercises the primary use case: reshaping
// a "v"-prefixed tag into bare semver for a target field that doesn't use the
// "v" convention.
func TestYAMLFormatTrimsVersionPrefix(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"version: 1.0.0\n",
		setOptions(setEntry{Path: "version", From: "cli", Format: `{{ trimPrefix "v" .Version }}`}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 2.5.0`)) {
		t.Fatalf("expected formatted version rewrite, got:\n%s", changes[0].New)
	}
	if bytes.Contains(changes[0].New, []byte(testRefs["cli"].Version)) {
		t.Fatalf("expected unformatted %q not to appear, got:\n%s", testRefs["cli"].Version, changes[0].New)
	}
}

// TestYAMLFormatInvalidTemplateErrors guards against a bad Format template
// silently writing garbage: both a syntax error and a reference to a field
// that doesn't exist on manager.VersionRef (which would otherwise render
// "<no value>") must surface as a clear, wrapped error.
func TestYAMLFormatInvalidTemplateErrors(t *testing.T) {
	for name, format := range map[string]string{
		"malformed syntax": `{{ .Version`,
		"undefined field":  `{{ .Bogus }}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := planFixtureErr(t, "values.yaml",
				"version: 1.0.0\n",
				setOptions(setEntry{Path: "version", From: "opentofu", Format: format}))
			if err == nil {
				t.Fatalf("expected an error for format %q", format)
			}
			if !errors.Is(err, errUtils.ErrVersionYAMLFormatInvalid) {
				t.Fatalf("expected error to wrap ErrVersionYAMLFormatInvalid, got: %v", err)
			}
		})
	}
}

// TestYAMLFormatHonorsConfiguredDelimiters guards against the Format path
// ignoring a project's `templates.settings.delimiters`: with custom
// delimiters configured, a Format written in them must render rather than
// being copied into the file as a literal string.
func TestYAMLFormatHonorsConfiguredDelimiters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("version: 1.0.0\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Templates.Settings.Delimiters = []string{"<<", ">>"}
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Config:  atmosConfig,
		Dir:     dir,
		Paths:   []string{"values.yaml"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "cli", Format: `<< trimPrefix "v" .Version >>`}),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 2.5.0`)) {
		t.Fatalf("expected custom-delimiter format to render, got:\n%s", changes[0].New)
	}
}

func TestYAMLUnknownEntryIgnored(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"version: 1.0.0\n",
		setOptions(setEntry{Path: "version", From: "does-not-exist"}))
	if len(changes) != 0 {
		t.Fatalf("expected unknown entry to be ignored, got %d changes", len(changes))
	}
}

func TestYAMLEmptyOptionsIsNoOp(t *testing.T) {
	for name, options := range map[string]map[string]any{
		"nil options":     nil,
		"empty set list":  {"set": []map[string]any{}},
		"absent set list": {},
	} {
		t.Run(name, func(t *testing.T) {
			_, changes := planFixture(t, "values.yaml", "version: 1.0.0\n", options)
			if len(changes) != 0 {
				t.Fatalf("expected no changes, got %d", len(changes))
			}
		})
	}
}

func TestYAMLNoPathsIsNoOp(t *testing.T) {
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     t.TempDir(),
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

func TestYAMLMissingFileIsNoOp(t *testing.T) {
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     t.TempDir(),
		Paths:   []string{"does-not-exist.yaml"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

func TestYAMLDefaultPathsIsNil(t *testing.T) {
	var m Manager
	if paths := m.DefaultPaths(); paths != nil {
		t.Fatalf("expected nil default paths, got %v", paths)
	}
}

func TestYAMLInvalidOptionsErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("version: 1.0.0\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"values.yaml"},
		Refs:    testRefs,
		Options: map[string]any{"set": "not-a-list"},
	})
	if err == nil {
		t.Fatal("expected an error for a malformed options.set shape")
	}
}

func TestYAMLBadGlobPatternErrors(t *testing.T) {
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     t.TempDir(),
		Paths:   []string{"["},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for a malformed glob pattern")
	}
}

// TestYAMLContainerPathRejected guards against a regression to the json
// manager's field-test finding, applied here too: a `path` pointing at a map
// silently replacing the whole subtree with a scalar string, destroying its
// contents.
func TestYAMLContainerPathRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	original := "engines:\n  node: 18.0.0\n  npm: 9.0.0\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"values.yaml"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "engines", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for a path targeting a map value")
	}
	if !errors.Is(err, errUtils.ErrVersionYAMLPathTypeMismatch) {
		t.Fatalf("expected error to wrap ErrVersionYAMLPathTypeMismatch, got: %v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading fixture: %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("expected file untouched, got:\n%s", content)
	}
}

// TestYAMLDuplicatePathRejected guards against two set entries targeting the
// identical path silently last-winning, discarding the first write with no
// error.
func TestYAMLDuplicatePathRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte("version: 1.0.0\n"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:   dir,
		Paths: []string{"values.yaml"},
		Refs:  testRefs,
		Options: setOptions(
			setEntry{Path: "version", From: "opentofu"},
			setEntry{Path: "version", From: "cli"},
		),
	})
	if err == nil {
		t.Fatal("expected an error for duplicate set entries targeting the same path")
	}
	if !errors.Is(err, errUtils.ErrVersionYAMLDuplicatePath) {
		t.Fatalf("expected error to wrap ErrVersionYAMLDuplicatePath, got: %v", err)
	}
}

func TestYAMLMultipleSetEntriesInOneFile(t *testing.T) {
	_, changes := planFixture(t, "build.yaml",
		"cliVersion: 1.0.0\ntoolVersion: 1.0.0\n",
		setOptions(
			setEntry{Path: "cliVersion", From: "cli"},
			setEntry{Path: "toolVersion", From: "opentofu"},
		))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`cliVersion: v2.5.0`)) {
		t.Fatalf("expected cliVersion rewrite, got:\n%s", changes[0].New)
	}
	if !bytes.Contains(changes[0].New, []byte(`toolVersion: 1.10.6`)) {
		t.Fatalf("expected toolVersion rewrite, got:\n%s", changes[0].New)
	}
}

// TestYAMLIndexedPath exercises the bracket-index dot-path syntax shared with
// `atmos config set`/`atmos stack set` (e.g. sources[0].version), distinct
// from the json manager's sjson-style dialect.
func TestYAMLIndexedPath(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"sources:\n  - component: vpc\n    version: 1.0.0\n",
		setOptions(setEntry{Path: "sources[0].version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`version: 1.10.6`)) {
		t.Fatalf("expected indexed path rewrite, got:\n%s", changes[0].New)
	}
}

// TestYAMLBareNumericVersionStaysString guards against a regression where a
// bare-digit version (e.g. "5", with no dots to force string disambiguation)
// silently round-trips as a YAML !!int instead of a string: atmosyaml.Set
// must write it in a form that reads back with a !!str tag.
func TestYAMLBareNumericVersionStaysString(t *testing.T) {
	_, changes := planFixture(t, "values.yaml",
		"version: 1.0.0\n",
		setOptions(setEntry{Path: "version", From: "bare-numeric"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	tag, ok := atmosyaml.GetType(changes[0].New, "version")
	if !ok {
		t.Fatalf("expected version path to resolve, got:\n%s", changes[0].New)
	}
	if tag != atmosyaml.TypeString {
		t.Fatalf("expected version to stay a YAML string, got type %q for:\n%s", tag, changes[0].New)
	}
}
