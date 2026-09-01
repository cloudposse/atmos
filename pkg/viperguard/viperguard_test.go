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
