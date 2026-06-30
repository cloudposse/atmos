package asciicast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolvePathUsesXDGCacheBase(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", temp)

	started := time.Date(2026, 6, 30, 14, 22, 33, 0, time.UTC)
	path, err := ResolvePath(Options{Command: []string{"terraform", "plan", "vpc"}}, started)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := filepath.Join(temp, "atmos", "casts", "2026", "06", "30")
	if !strings.HasPrefix(path, wantPrefix) {
		t.Fatalf("path %q does not start with %q", path, wantPrefix)
	}
	if !strings.Contains(filepath.Base(path), "142233-terraform-plan-vpc-") {
		t.Fatalf("path %q does not include command slug", path)
	}
}

func TestStartFailsWhenExplicitPathExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	if err := os.WriteFile(path, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Start(Options{Path: path, Explicit: true})
	if err == nil {
		t.Fatal("expected explicit path collision error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecorderOmitsInputUnlessEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(Options{Path: path, Explicit: true, Now: func() time.Time {
		return time.Unix(10, 0)
	}})
	if err != nil {
		t.Fatal(err)
	}
	rec.Record("i", "secret input")
	rec.Record("o", "visible output")
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "secret input") {
		t.Fatal("input was recorded even though input recording is disabled")
	}
	if !strings.Contains(string(content), "visible output") {
		t.Fatal("output event was not recorded")
	}
}
