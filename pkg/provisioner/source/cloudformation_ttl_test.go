package source

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCloudFormationRemoteSourceTTL exercises actual downloads, cache hits, and refreshes.
func TestCloudFormationRemoteSourceTTL(t *testing.T) {
	for _, useWorkdir := range []bool{false, true} {
		for _, tc := range []struct {
			name      string
			globalTTL string
			localTTL  string
			age       time.Duration
			refresh   bool
		}{
			{name: "fresh global TTL", globalTTL: "1h", age: time.Minute},
			{name: "expired global TTL", globalTTL: "1h", age: 2 * time.Hour, refresh: true},
			{name: "global zero TTL", globalTTL: "0s", refresh: true},
			{name: "component overrides global", globalTTL: "0s", localTTL: "24h", age: time.Hour},
			{name: "component zero overrides global", globalTTL: "24h", localTTL: "0s", refresh: true},
			{name: "no TTL reuses source", age: 48 * time.Hour},
		} {
			name := "shared/" + tc.name
			if useWorkdir {
				name = "workdir/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				var downloads atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodGet {
						downloads.Add(1)
					}
					_, _ = io.WriteString(w, "Resources: {}\n")
				}))
				t.Cleanup(server.Close)
				base := t.TempDir()
				config := &schema.AtmosConfiguration{
					BasePath: base,
					Components: schema.Components{CloudFormation: schema.AwsCloudFormation{
						BasePath: "components/cloudformation",
						Source:   &schema.SourceSettings{TTL: tc.globalTTL},
					}},
				}
				newComponent := func() map[string]any {
					source := map[string]any{"uri": server.URL + "/template.yaml"}
					if tc.localTTL != "" {
						source["ttl"] = tc.localTTL
					}
					return map[string]any{
						"component": "vpc", "atmos_stack": "dev", "source": source,
						"provision": map[string]any{"workdir": map[string]any{"enabled": useWorkdir}},
					}
				}
				ctx := workdir.WithOutputSuppressed(t.Context())
				section := newComponent()
				err := AutoProvisionSource(ctx, config, cfg.CloudFormationComponentType, section, nil, provisioner.OutputWriters{})
				require.NoError(t, err)
				firstDownloads := downloads.Load()
				require.Positive(t, firstDownloads)
				// Even a zero TTL must not refresh twice within one invocation.
				require.NoError(t, AutoProvisionSource(ctx, config, cfg.CloudFormationComponentType, section, nil, provisioner.OutputWriters{}))
				assert.Equal(t, firstDownloads, downloads.Load())

				target, err := ResolveTarget(config, cfg.CloudFormationComponentType, "vpc", section)
				require.NoError(t, err)
				cached := filepath.Join(target.Dir, "template.yaml")
				require.NoError(t, os.WriteFile(cached, []byte("cached template\n"), 0o600))
				if useWorkdir {
					metadata, readErr := workdir.ReadMetadata(target.Dir)
					require.NoError(t, readErr)
					metadata.UpdatedAt = time.Now().Add(-tc.age)
					require.NoError(t, workdir.WriteMetadata(target.Dir, metadata))
				} else {
					require.NoError(t, WriteProvenance(target.Dir, &Provenance{
						Component: "vpc", Source: server.URL + "/template.yaml",
						ProvisionedAt: time.Now().Add(-tc.age),
					}))
				}

				err = AutoProvisionSource(ctx, config, cfg.CloudFormationComponentType, newComponent(), nil, provisioner.OutputWriters{})
				require.NoError(t, err)
				body, err := os.ReadFile(cached)
				require.NoError(t, err)
				if tc.refresh {
					assert.Greater(t, downloads.Load(), firstDownloads)
					assert.Equal(t, "Resources: {}\n", string(body))
				} else {
					assert.Equal(t, firstDownloads, downloads.Load())
					assert.Equal(t, "cached template\n", string(body))
				}
			})
		}
	}
}

// TestSharedSourceTTLRequiresProvenance protects authored and legacy component directories.
func TestSharedSourceTTLRequiresProvenance(t *testing.T) {
	for _, marker := range []string{"", "invalid json", `{}`} {
		t.Run(marker, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("authored"), 0o600))
			if marker != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, workdir.AtmosDir), 0o755))
				require.NoError(t, os.WriteFile(provenancePath(dir), []byte(marker), 0o600))
			}
			refresh, _ := needsProvisioning(dir, &schema.VendorComponentSource{Uri: "https://example.com/template.yaml", TTL: "0s"}, false)
			assert.False(t, refresh)
		})
	}
}

// TestSharedLocalSourceTTLDoesNotReplaceFiles keeps local directories outside remote expiry.
func TestSharedLocalSourceTTLDoesNotReplaceFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("authored"), 0o600))
	require.NoError(t, WriteProvenance(dir, &Provenance{ProvisionedAt: time.Now().Add(-time.Hour)}))
	refresh, _ := needsProvisioning(dir, &schema.VendorComponentSource{Uri: "./local", TTL: "0s"}, false)
	assert.False(t, refresh)
}
