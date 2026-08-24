package managers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	versionmanager "github.com/cloudposse/atmos/pkg/version/manager"
)

type fakeManager struct {
	name     string
	defaults []string
	changes  []FileChange
	err      error
	inputs   []*Input
}

func (m *fakeManager) Name() string { return m.name }

func (m *fakeManager) DefaultPaths() []string { return m.defaults }

func (m *fakeManager) Plan(_ context.Context, in *Input) ([]FileChange, error) {
	m.inputs = append(m.inputs, in)
	return m.changes, m.err
}

func resetRegistryForTest(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	previous := registry
	registry = map[string]Manager{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = previous
		registryMu.Unlock()
	})
}

func testConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version: schema.Version{
			Track:    "prod",
			LockFile: "versions.lock.yaml",
			Tracks: map[string]schema.VersionTrack{
				"prod": {
					Dependencies: map[string]schema.VersionEntry{
						"opentofu": {
							Ecosystem: "toolchain",
							Package:   "opentofu",
							Desired:   "1.10.0",
							Update:    schema.VersionUpdatePolicy{Pin: "digest"},
						},
					},
				},
			},
		},
	}
	lock := &versionmanager.LockFile{
		Tracks: map[string]map[string]versionmanager.LockEntry{
			"prod": {
				"opentofu": {Version: "1.10.0", Digest: "sha256:abc"},
			},
		},
	}
	if err := versionmanager.SaveLock(cfg, lock); err != nil {
		t.Fatalf("SaveLock: %v", err)
	}
	return cfg
}

func TestRegistryOperations(t *testing.T) {
	resetRegistryForTest(t)

	Register(&fakeManager{name: "zeta"})
	Register(&fakeManager{name: "alpha"})
	Register(&fakeManager{name: "middle"})

	got, ok := Get("alpha")
	if !ok {
		t.Fatal("expected alpha manager to be registered")
	}
	if got.Name() != "alpha" {
		t.Fatalf("Get(alpha) returned %q", got.Name())
	}
	if _, ok := Get("missing"); ok {
		t.Fatal("expected missing manager lookup to fail")
	}

	all := All()
	var names []string
	for _, manager := range all {
		names = append(names, manager.Name())
	}
	want := []string{"alpha", "middle", "zeta"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("All() names = %v, want %v", names, want)
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("expected duplicate registration to panic")
		}
		if !strings.Contains(recovered.(error).Error(), ErrDuplicateManager.Error()) {
			t.Fatalf("duplicate panic = %v, want %v", recovered, ErrDuplicateManager)
		}
	}()
	Register(&fakeManager{name: "alpha"})
}

