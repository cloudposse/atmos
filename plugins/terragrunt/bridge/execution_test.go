package atmosbridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

func TestEnvironmentLookupUsesInvocationPath(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"PATH": filepath.Dir(executable), "PROBE": "original"}
	scoped := Environment(env, nil, nil)
	got, err := scoped.Exec.LookPath(filepath.Base(executable))
	if err != nil || got != executable {
		t.Fatalf("lookup = %q, %v", got, err)
	}
	other := Environment(map[string]string{"PATH": t.TempDir()}, nil, nil)
	if _, err := other.Exec.LookPath(filepath.Base(executable)); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("unexpected fallback: %v", err)
	}
	if err := other.Exec.Command(context.Background(), filepath.Base(executable)).Run(); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("unexpected spawn: %v", err)
	}
	if _, err := other.Exec.LookPath(executable); err != nil {
		t.Fatal(err)
	}
	// Simultaneous providers cannot replace the process environment or each other's PATH.
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if _, err := scoped.Exec.LookPath(filepath.Base(executable)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if env["PROBE"] != "original" {
		t.Fatal("environment mutated")
	}
}

// accessLog records every path a filesystem is asked to open or stat.
type accessLog struct {
	afero.Fs
	mu    sync.Mutex
	paths []string
}

func (a *accessLog) record(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.paths = append(a.paths, name)
}

func (a *accessLog) Open(name string) (afero.File, error) {
	a.record(name)
	return a.Fs.Open(name)
}

func (a *accessLog) Stat(name string) (os.FileInfo, error) {
	a.record(name)
	return a.Fs.Stat(name)
}

func (a *accessLog) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	a.record(name)
	return a.Fs.(afero.Lstater).LstatIfPossible(name)
}

// The unit directory is used as the root of copy-rule globs. A glob metacharacter in its path
// must neither widen the scan to the unit's parent nor change which files the overlay carries.
func TestUnitOverlayStaysInsideUnitWithGlobMetacharacters(t *testing.T) {
	base := t.TempDir()
	unit := filepath.Join(base, "unit{with-brace")
	sibling := filepath.Join(base, "sibling")
	for _, dir := range []string{unit, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(unit, "main.tf"):       "# unit",
		filepath.Join(unit, ".tflint.hcl"):   "# lint",
		filepath.Join(sibling, "other.tf"):   "# sibling",
		filepath.Join(sibling, "nested.tfx"): "# sibling",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &accessLog{Fs: afero.NewOsFs()}
	env := Environment(map[string]string{}, nil, nil).WithFS(recorder)
	files, _, err := UnitOverlay(log.New(), env, unit, &config.TerraformConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if string(files["main.tf"]) != "# unit" || string(files[".tflint.hcl"]) != "# lint" || len(files) != 2 {
		t.Fatalf("overlay files = %v", files)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, path := range recorder.paths {
		if rel, err := filepath.Rel(unit, path); err != nil || !filepath.IsLocal(rel) {
			t.Errorf("overlay read %q outside the unit directory", path)
		}
	}
}

// An unreadable sibling of a unit whose path holds a glob metacharacter must not fail the overlay.
func TestUnitOverlayIgnoresUnreadableSiblingDirectories(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced here")
	}
	base := t.TempDir()
	unit := filepath.Join(base, "unit{with-brace")
	locked := filepath.Join(base, "locked")
	for _, dir := range []string{unit, locked} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(unit, "main.tf"), []byte("# unit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	files, _, err := UnitOverlay(log.New(), Environment(map[string]string{}, nil, nil), unit, &config.TerraformConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if string(files["main.tf"]) != "# unit" {
		t.Fatalf("overlay files = %v", files)
	}
}

func TestWindowsLookupDoesNotAppendAmbientSuffix(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"helper.EXE", "helper.COM.EXE"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("executable"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATHEXT", ".EXE")
	scoped := Environment(map[string]string{"PATH": directory, "PATHEXT": ".COM;.EXE"}, nil, nil).Exec.(scopedExec)
	got, err := scoped.lookPath("helper", "windows")
	if err != nil || got != filepath.Join(directory, "helper.EXE") {
		t.Fatalf("lookup = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "helper.COM"), []byte("first suffix"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err = scoped.lookPath("helper", "windows")
	if err != nil || got != filepath.Join(directory, "helper.COM") {
		t.Fatalf("suffix order = %q, %v", got, err)
	}
}
