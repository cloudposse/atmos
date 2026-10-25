package dependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestLoadToolVersionsDependencies_ConcurrentProjects(t *testing.T) {
	original := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(original) })
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 8 {
		root := t.TempDir()
		version := fmt.Sprintf("1.0.%d", i)
		require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte("terraform "+version+"\n"), 0o600))
		config := &schema.AtmosConfiguration{BasePathAbsolute: root}
		wg.Go(func() {
			<-start
			for range 20 {
				got, err := LoadToolVersionsDependencies(config)
				if assert.NoError(t, err) {
					assert.Equal(t, map[string]string{"terraform": version}, got)
				}
			}
		})
	}
	close(start)
	wg.Wait()
	assert.Same(t, original, toolchain.GetAtmosConfig(), "reading project defaults must not replace global toolchain configuration")
}
