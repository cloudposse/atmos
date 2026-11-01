package terraform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProviderRecoveryInfoScopedEnvironment(t *testing.T) {
	root := t.TempDir()
	cache := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".terraform.lock.hcl"), []byte("provider \"registry.terraform.io/hashicorp/null\" {\n version = \"3.2.1\"\n}\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(cache, "registry.terraform.io", "hashicorp", "null", "3.2.1", runtime.GOOS+"_"+runtime.GOARCH), 0o755))
	cfg := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{ComponentEnvList: []string{"TF_PLUGIN_CACHE_DIR=" + cache}}
	copy, err := providerRecoveryInfo(cfg, info, root)
	require.NoError(t, err)
	assert.Equal(t, []string{"TF_PLUGIN_CACHE_DIR=" + cache, "TF_PLUGIN_CACHE_DIR=" + filepath.Join(root, ".terraform", "atmos-provider-recovery")}, copy.ComponentEnvList)
	copy.ComponentEnvList[0] = "result-change"
	assert.Equal(t, "TF_PLUGIN_CACHE_DIR="+cache, info.ComponentEnvList[0])
	info.ComponentEnvList[0] = "source-change"
	assert.Equal(t, "result-change", copy.ComponentEnvList[0])
	info.DryRun = true
	dry, err := providerRecoveryInfo(cfg, info, root)
	require.NoError(t, err)
	assert.Same(t, info, dry)
}
