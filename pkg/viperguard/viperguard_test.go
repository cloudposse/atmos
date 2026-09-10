package viperguard_test

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/viperguard"
)

// TestConcurrentBindEnvAndGet reproduces the exact shape of a data race caught
// by CI's race-detector job: pkg/ui/theme.getActiveThemeName calling
// viper.BindEnv concurrently with pkg/http.GetGitHubTokenFromEnv calling
// viper.GetString, both against the global viper singleton, from two
// goroutines spawned by the toolchain's concurrent batch installer. Run under
// `go test -race`, this fails if BindEnv/GetString/IsSet/Set/GetStringSlice
// ever go back to calling viper.* directly instead of through this package.
func TestConcurrentBindEnvAndGet(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_THEME", "dark")

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = viperguard.BindEnv("settings.terminal.theme", "ATMOS_THEME", "THEME")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = viperguard.GetString("github-token")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			viperguard.Set("some.unrelated.key", i)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = viperguard.IsSet("settings.terminal.theme")
			_ = viperguard.GetStringSlice("some.slice")
		}
	}()

	wg.Wait()

	assert.True(t, viperguard.IsSet("settings.terminal.theme"),
		"BindEnv must still have taken effect once every goroutine finished")
}

// TestGetBoolAndView covers the package's GetBool and View, plus every
// viperReaderAdapter method View hands to its callback (IsSet, GetBool,
// GetString, GetStringSlice) -- none of which TestConcurrentBindEnvAndGet
// exercises, since that test only covers the concurrency contract.
func TestGetBoolAndView(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viperguard.Set("some.bool.key", true)
	viperguard.Set("some.string.key", "hello")
	viperguard.Set("some.slice.key", []string{"a", "b"})

	assert.True(t, viperguard.GetBool("some.bool.key"))
	assert.False(t, viperguard.GetBool("unset.bool.key"))

	var viewedBool bool
	var viewedString string
	var viewedSlice []string
	var viewedSet bool
	viperguard.View(func(v viperguard.ViperReader) {
		viewedBool = v.GetBool("some.bool.key")
		viewedString = v.GetString("some.string.key")
		viewedSlice = v.GetStringSlice("some.slice.key")
		viewedSet = v.IsSet("some.slice.key")
	})

	assert.True(t, viewedBool)
	assert.Equal(t, "hello", viewedString)
	assert.Equal(t, []string{"a", "b"}, viewedSlice)
	assert.True(t, viewedSet)
	assert.False(t, viperguard.IsSet("unset.slice.key"))

	// GetStringSlice, both the package-level function and View's adapter,
	// must clone Viper's backing array rather than hand it out: mutating the
	// returned slice must never corrupt what a later read observes.
	viewedSlice[0] = "mutated"
	assert.Equal(t, []string{"a", "b"}, viperguard.GetStringSlice("some.slice.key"))
}