func TestPlanUsesConfiguredRulesOnlyFilterAndRefs(t *testing.T) {
	resetRegistryForTest(t)
	cfg := testConfig(t)

	alpha := &fakeManager{
		name: "alpha",
		changes: []FileChange{{
			Path: filepath.Join(cfg.BasePath, "managed.txt"),
			New:  []byte("updated"),
		}},
	}
	beta := &fakeManager{name: "beta"}
	Register(alpha)
	Register(beta)

	cfg.Version.Files = []schema.VersionFileRule{
		{Manager: "alpha", Paths: []string{"managed.txt"}, Options: map[string]any{"mode": "test"}},
		{Manager: "beta", Paths: []string{"ignored.txt"}},
	}
	changes, err := Plan(context.Background(), &RunOptions{
		Config: cfg,
		Track:  "prod",
		Dir:    cfg.BasePath,
		Only:   []string{"alpha"},
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 1 || changes[0].Manager != "alpha" {
		t.Fatalf("Plan changes = %#v", changes)
	}
	if len(alpha.inputs) != 1 {
		t.Fatalf("alpha Plan calls = %d, want 1", len(alpha.inputs))
	}
	if len(beta.inputs) != 0 {
		t.Fatalf("beta Plan calls = %d, want 0", len(beta.inputs))
	}
	in := alpha.inputs[0]
	if in.Track != "prod" || in.Dir != cfg.BasePath {
		t.Fatalf("input track/dir = %q/%q", in.Track, in.Dir)
	}
	if got := in.Entries["opentofu"].Package; got != "opentofu" {
		t.Fatalf("input entry package = %q", got)
	}
	if got := in.Refs["opentofu"].String(); got != "sha256:abc" {
		t.Fatalf("input ref = %q", got)
	}
	if got := in.Options["mode"]; got != "test" {
		t.Fatalf("input options[mode] = %#v", got)
	}
}

func TestPlanDefaultsAndErrors(t *testing.T) {
	t.Run("default rules use managers with default paths", func(t *testing.T) {
		resetRegistryForTest(t)
		cfg := testConfig(t)
		withDefault := &fakeManager{name: "with-default", defaults: []string{"*.txt"}}
		withoutDefault := &fakeManager{name: "without-default"}
		Register(withDefault)
		Register(withoutDefault)

		_, err := Plan(context.Background(), &RunOptions{Config: cfg})
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		if len(withDefault.inputs) != 1 {
			t.Fatalf("with-default calls = %d, want 1", len(withDefault.inputs))
		}
		if len(withoutDefault.inputs) != 0 {
			t.Fatalf("without-default calls = %d, want 0", len(withoutDefault.inputs))
		}
	})

	t.Run("unknown manager", func(t *testing.T) {
		resetRegistryForTest(t)
		cfg := testConfig(t)
		cfg.Version.Files = []schema.VersionFileRule{{Manager: "ghost"}}

		_, err := Plan(context.Background(), &RunOptions{Config: cfg})
		if !errors.Is(err, ErrUnknownManager) {
			t.Fatalf("Plan error = %v, want %v", err, ErrUnknownManager)
		}
	})

	t.Run("manager error is wrapped with manager name", func(t *testing.T) {
		resetRegistryForTest(t)
		cfg := testConfig(t)
		sentinel := errors.New("boom")
		Register(&fakeManager{name: "broken", err: sentinel})
		cfg.Version.Files = []schema.VersionFileRule{{Manager: "broken"}}

		_, err := Plan(context.Background(), &RunOptions{Config: cfg})
		if !errors.Is(err, sentinel) {
			t.Fatalf("Plan error = %v, want sentinel", err)
		}
		if !strings.Contains(err.Error(), "broken:") {
			t.Fatalf("Plan error = %v, want manager name", err)
		}
	})
}

func TestApplyCheckAndExpandPaths(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	newFile := filepath.Join(dir, "nested", "created.txt")
	changes := []PlannedChange{
		{Manager: "alpha", FileChange: FileChange{Path: existing, New: []byte("new")}},
		{Manager: "beta", FileChange: FileChange{Path: newFile, New: []byte("created")}},
	}

	if err := Check(nil); err != nil {
		t.Fatalf("Check(nil) returned error: %v", err)
	}
	err := Check(changes)
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("Check error = %v, want %v", err, ErrDrift)
	}
	if !strings.Contains(err.Error(), existing+" (alpha)") || !strings.Contains(err.Error(), newFile+" (beta)") {
		t.Fatalf("Check error does not list drift paths: %v", err)
	}

	if err := Apply(changes); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("existing content = %q", got)
	}
	got, err = os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(got) != "created" {
		t.Fatalf("new content = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(existing)
		if err != nil {
			t.Fatalf("stat existing: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("existing mode = %v, want 0600", info.Mode().Perm())
		}
	}

	nestedMatch := filepath.Join(dir, "nested", "match.txt")
	if err := os.WriteFile(nestedMatch, []byte("match"), 0o644); err != nil {
		t.Fatalf("write nested match: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested", "directory"), 0o755); err != nil {
		t.Fatalf("mkdir nested directory: %v", err)
	}
	files, err := ExpandPaths(dir, []string{"*.txt", "nested/*", "nested/*", "missing*.txt"})
	if err != nil {
		t.Fatalf("ExpandPaths returned error: %v", err)
	}
	want := []string{existing, newFile, nestedMatch}
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ExpandPaths files = %v, want %v", files, want)
	}
}
