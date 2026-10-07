package git

import (
	"os"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

var yamlRootCache rootTagCache

type rootTagCache struct {
	mu    sync.Mutex
	roots map[string]string
}

// ResetRootTagCache bounds !repo-root and !git.root caching to one invocation.
// Direct GetRoot calls continue to discover the repository on every call.
func ResetRootTagCache() {
	defer perf.Track(nil, "git.ResetRootTagCache")()
	yamlRootCache.clear()
}

func (c *rootTagCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roots = nil
}

func (c *rootTagCache) resolve(directory string, lookup func() (string, error)) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if root, ok := c.roots[directory]; ok {
		return root, nil
	}
	root, err := lookup()
	if err != nil {
		// Failed lookups and tag-specific fallbacks must not hide a repository
		// created later in the invocation, or a different fallback value.
		return "", err
	}
	if c.roots == nil {
		c.roots = make(map[string]string)
	}
	c.roots[directory] = root
	return root, nil
}

func cachedRootTag() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return yamlRootCache.resolve(directory, GetRoot)
}
