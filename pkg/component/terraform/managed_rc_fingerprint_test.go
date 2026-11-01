package terraform

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/terraform/rc"
)

func TestManagedRegistryRCFingerprintStable(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true, Location: t.TempDir()}
	componentPath := t.TempDir()
	var hashes, contents []string
	for range 2 {
		setup, err := tfcache.Start(t.Context(), config)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, setup.Close(context.Background())) })
		info := &schema.ConfigAndStacksInfo{TerraformCache: setup}
		require.NoError(t, configureTerraformRC(config, info))
		require.NotNil(t, info.RCCleanup)
		t.Cleanup(func() { require.NoError(t, info.RCCleanup()) })
		fp, err := autoinit.Compute(autoinit.InputsFromInfo(config, info, componentPath, ""))
		require.NoError(t, err)
		hashes = append(hashes, fp.Hash)
		for _, entry := range info.ComponentEnvList {
			if path, ok := strings.CutPrefix(entry, rc.EnvKeyTFCLIConfigFile+"="); ok {
				b, e := os.ReadFile(path)
				require.NoError(t, e)
				contents = append(contents, string(b))
			}
		}
	}
	require.Len(t, contents, 2)
	assert.NotEqual(t, contents[0], contents[1], "live proxies must use distinct ports")
	assert.Equal(t, hashes[0], hashes[1], "ephemeral Atmos-owned proxy ports must not invalidate init")
}

func TestManagedRegistryRCFingerprintTracksSemanticChanges(t *testing.T) {
	clearUserCLIConfigEnv(t)
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true, Location: t.TempDir()}
	config.Components.Terraform.RC = &schema.TerraformRCConfig{Enabled: true, Config: map[string]any{"host": map[string]any{"private.example.com": map[string]any{"services": map[string]any{"modules.v1": "https://127.0.0.1:1234/"}}}}}
	componentPath := t.TempDir()
	compute := func() string {
		t.Helper()
		setup, err := tfcache.Start(t.Context(), config)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, setup.Close(context.Background())) })
		info := &schema.ConfigAndStacksInfo{TerraformCache: setup}
		require.NoError(t, configureTerraformRC(config, info))
		t.Cleanup(func() { require.NoError(t, info.RCCleanup()) })
		fp, err := autoinit.Compute(autoinit.InputsFromInfo(config, info, componentPath, ""))
		require.NoError(t, err)
		return fp.Hash
	}
	original := compute()
	services := config.Components.Terraform.RC.Config["host"].(map[string]any)["private.example.com"].(map[string]any)["services"].(map[string]any)
	services["modules.v1"] = "https://127.0.0.1:5678/"
	changedUser := compute()
	assert.NotEqual(t, original, changedUser, "user localhost ports must remain significant")
	config.Components.Terraform.Cache.Location = t.TempDir()
	changedRoot := compute()
	assert.NotEqual(t, changedUser, changedRoot, "cache root/trust identity must remain significant")
	config.Components.Terraform.Cache.MetadataTTL = "2h"
	assert.NotEqual(t, changedRoot, compute(), "cache resolution policy must remain significant")
}
