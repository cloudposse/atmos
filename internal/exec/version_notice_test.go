package exec

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestVersionExecuteUsesInjectedNotice(t *testing.T) {
	t.Parallel()
	var notified string
	v := versionExec{
		atmosConfig:                &schema.AtmosConfiguration{Version: schema.Version{Check: schema.VersionCheck{Enabled: true}}},
		printStyledText:            func(string) error { return nil },
		printMessage:               func(string) {},
		getLatestGitHubRepoRelease: func() (string, error) { return "v9999.0.0", nil },
		loadCacheConfig:            func() (cfg.CacheConfig, error) { return cfg.CacheConfig{}, nil },
		shouldCheckForUpdates:      func(int64, string) bool { return true },
		printMessageToUpgradeToAtmosLatestRelease: func(version string) { notified = version },
	}
	require.NoError(t, v.Execute(false, ""))
	assert.Equal(t, "9999.0.0", notified)
}

func TestVersionStructuredOutputDoesNotNotify(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			streams := &vendorModelTestStreams{stdout: &stdout, stderr: &stderr}
			ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
			require.NoError(t, err)
			data.InitWriter(ioCtx)
			ui.InitFormatter(ioCtx)
			t.Cleanup(func() {
				data.InitWriter(iolib.GetContext())
				ui.Reset()
			})
			v := versionExec{
				atmosConfig:                               &schema.AtmosConfiguration{},
				getLatestGitHubRepoRelease:                func() (string, error) { return "v9999.0.0", nil },
				printMessageToUpgradeToAtmosLatestRelease: func(string) { t.Fatal("structured output must not notify") },
			}
			require.NoError(t, v.Execute(true, format))
			assert.Empty(t, stderr.String())
			assert.NotContains(t, stdout.String(), "Run:")
			assert.Contains(t, stdout.String(), "update_version")
			assert.Contains(t, stdout.String(), "9999.0.0")
			if format == "json" {
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded))
				assert.Len(t, decoded, 5, "version schema must remain unchanged")
			}
		})
	}
}
