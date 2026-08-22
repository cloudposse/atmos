package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfig_ConcurrentCalls reproduces the "fatal error: concurrent map
// writes" panic reported when the DAG scheduler (pkg/scheduler) runs with
// --max-concurrency > 1: multiple worker goroutines each call
// InitCliConfig/LoadConfig for their own graph node, and LoadConfig used to
// write profiles.base_path and vendor.update.*/vendor.ci.* into the
// process-wide global Viper singleton with no synchronization. The
// spf13/viper package has no internal locking, so concurrent Set/Get calls
// on that singleton panicked.
//
// Run with `go test -race`: without -race this test can pass even on the
// unfixed code, since the panic depends on unlucky goroutine interleaving.
func TestLoadConfig_ConcurrentCalls(t *testing.T) {
	tmpDir := t.TempDir()
	atmosYAML := "" +
		"base_path: ./\n" +
		"profiles:\n" +
		"  base_path: ./profiles\n" +
		"vendor:\n" +
		"  update:\n" +
		"    execution:\n" +
		"      mode: worktree\n" +
		"    groups:\n" +
		"      platform:\n" +
		"        include: [\"terraform/vpc\"]\n" +
		"  ci:\n" +
		"    pull_request:\n" +
		"      provider: github\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName), []byte(atmosYAML), 0o644))

	t.Chdir(tmpDir)
	viper.Reset()
	t.Cleanup(viper.Reset)

	const goroutines = 20
	done := make(chan struct{})
	timeout := time.After(30 * time.Second)

	go func() {
		var wg sync.WaitGroup
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-timeout:
		t.Fatal("timed out - possible deadlock in concurrent LoadConfig calls")
	}
}
