package atmosbridge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
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
