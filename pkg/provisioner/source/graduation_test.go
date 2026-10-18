package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Exercise real source staging, cache reuse, and refresh with restrictive policy.
func TestStableSourceProvisioning(t *testing.T) {
	for _, mode := range []string{"disable", "error"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			sourceDir := filepath.Join(base, "source")
			require.NoError(t, os.MkdirAll(sourceDir, 0o755))
			sourceFile := filepath.Join(sourceDir, "main.tf")
			require.NoError(t, os.WriteFile(sourceFile, []byte("# original\n"), 0o644))
			config := &schema.AtmosConfiguration{
				BasePath: base,
				Settings: schema.AtmosSettings{Experimental: mode},
			}
			invoke := func(ttl string) string {
				t.Helper()
				component := map[string]any{
					"component": "app", "atmos_stack": "dev",
					"source":    map[string]any{"uri": sourceDir, "ttl": ttl},
					"provision": map[string]any{"workdir": map[string]any{"enabled": true}},
				}
				err := AutoProvisionSource(t.Context(), config, "terraform", component, nil, provisioner.OutputWriters{})
				require.NoError(t, err)
				target, ok := component[workdir.WorkdirPathKey].(string)
				require.True(t, ok)
				contents, err := os.ReadFile(filepath.Join(target, "main.tf"))
				require.NoError(t, err)
				return string(contents)
			}
			assert.Equal(t, "# original\n", invoke("1h"))
			require.NoError(t, os.WriteFile(sourceFile, []byte("# refreshed\n"), 0o644))
			assert.Equal(t, "# original\n", invoke("1h"), "fresh cache is reused")
			assert.Equal(t, "# refreshed\n", invoke("0s"), "expired source is re-fetched")
		})
	}
}
