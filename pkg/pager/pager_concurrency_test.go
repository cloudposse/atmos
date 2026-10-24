package pager

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/viperguard"
)

// Pager construction can overlap theme rendering, which binds environment
// variables on the same global Viper instance even when the keys differ.
func TestNewConcurrentThemeBinding(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetDefault("settings.terminal.speed", 123.5)

	const iterations = 500
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			assert.NoError(t, viperguard.BindEnv("settings.terminal.theme", "ATMOS_THEME", "THEME"))
		}
	}()

	close(start)
	for range iterations {
		assert.Equal(t, 123.5, New().(*pageCreator).terminalSpeed)
		assert.Equal(t, 123.5, NewWithAtmosConfig(true).(*pageCreator).terminalSpeed)
		assert.Equal(t, 123.5, NewWithViewport(true, 20, 80).(*pageCreator).terminalSpeed)
	}
	wg.Wait()
}
