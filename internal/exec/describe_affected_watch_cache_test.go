package exec

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// BenchmarkConfigurationWatchCacheHitDuringBuild measures cached lookup latency
// while another exclusion set is built from a large shared change set.
func BenchmarkConfigurationWatchCacheHitDuringBuild(b *testing.B) {
	root := b.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	files := make([]string, 9000)
	for i := range files {
		files[i] = filepath.Join(root, "modules", fmt.Sprintf("unit-%d/main.tf", i))
	}
	settings := func(pattern string) map[string]any {
		return map[string]any{"configuration": map[string]any{"watch_excludes": []any{pattern}}}
	}
	var worstTotal time.Duration
	for i := 0; i < b.N; i++ {
		index := newChangedFilesIndex(config, files, root)
		if _, err := configurationWatchIndex(config, settings("**/.terragrunt-cache/**"), index); err != nil {
			b.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := configurationWatchIndex(config, settings("**/*.md"), index); done <- err }()
		var worst time.Duration
	loop:
		for {
			select {
			case err := <-done:
				if err != nil {
					b.Fatal(err)
				}
				break loop
			default:
			}
			start := time.Now()
			if _, err := configurationWatchIndex(config, settings("**/.terragrunt-cache/**"), index); err != nil {
				b.Fatal(err)
			}
			if elapsed := time.Since(start); elapsed > worst {
				worst = elapsed
			}
		}
		worstTotal += worst
	}
	b.ReportMetric(float64(worstTotal.Nanoseconds())/float64(b.N), "max-hit-ns/build")
}

// TestConfigurationWatchIndexConcurrentViews checks publication to concurrent
// callers, independent keys, and preservation of the original change set.
func TestConfigurationWatchIndexConcurrentViews(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	files := make([]string, 9000)
	for i := range files {
		files[i] = filepath.Join("modules", fmt.Sprintf("unit-%d/main.tf", i))
	}
	files = append(files, "README.md")
	index := newChangedFilesIndex(config, files, root)
	type result struct {
		key  int
		view *changedFilesIndex
		err  error
	}
	results := make(chan result, 32)
	start := make(chan struct{})
	for i := 0; i < cap(results); i++ {
		go func(key int) {
			<-start
			patterns := []string{"**/*.tf", "**/*.md"}
			settings := map[string]any{"configuration": map[string]any{"watch_excludes": []any{patterns[key]}}}
			view, err := configurationWatchIndex(config, settings, index)
			results <- result{key, view, err}
		}(i % 2)
	}
	close(start)
	views := [2]*changedFilesIndex{}
	for i := 0; i < cap(results); i++ {
		got := <-results
		require.NoError(t, got.err)
		if views[got.key] == nil {
			views[got.key] = got.view
		} else {
			require.Same(t, views[got.key], got.view)
		}
	}
	require.NotSame(t, views[0], views[1])
	require.Equal(t, []string{filepath.Join(root, "README.md")}, views[0].getAllFiles())
	require.Len(t, views[1].getAllFiles(), 9000)
	require.Len(t, index.getAllFiles(), 9001)
}

// TestConfigurationWatchIndexBuildsAreIsolated verifies that an in-flight key
// does not prevent cached lookups or a different key from being built.
func TestConfigurationWatchIndexBuildsAreIsolated(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, BasePath: root}
	index := newChangedFilesIndex(config, []string{"main.tf", "README.md"}, root)
	settings := func(pattern string) map[string]any {
		return map[string]any{"configuration": map[string]any{"watch_excludes": []any{pattern}}}
	}
	cached, err := configurationWatchIndex(config, settings("**/*.md"), index)
	require.NoError(t, err)
	key, err := json.Marshal([]string{root, "**/*.tf"})
	require.NoError(t, err)
	entered, release := make(chan struct{}), make(chan struct{})
	blocked := index.watchBuilds.DoChan(string(key), func() (any, error) {
		close(entered)
		<-release
		return cached, nil
	})
	defer func() { close(release); <-blocked }()
	<-entered
	type lookup struct {
		sameView bool
		err      error
	}
	done := make(chan lookup, 1)
	go func() {
		hit, err := configurationWatchIndex(config, settings("**/*.md"), index)
		if err != nil {
			done <- lookup{err: err}
			return
		}
		_, err = configurationWatchIndex(config, settings("**/*.hcl"), index)
		done <- lookup{sameView: hit == cached, err: err}
	}()
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.True(t, result.sameView, "the cached view must be returned unchanged")
	case <-time.After(5 * time.Second):
		t.Fatal("an unrelated cache build blocked other keys")
	}
}
