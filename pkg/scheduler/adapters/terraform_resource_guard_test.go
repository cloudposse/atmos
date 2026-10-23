package adapters

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformResourceGuardSerializesSharedDirectories(t *testing.T) {
	guard := NewTerraformResourceGuard(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{SubCommand: "deploy"})
	a := &dependency.Node{Component: "alias-a", Stack: "dev", Metadata: map[string]any{"component_info": map[string]any{"component_path": "/repo/shared"}}}
	b := &dependency.Node{Component: "alias-b", Stack: "prod", Metadata: map[string]any{"component_info": map[string]any{"component_path": "/repo/shared"}}}
	var active, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n := a
			if i%2 == 1 {
				n = b
			}
			unlock := guard(n)
			defer unlock()
			count := active.Add(1)
			for p := peak.Load(); count > p && !peak.CompareAndSwap(p, count); p = peak.Load() {
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 1, peak.Load())
	// Distinct directories are independent, even while the shared path is held.
	unlock := guard(a)
	defer unlock()
	done := make(chan struct{})
	go func() {
		n := &dependency.Node{Component: "other", Metadata: map[string]any{"component_info": map[string]any{"component_path": "/repo/other"}}}
		release := guard(n)
		release()
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		require.Fail(t, "distinct source directories must run concurrently")
	}
}
