package config

import (
	"os"
	"path/filepath"
	"strings"
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

// TestLoadConfig_ConcurrentCalls_IsolatedResults verifies that concurrent
// LoadConfig calls loading DIFFERENT config files never mix up which files
// they tracked for case-sensitive key extraction. A mutex alone (guarding
// reads/writes to a single shared tracker) is not enough to prevent this: it
// stops the "concurrent map writes" panic, but one call's reset()/track()
// calls can still interleave with another's, letting a call snapshot a
// different call's tracked files -- silently corrupting env/auth.identities
// case restoration with the wrong config's keys. Each concurrent LoadConfig
// call must get its own tracker (see mergedFilesRegistry, global_viper.go).
func TestLoadConfig_ConcurrentCalls_IsolatedResults(t *testing.T) {
	dirA := t.TempDir()
	atmosYAMLA := "base_path: ./\n" +
		"env:\n" +
		"  UNIQUE_KEY_A: \"a\"\n"
	pathA := filepath.Join(dirA, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(pathA, []byte(atmosYAMLA), 0o644))

	dirB := t.TempDir()
	atmosYAMLB := "base_path: ./\n" +
		"env:\n" +
		"  UNIQUE_KEY_B: \"b\"\n"
	pathB := filepath.Join(dirB, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(pathB, []byte(atmosYAMLB), 0o644))

	type result struct {
		wantKey string
		envCase map[string]string
		err     error
	}

	const perDir = 10
	results := make(chan result, perDir*2)
	var wg sync.WaitGroup

	load := func(path, wantKey string) {
		defer wg.Done()
		atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{
			AtmosConfigFilesFromArg: []string{path},
		})
		if err != nil {
			results <- result{err: err}
			return
		}
		var envCase map[string]string
		if atmosConfig.CaseMaps != nil {
			envCase = atmosConfig.CaseMaps.Get(envKey)
		}
		results <- result{wantKey: wantKey, envCase: envCase}
	}

	for i := 0; i < perDir; i++ {
		wg.Add(2)
		go load(pathA, "UNIQUE_KEY_A")
		go load(pathB, "UNIQUE_KEY_B")
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out - possible deadlock in concurrent LoadConfig calls")
	}
	close(results)

	otherKey := map[string]string{"UNIQUE_KEY_A": "unique_key_b", "UNIQUE_KEY_B": "unique_key_a"}
	count := 0
	for r := range results {
		require.NoError(t, r.err)
		count++

		lowerWant := strings.ToLower(r.wantKey)
		assert.Equal(t, r.wantKey, r.envCase[lowerWant],
			"result meant for %s must preserve its own config file's case, not another concurrent call's", r.wantKey)
		assert.NotContains(t, r.envCase, otherKey[r.wantKey],
			"result meant for %s must not contain the other concurrent call's tracked key", r.wantKey)
	}
	assert.Equal(t, perDir*2, count)
}
