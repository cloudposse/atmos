package vendoring

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/filelock"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

func TestSetComponentVersionLocksImportedDeclaration(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "vendor.yaml")
	imported := filepath.Join(dir, "common.yaml")
	require.NoError(t, os.WriteFile(root, []byte("spec:\n  imports: [common.yaml]\n"), 0o644))
	original := "spec:\n  sources:\n    - component: vpc\n      version: '1.0.0' # pin\n"
	require.NoError(t, os.WriteFile(imported, []byte(original), 0o644))
	locked := make(chan struct{})
	release := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- filelock.New(imported+".lock").WithExclusive(context.Background(), func() error { close(locked); <-release; return nil })
	}()
	<-locked
	released := false
	defer func() {
		if !released {
			close(release)
		}
		require.NoError(t, <-held)
	}()
	done := make(chan error, 1)
	go func() { done <- SetComponentVersion(root, "vpc", "2.0.0") }()
	select {
	case err := <-done:
		t.Fatalf("setter bypassed declaring file lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	// Reordering while holding the lock must be observed after it is acquired.
	reordered := "spec:\n  sources:\n    - component: other\n      version: '9.0.0'\n    - component: vpc\n      version: '1.0.0' # pin\n"
	require.NoError(t, os.WriteFile(imported, []byte(reordered), 0o644))
	close(release)
	released = true
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("setter did not resume after lock release")
	}
	value, err := atmosyaml.GetFile(imported, "spec.sources[1].version")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", value)
	value, err = atmosyaml.GetFile(imported, "spec.sources[0].version")
	require.NoError(t, err)
	assert.Equal(t, "9.0.0", value)
	content, err := os.ReadFile(imported)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# pin")
	assert.NoFileExists(t, root+".lock", "lock must follow the declaration, not the entrypoint")
}
