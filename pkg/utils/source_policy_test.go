package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIncludePolicyConfinement(t *testing.T) {
	root := t.TempDir()
	policyDir := filepath.Join(root, "policy.d")
	require.NoError(t, os.Mkdir(policyDir, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", policyDir)
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	manifest := fmt.Sprintf("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: paths}\nspec:\n  policies:\n    guard:\n      on: source\n      allowed_roots: [%q]\n      action: deny\n      rule: input.operation == 'include' && !input.within_allowed_roots\n", allowed)
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "policy.yaml"), []byte(manifest), 0o600))
	inside := filepath.Join(allowed, "safe.txt")
	outside := filepath.Join(root, "secret.txt")
	require.NoError(t, os.WriteFile(inside, []byte("public"), 0o600))
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	cfg := &schema.AtmosConfiguration{BasePath: root}
	node := &yaml.Node{}
	require.NoError(t, ProcessIncludeRawTag(cfg, node, inside, filepath.Join(root, "stack.yaml")))
	require.Equal(t, "public", node.Value)
	require.Error(t, ProcessIncludeRawTag(cfg, &yaml.Node{}, outside, filepath.Join(root, "stack.yaml")))
}
