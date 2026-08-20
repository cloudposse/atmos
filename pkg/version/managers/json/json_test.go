package json

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

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
