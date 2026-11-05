package deferred

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTargetSectionConcurrentDescribesDoNotShareMutableMaps(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "false")
	var calls atomic.Int64
	//nolint:unparam // The describe callback requires an error result; this case tests concurrency.
	describe := func() (map[string]any, error) {
		calls.Add(1)
		return map[string]any{"vars": map[string]any{"value": "original"}}, nil
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			section, err := TargetSection(ac, "target", "dev", describe)
			if err != nil {
				t.Error(err)
				return
			}
			section["vars"].(map[string]any)["value"] = "caller mutation"
		})
	}
	wg.Wait()
	coldCalls := calls.Load()
	require.Positive(t, coldCalls)
	section, err := TargetSection(ac, "target", "dev", describe)
	require.NoError(t, err)
	require.Equal(t, "original", section["vars"].(map[string]any)["value"])
	require.Equal(t, coldCalls, calls.Load(), "warm references reuse a completed describe")
}

func TestTargetSectionIsolation(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "false")
	calls := 0
	source := map[string]any{"vars": map[string]any{"value": "original"}}
	//nolint:unparam // The describe callback requires an error result; this case tests isolation.
	describe := func() (map[string]any, error) {
		calls++
		return source, nil
	}
	load := func(config *schema.AtmosConfiguration) map[string]any {
		section, err := TargetSection(config, "target", "dev", describe)
		require.NoError(t, err)
		return section
	}
	first := load(ac)
	source["vars"].(map[string]any)["value"] = "source mutation"
	require.Equal(t, "original", first["vars"].(map[string]any)["value"])
	require.Equal(t, "original", load(ac)["vars"].(map[string]any)["value"])
	require.Equal(t, 1, calls)
	for _, mutate := range []func(*schema.AtmosConfiguration){
		func(c *schema.AtmosConfiguration) { c.BasePath = "other-root" },
		func(c *schema.AtmosConfiguration) { c.CliConfigPath = "other-config" },
		func(c *schema.AtmosConfiguration) {
			c.Auth.Identities = map[string]schema.Identity{"other": {Kind: "aws/user"}}
		},
		func(c *schema.AtmosConfiguration) { authdeferred.ConfigureAuth(c, "false") },
	} {
		nested := *ac
		mutate(&nested)
		before := calls
		load(&nested)
		load(&nested)
		require.Equal(t, before+1, calls, "different configuration and invocations must resolve independently")
	}
	for _, target := range []struct{ component, stack string }{{"other", "dev"}, {"target", "prod"}} {
		before := calls
		_, err := TargetSection(ac, target.component, target.stack, describe)
		require.NoError(t, err)
		require.Equal(t, before+1, calls)
	}
}

func TestTargetSectionRetriesMissingAndFailedDescribes(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "false")
	calls := 0
	describe := func() (map[string]any, error) {
		calls++
		switch calls {
		case 1:
			return nil, errUtils.ErrDescribeComponent
		case 2:
			return nil, nil
		case 3:
			return map[string]any{"value": degradation.AtmosComputedValue{}}, nil
		default:
			return map[string]any{"value": "resolved"}, nil
		}
	}
	_, err := TargetSection(ac, "target", "dev", describe)
	require.ErrorIs(t, err, errUtils.ErrDescribeComponent)
	section, err := TargetSection(ac, "target", "dev", describe)
	require.NoError(t, err)
	require.Nil(t, section)
	section, err = TargetSection(ac, "target", "dev", describe)
	require.NoError(t, err)
	require.Equal(t, degradation.AtmosComputedValue{}, section["value"])
	for range 2 {
		section, err = TargetSection(ac, "target", "dev", describe)
		require.NoError(t, err)
		require.Equal(t, "resolved", section["value"])
	}
	require.Equal(t, 4, calls)
}

func TestTargetSectionUncacheableConfigurations(t *testing.T) {
	unsupported := &schema.AtmosConfiguration{Auth: schema.AuthConfig{
		Identities: map[string]schema.Identity{"local": {Credentials: map[string]any{"unsupported": make(chan int)}}},
	}}
	authdeferred.ConfigureAuth(unsupported, "false")
	for _, ac := range []*schema.AtmosConfiguration{nil, {}, unsupported} {
		calls := 0
		for range 2 {
			section, err := TargetSection(ac, "target", "dev", func() (map[string]any, error) {
				calls++
				return map[string]any{"value": calls}, nil
			})
			require.NoError(t, err)
			require.Equal(t, calls, section["value"])
		}
		require.Equal(t, 2, calls, "unsupported configuration safely bypasses caching")
	}
}

func TestTargetSectionCopiesShareInvocationBeforeFirstLookup(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "false")
	first, second := *ac, *ac
	calls := 0
	for _, copy := range []*schema.AtmosConfiguration{&first, &second} {
		section, err := TargetSection(copy, "target", "dev", func() (map[string]any, error) {
			calls++
			return map[string]any{"value": "shared"}, nil
		})
		require.NoError(t, err)
		require.Equal(t, "shared", section["value"])
	}
	require.Equal(t, 1, calls)
}

func TestTargetSectionRecursiveMissesDoNotWaitOnEachOther(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	authdeferred.ConfigureAuth(ac, "false")
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan error, 2)
	for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
		go func() {
			_, err := TargetSection(ac, pair[0], "dev", func() (map[string]any, error) {
				started <- struct{}{}
				<-release
				return TargetSection(ac, pair[1], "dev", func() (map[string]any, error) {
					return map[string]any{"value": "resolved"}, nil
				})
			})
			done <- err
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent describes did not start")
		}
	}
	close(release)
	for range 2 {
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("recursive describes must not wait on another in-flight target")
		}
	}
}
