package json

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/tidwall/gjson"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

var testRefs = map[string]manager.VersionRef{
	"opentofu": {Version: "1.10.6"},
	"nginx":    {Version: "1.29.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Pin: manager.PinDigest},
	"cli":      {Version: "v2.5.0"},
}

func setOptions(entries ...setEntry) map[string]any {
	raw := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		raw = append(raw, map[string]any{"path": e.Path, "from": e.From})
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

func TestJSONSetsValueAtPath(t *testing.T) {
	_, changes := planFixture(t, "plugin.json",
		`{"name": "atmos", "version": "1.0.0"}`,
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`"version": "1.10.6"`)) {
		t.Fatalf("expected version rewrite, got:\n%s", changes[0].New)
	}
}

// TestJSONPreservesUnrelatedFormatting proves the core selling point of using
// sjson over a full unmarshal/remarshal: unusual spacing, out-of-order keys,
// an unsorted array, and trailing whitespace on unrelated lines survive
// byte-for-byte -- only the targeted value's bytes change.
func TestJSONPreservesUnrelatedFormatting(t *testing.T) {
	original := "{\n" +
		"  \"name\":    \"atmos-plugin\",\n" +
		"  \"version\": \"1.0.0\",\n" +
		"  \"keywords\": [\"b\", \"a\", \"c\"],\n" +
		"  \"nested\": {\n" +
		"    \"z\": 1,\n" +
		"    \"a\": 2\n" +
		"  },\n" +
		"  \"trailingSpace\": \"value\"   \n" +
		"}\n"
	path, changes := planFixture(t, "plugin.json", original,
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	want := bytes.Replace([]byte(original), []byte(`"1.0.0"`), []byte(`"1.10.6"`), 1)
	if !bytes.Equal(changes[0].New, want) {
		t.Fatalf("expected only the version value to change, got:\nwant:\n%s\ngot:\n%s", want, changes[0].New)
	}
	if changes[0].Path != path {
		t.Fatalf("expected changed path %q, got %q", path, changes[0].Path)
	}
}

func TestJSONNoOpWhenValueMatches(t *testing.T) {
	_, changes := planFixture(t, "plugin.json",
		`{"version": "1.10.6"}`,
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

// TestJSONNumberIsRewrittenToString guards against a regression where a
// numeric JSON value (e.g. `"version": 1`) was wrongly treated as already
// matching a locked string value with the same text: gjson.Result.String()
// renders "1" for both the JSON number 1 and the JSON string "1", but
// sjson.SetBytes always writes a Go string as a JSON string, so the field
// must be rewritten to {"version": "1"} rather than left as a number.
func TestJSONNumberIsRewrittenToString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, []byte(`{"version": 1}`), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"plugin.json"},
		Refs:    map[string]manager.VersionRef{"tool": {Version: "1"}},
		Options: setOptions(setEntry{Path: "version", From: "tool"}),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	result := gjson.GetBytes(changes[0].New, "version")
	if result.Type != gjson.String || result.String() != "1" {
		t.Fatalf("expected version to become the JSON string \"1\", got type=%v raw=%s", result.Type, result.Raw)
	}
}

func TestJSONMultipleSetEntriesInOneFile(t *testing.T) {
	_, changes := planFixture(t, "build.json",
		`{"cliVersion": "1.0.0", "toolVersion": "1.0.0"}`,
		setOptions(
			setEntry{Path: "cliVersion", From: "cli"},
			setEntry{Path: "toolVersion", From: "opentofu"},
		))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !bytes.Contains(changes[0].New, []byte(`"cliVersion": "v2.5.0"`)) {
		t.Fatalf("expected cliVersion rewrite, got:\n%s", changes[0].New)
	}
	if !bytes.Contains(changes[0].New, []byte(`"toolVersion": "1.10.6"`)) {
		t.Fatalf("expected toolVersion rewrite, got:\n%s", changes[0].New)
	}
}

func TestJSONMultipleFilesInOneRule(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.json": `{"version": "1.0.0"}`,
		"b.json": `{"version": "1.0.0"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("writing fixture %s: %v", name, err)
		}
	}
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"a.json", "b.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	for _, change := range changes {
		if !bytes.Contains(change.New, []byte(`"version": "1.10.6"`)) {
			t.Fatalf("expected version rewrite in %s, got:\n%s", change.Path, change.New)
		}
	}
}

func TestJSONIdempotency(t *testing.T) {
	path, changes := planFixture(t, "plugin.json",
		`{"version": "1.0.0"}`,
		setOptions(setEntry{Path: "version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if err := managers.Apply([]managers.PlannedChange{{Manager: Name, FileChange: changes[0]}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	var m Manager
	again, err := m.Plan(context.Background(), &managers.Input{
		Dir:     filepath.Dir(path),
		Paths:   []string{filepath.Base(path)},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent apply, got %d changes", len(again))
	}
}

// TestJSONDigestPinnedEntryUsesDigest guards against regressing to ref.Version
// for a digest-pinned entry: the manager must call ref.String(), which
// resolves to the locked digest, not the plain version.
func TestJSONDigestPinnedEntryUsesDigest(t *testing.T) {
	_, changes := planFixture(t, "deploy.json",
		`{"image": {"digest": "sha256:old"}}`,
		setOptions(setEntry{Path: "image.digest", From: "nginx"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	want := `"digest": "` + testRefs["nginx"].Digest + `"`
	if !bytes.Contains(changes[0].New, []byte(want)) {
		t.Fatalf("expected digest rewrite %q, got:\n%s", want, changes[0].New)
	}
	if bytes.Contains(changes[0].New, []byte(testRefs["nginx"].Version)) {
		t.Fatalf("expected plain version not to appear for a digest-pinned entry, got:\n%s", changes[0].New)
	}
}

func TestJSONUnknownEntryIgnored(t *testing.T) {
	_, changes := planFixture(t, "plugin.json",
		`{"version": "1.0.0"}`,
		setOptions(setEntry{Path: "version", From: "does-not-exist"}))
	if len(changes) != 0 {
		t.Fatalf("expected unknown entry to be ignored, got %d changes", len(changes))
	}
}

func TestJSONEmptyOptionsIsNoOp(t *testing.T) {
	for name, options := range map[string]map[string]any{
		"nil options":     nil,
		"empty set list":  {"set": []map[string]any{}},
		"absent set list": {},
	} {
		t.Run(name, func(t *testing.T) {
			_, changes := planFixture(t, "plugin.json", `{"version": "1.0.0"}`, options)
			if len(changes) != 0 {
				t.Fatalf("expected no changes, got %d", len(changes))
			}
		})
	}
}

func TestJSONNoPathsIsNoOp(t *testing.T) {
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

func TestJSONInvalidContentErrors(t *testing.T) {
	_, err := (Manager{}).Plan(context.Background(), &managers.Input{
		Dir:     t.TempDir(),
		Paths:   []string{"broken.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	// No file has been written yet, so ExpandPaths matches nothing and Plan
	// succeeds with zero changes; write the broken fixture and retry to
	// exercise the invalid-content path.
	if err != nil {
		t.Fatalf("unexpected error before fixture exists: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	_, err = (Manager{}).Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"broken.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "version", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for invalid JSON content")
	}
}

func TestJSONMissingFileIsNoOp(t *testing.T) {
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:     t.TempDir(),
		Paths:   []string{"does-not-exist.json"},
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

func TestJSONDefaultPathsIsNil(t *testing.T) {
	var m Manager
	if paths := m.DefaultPaths(); paths != nil {
		t.Fatalf("expected nil default paths, got %v", paths)
	}
}

func TestJSONInvalidOptionsErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"version": "1.0.0"}`), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"plugin.json"},
		Refs:    testRefs,
		Options: map[string]any{"set": "not-a-list"},
	})
	if err == nil {
		t.Fatal("expected an error for a malformed options.set shape")
	}
}

func TestJSONBadPathErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"version": "1.0.0"}`), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	// An empty sjson path is rejected by sjson.SetBytes, exercising the
	// applySets/Plan error-wrapping path.
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"plugin.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for an empty sjson path")
	}
}

func TestJSONBadGlobPatternErrors(t *testing.T) {
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

// TestJSONAppendPathRejected guards against a regression to the field-test
// finding that "-1" (sjson's array-append marker) grows the target array by
// one element on every single apply, forever: there is no way to tell
// "already applied" from "not yet applied" by reading an appended array back,
// so the path is rejected outright instead of silently corrupting the file.
func TestJSONAppendPathRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	original := `{"versions": ["1.0.0"]}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"data.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "versions.-1", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for an array-append path")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading fixture: %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("expected the file to remain untouched, got:\n%s", content)
	}
}

// TestJSONMidPathAppendSegmentRejected guards against a regression where
// isAppendPath only checked the trailing segment: a mid-path "-1" (e.g.
// items.-1.version) appends a new array element just as unsafely as a
// standalone or trailing one, so it must be rejected too.
func TestJSONMidPathAppendSegmentRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	original := `{"items": [{"version": "1.0.0"}]}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"data.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "items.-1.version", From: "opentofu"}),
	})
	if !errors.Is(err, errUtils.ErrVersionJSONAppendPathUnsupported) {
		t.Fatalf("expected ErrVersionJSONAppendPathUnsupported, got: %v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading fixture: %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("expected the file to remain untouched, got:\n%s", content)
	}
}

// TestJSONContainerPathRejected guards against a regression to the field-test
// finding that a `path` pointing at an object or array silently replaces the
// whole subtree with a scalar string, destroying its contents.
func TestJSONContainerPathRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	original := `{"engines": {"node": "18.0.0", "npm": "9.0.0"}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:     dir,
		Paths:   []string{"package.json"},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "engines", From: "opentofu"}),
	})
	if err == nil {
		t.Fatal("expected an error for a path targeting a container value")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading fixture: %v", readErr)
	}
	if string(content) != original {
		t.Fatalf("expected the file to remain untouched, got:\n%s", content)
	}
}

