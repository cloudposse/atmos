package cloudformation

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run the real provisioner and generator in separate processes, sharing only the source.
func TestGenerationConcurrentProcesses(t *testing.T) {
	if root := os.Getenv("ATMOS_TEST_GENERATION_ROOT"); root != "" {
		stack := os.Getenv("ATMOS_TEST_GENERATION_STACK")
		for range 2 {
			config, info := generationFixture(t, root, stack)
			spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
			require.NoError(t, err)
			body, err := os.ReadFile(spec.TemplateAbsPath)
			require.NoError(t, err)
			require.Equal(t, string(body), spec.TemplateBody)
			require.Contains(t, spec.TemplateBody, "Value: '"+stack+"'")
			policy, err := os.ReadFile(filepath.Join(filepath.Dir(spec.TemplateAbsPath), "policy.json"))
			require.NoError(t, err)
			require.Equal(t, string(policy), spec.StackPolicyBody)
			result, err := json.Marshal(map[string]string{"path": spec.TemplateAbsPath, "body": spec.TemplateBody})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, stack+".json"), result, 0o600))
		}
		return
	}
	root := t.TempDir()
	source := filepath.Join(root, "components", "cloudformation", "shared")
	require.NoError(t, os.MkdirAll(source, 0o755))
	canary := filepath.Join(source, "keep.txt")
	require.NoError(t, os.WriteFile(canary, []byte("unchanged"), 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	cmds := make([]*exec.Cmd, 0, 2)
	for _, stack := range []string{"dev", "sandbox"} {
		cmd := exec.Command(executable, "-test.run=^TestGenerationConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "ATMOS_TEST_GENERATION_ROOT="+root, "ATMOS_TEST_GENERATION_STACK="+stack)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		require.NoError(t, cmd.Start())
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		require.NoError(t, cmd.Wait())
	}
	paths := map[string]bool{}
	for _, stack := range []string{"dev", "sandbox"} {
		result, err := os.ReadFile(filepath.Join(root, stack+".json"))
		require.NoError(t, err)
		var decoded map[string]string
		require.NoError(t, json.Unmarshal(result, &decoded))
		assert.False(t, paths[decoded["path"]], "each stack needs its own workdir")
		paths[decoded["path"]] = true
		assert.Contains(t, decoded["body"], "Value: '"+stack+"'")
	}
	entries, err := os.ReadDir(source)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	data, err := os.ReadFile(canary)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(data))
}

// TestGenerateForLocalTemplateOperations checks generation dispatch for operations that consume local templates.
func TestGenerateForLocalTemplateOperations(t *testing.T) {
	for _, op := range []Operation{OperationRender, OperationValidate, OperationDiff, OperationApply, OperationChangesetCreate, OperationStackSetCreate, OperationStackSetUpdate, OperationFmt} {
		t.Run(string(op), func(t *testing.T) {
			config, info := generationFixture(t, t.TempDir(), "generated")
			spec, err := resolveSpecAndTemplate(t.Context(), config, info, op)
			require.NoError(t, err)
			assert.Contains(t, spec.TemplateBody, "Value: 'generated'")
			policy, err := os.ReadFile(filepath.Join(filepath.Dir(spec.TemplateAbsPath), "policy.json"))
			require.NoError(t, err)
			assert.Contains(t, string(policy), "Allow")
			if op != OperationFmt && op != OperationChangesetCreate {
				assert.Equal(t, string(policy), spec.StackPolicyBody)
			}
		})
	}
}
