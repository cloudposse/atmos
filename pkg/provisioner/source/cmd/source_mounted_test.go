package cmd_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfncmd "github.com/cloudposse/atmos/cmd/aws/cloudformation"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
)

// writeCollisionFixture writes the field-test layout: a hand-written component `x-handmade` whose
// directory is components/cloudformation/handmade, and an instance NAMED `handmade` that sources
// into components/cloudformation/src-nested through metadata.component.
func writeCollisionFixture(t *testing.T, rootDir, sourceDir string) {
	t.Helper()
	files := map[string]any{
		"atmos.yaml": map[string]any{
			"base_path":  rootDir,
			"components": map[string]any{"aws/cloudformation": map[string]any{"base_path": "components/cloudformation"}},
			"stacks":     map[string]any{"base_path": "stacks", "included_paths": []string{"**/*"}, "name_template": "{{ .vars.stage }}"},
		},
		filepath.Join("stacks", "dev.yaml"): map[string]any{
			"vars": map[string]any{"stage": "dev"},
			"components": map[string]any{"aws/cloudformation": map[string]any{
				"x-handmade": map[string]any{"metadata": map[string]any{"component": "handmade"}, "path": "template.yaml", "stack_name": "handmade"},
				"handmade": map[string]any{
					"metadata":   map[string]any{"component": "src-nested"},
					"source":     map[string]any{"uri": sourceDir},
					"path":       "template.yaml",
					"stack_name": "handmade-src",
				},
			}},
		},
	}
	for name, value := range files {
		path := filepath.Join(rootDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		content, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
}

// runMountedSource executes `aws cloudformation source <verb> handmade --stack dev ...` through the real command tree.
func runMountedSource(t *testing.T, rootDir string, args ...string) error {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", rootDir)
	t.Setenv("ATMOS_BASE_PATH", rootDir)
	t.Setenv("ATMOS_INTERACTIVE", "false")
	t.Setenv("ATMOS_DRY_RUN", "false")

	root := cfncmd.CloudFormationCmd
	verb := args[0]
	cmd, _, err := root.Find([]string{"source", verb})
	require.NoError(t, err)
	for _, name := range []string{"dry-run", "stack", "force"} {
		flag := cmd.Flag(name)
		require.NotNil(t, flag)
		previous, changed := flag.Value.String(), flag.Changed
		t.Cleanup(func() { require.NoError(t, flag.Value.Set(previous)); flag.Changed = changed })
	}
	t.Cleanup(func() { root.SetArgs(nil) })
	root.SetArgs(append([]string{"source"}, args...))
	return root.Execute()
}

// TestSourceCommands_ActOnTheDirectoryTheRuntimeUses reproduces the field-test collision end to end:
// pull and delete must target components/cloudformation/src-nested (metadata.component), never the
// instance-named components/cloudformation/handmade owned by another component.
func TestSourceCommands_ActOnTheDirectoryTheRuntimeUses(t *testing.T) {
	rootDir := t.TempDir()
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "template.yaml"), []byte("Resources: {}\n"), 0o600))
	writeCollisionFixture(t, rootDir, sourceDir)

	components := filepath.Join(rootDir, "components", "cloudformation")
	handmade := filepath.Join(components, "handmade")
	sourced := filepath.Join(components, "src-nested")
	require.NoError(t, os.MkdirAll(handmade, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(handmade, "template.yaml"), []byte("hand written\n"), 0o600))

	// Pull provisions into the metadata.component directory and records provenance there.
	require.NoError(t, runMountedSource(t, rootDir, "pull", "handmade", "--stack", "dev", "--force"))
	assert.FileExists(t, filepath.Join(sourced, "template.yaml"))
	assert.True(t, source.HasProvenance(sourced))
	handwritten, err := os.ReadFile(filepath.Join(handmade, "template.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "hand written\n", string(handwritten), "pull must not overwrite the instance-named hand-written directory")

	// Delete removes the provisioned copy and leaves the hand-written directory alone.
	require.NoError(t, runMountedSource(t, rootDir, "delete", "handmade", "--stack", "dev", "--force"))
	assert.NoDirExists(t, sourced)
	assert.FileExists(t, filepath.Join(handmade, "template.yaml"))
}

// TestSourceDelete_RefusesDirectoryOwnedByAnotherComponent covers the safety guard: even when the
// sourced instance resolves onto the hand-written component's directory, delete refuses.
func TestSourceDelete_RefusesDirectoryOwnedByAnotherComponent(t *testing.T) {
	rootDir := t.TempDir()
	sourceDir := t.TempDir()
	writeCollisionFixture(t, rootDir, sourceDir)

	// Point the sourced instance at the hand-written directory by giving it the same
	// metadata.component as x-handmade, and mark the directory as provisioned.
	stackPath := filepath.Join(rootDir, "stacks", "dev.yaml")
	content, err := os.ReadFile(stackPath)
	require.NoError(t, err)
	var stack map[string]any
	require.NoError(t, json.Unmarshal(content, &stack))
	instances := stack["components"].(map[string]any)["aws/cloudformation"].(map[string]any)
	instances["handmade"].(map[string]any)["metadata"] = map[string]any{"component": "handmade"}
	content, err = json.Marshal(stack)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stackPath, content, 0o600))

	handmade := filepath.Join(rootDir, "components", "cloudformation", "handmade")
	require.NoError(t, os.MkdirAll(handmade, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(handmade, "template.yaml"), []byte("hand written\n"), 0o600))
	require.NoError(t, source.WriteProvenance(handmade, &source.Provenance{Component: "handmade", Source: "x"}))

	err = runMountedSource(t, rootDir, "delete", "handmade", "--stack", "dev", "--force")

	require.ErrorIs(t, err, errUtils.ErrSourceDeleteRefused)
	assert.FileExists(t, filepath.Join(handmade, "template.yaml"))
}
