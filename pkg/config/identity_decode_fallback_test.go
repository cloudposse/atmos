package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFixAuthIdentitiesDecodeFallback checks that a failed reconstruction retains
// an already decoded identity and warns, while successful decoding stays authoritative.
func TestFixAuthIdentitiesDecodeFallback(t *testing.T) {
	for _, tc := range []struct {
		name        string
		identity    string
		defaultYAML string
		override    bool
		wantWarning bool
		wantEntry   bool
		wantDefault bool
	}{
		{name: "preserves decoded fallback", identity: "reader", defaultYAML: "invalid", override: true, wantWarning: true, wantEntry: true},
		{name: "warns without a fallback", identity: "broken.reader", defaultYAML: "invalid", wantWarning: true},
		{name: "successful reconstruction wins", identity: "reader", defaultYAML: "true", override: true, wantEntry: true, wantDefault: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalLogger := log.Default()
			var warnings bytes.Buffer
			logger := log.New()
			logger.SetOutput(&warnings)
			logger.SetLevel(log.WarnLevel)
			logger.SetReportTimestamp(false)
			log.SetDefault(logger)
			t.Cleanup(func() { log.SetDefault(originalLogger) })

			content := "auth:\n  identities:\n    " + tc.identity + ":\n      kind: mock\n      default: " + tc.defaultYAML + "\n      principal:\n        name: Reader\n    good.identity:\n      kind: mock\n"
			path := writeIdentityImportFixture(t, t.TempDir(), "atmos.yaml", content)
			v := viper.New()
			v.SetConfigFile(path)
			require.NoError(t, v.ReadInConfig())
			if tc.override {
				// Simulate an effective Viper value that differs from the raw YAML.
				v.Set("auth.identities.reader.default", false)
			}
			var cfg schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
			existing := cfg.Auth.Identities[tc.identity]
			if tc.override {
				require.Equal(t, "mock", existing.Kind)
			}

			require.NoError(t, fixAuthIdentities(v, &cfg))
			assert.Equal(t, "mock", cfg.Auth.Identities["good.identity"].Kind)
			assert.NotContains(t, cfg.Auth.Identities, "good", "do not retain Viper's split dotted keys")
			actual, exists := cfg.Auth.Identities[tc.identity]
			assert.Equal(t, tc.wantEntry, exists)
			if tc.wantEntry {
				assert.Equal(t, tc.wantDefault, actual.Default)
				assert.Equal(t, "Reader", actual.Principal["name"])
			}
			if tc.override && tc.wantWarning {
				assert.Equal(t, existing, actual, "preserve the entire decoded entry")
			}
			if tc.wantWarning {
				assert.Contains(t, warnings.String(), "Failed to decode identity")
				assert.Contains(t, warnings.String(), tc.identity)
			} else {
				assert.Empty(t, warnings.String())
			}
		})
	}
}
