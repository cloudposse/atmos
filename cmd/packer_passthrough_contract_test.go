package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise Cobra and the actual subprocess boundary, rather than copying separator logic.
func TestNativeCommandSeparatorPreservesArgv(t *testing.T) {
	for _, family := range []string{"packer", "helmfile"} {
		t.Run(family, func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
			componentDir := filepath.Join(root, "components", family, "probe")
			require.NoError(t, os.MkdirAll(componentDir, 0o755))
			executable, err := os.Executable()
			require.NoError(t, err)
			config := fmt.Sprintf("base_path: %q\ncomponents:\n  %s:\n    base_path: components/%s\n    command: %q\nstacks:\n  base_path: stacks\n  included_paths: ['*.yaml']\n  name_template: '{{ .vars.stage }}'\n", filepath.ToSlash(root), family, family, filepath.ToSlash(executable))
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(config), 0o600))
			stack := fmt.Sprintf("vars: {stage: dev}\ncomponents:\n  %s:\n    probe: {}\n", family)
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(stack), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.pkr.hcl"), []byte("// fake engine fixture\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(componentDir, "helmfile.yaml"), []byte("releases: []\n"), 0o600))
			t.Chdir(root)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			capture := filepath.Join(root, "argv.json")
			t.Setenv("_ATMOS_TEST_DUMP_ARGS", capture)
			operation, native := "validate", []string{"-syntax-only", "-var", "label=probe probe", "-var", "name=probe"}
			if family == "helmfile" {
				operation, native = "template", []string{"--set", "label=probe probe", "--set", "name=probe"}
			}
			args := append([]string{family, operation, "probe", "-s", "dev", "--"}, native...)
			RootCmd.SetArgs(args)
			require.NoError(t, Execute())
			raw, err := os.ReadFile(capture)
			require.NoError(t, err, "the native process must actually execute")
			var actual []string
			require.NoError(t, json.Unmarshal(raw, &actual))
			// Extra Atmos-generated args may precede/follow native args. Find the entire
			// contiguous sequence to assert ordering, spacing, and duplicate values.
			found := false
			for i := 0; i+len(native) <= len(actual); i++ {
				match := true
				for j := range native {
					if actual[i+j] != native[j] {
						match = false
						break
					}
				}
				if match {
					found = true
					break
				}
			}
			require.True(t, found, "native argv %q missing from actual %q", native, actual)
			require.Contains(t, actual, operation)
			require.NotContains(t, actual, "--")
		})
	}
}