// TestJSONComplexPathUpdatesAllMatches documents a working, previously
// undocumented capability: a wildcard/query path updates every matching
// element, and is idempotent on a second run.
func TestJSONComplexPathUpdatesAllMatches(t *testing.T) {
	path, changes := planFixture(t, "data.json",
		`{"items": [{"name": "a", "version": "1.0.0"}, {"name": "b", "version": "1.0.0"}]}`,
		setOptions(setEntry{Path: "items.#.version", From: "opentofu"}))
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if bytes.Count(changes[0].New, []byte(`"version": "1.10.6"`)) != 2 {
		t.Fatalf("expected both array elements updated, got:\n%s", changes[0].New)
	}

	if err := managers.Apply([]managers.PlannedChange{{Manager: Name, FileChange: changes[0]}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	var m Manager
	again, err := m.Plan(context.Background(), &managers.Input{
		Dir:     filepath.Dir(path),
		Paths:   []string{filepath.Base(path)},
		Refs:    testRefs,
		Options: setOptions(setEntry{Path: "items.#.version", From: "opentofu"}),
	})
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent apply, got %d changes", len(again))
	}
}

// TestJSONComplexPathNotFoundErrors guards against a regression to the
// field-test finding that a wildcard/query path which doesn't currently
// resolve to anything (empty array, or a missing parent key) silently
// produces zero changes and zero errors, giving no signal the configured
// field was never written.
func TestJSONComplexPathNotFoundErrors(t *testing.T) {
	tests := map[string]string{
		"empty array":    `{"items": []}`,
		"missing parent": `{"unrelated": "value"}`,
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "data.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("writing fixture: %v", err)
			}
			var m Manager
			_, err := m.Plan(context.Background(), &managers.Input{
				Dir:     dir,
				Paths:   []string{"data.json"},
				Refs:    testRefs,
				Options: setOptions(setEntry{Path: "items.#.version", From: "opentofu"}),
			})
			if err == nil {
				t.Fatal("expected an error for a complex path that matches nothing")
			}
		})
	}
}

// TestJSONDuplicatePathRejected guards against a regression to the field-test
// finding that two set entries targeting the identical path silently
// last-wins, discarding the first write with no error or warning.
func TestJSONDuplicatePathRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"version": "1.0.0"}`), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:   dir,
		Paths: []string{"data.json"},
		Refs:  testRefs,
		Options: setOptions(
			setEntry{Path: "version", From: "opentofu"},
			setEntry{Path: "version", From: "cli"},
		),
	})
	if err == nil {
		t.Fatal("expected an error for duplicate set entries targeting the same path")
	}
}

// TestJSONNumericPathHintsAtQuoting guards against a regression to the
// field-test finding that an unquoted numeric YAML path value (parsed as an
// int, not a string) fails with a confusing mapstructure error that doesn't
// explain the fix; the wrapped error must carry a hint about quoting.
func TestJSONNumericPathHintsAtQuoting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`["1.0.0","2.0.0"]`), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	_, err := m.Plan(context.Background(), &managers.Input{
		Dir:   dir,
		Paths: []string{"data.json"},
		Refs:  testRefs,
		// Simulates an unquoted `path: 0` in YAML, which decodes as an int.
		Options: map[string]any{"set": []map[string]any{{"path": 0, "from": "opentofu"}}},
	})
	if err == nil {
		t.Fatal("expected an error for a non-string path value")
	}
	hints := errors.GetAllHints(err)
	found := false
	for _, hint := range hints {
		if strings.Contains(hint, "Quote numeric path segments") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error to hint at quoting the path, got hints %v for error: %v", hints, err)
	}
}
