package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitWorkflowRejectsBrokenRoutes(t *testing.T) {
	t.Parallel()
	const linux = ".github/workflows/ci-test-linux.yml"
	const policy = ".github/actions/ci-pipeline/policy.json"
	cases := []struct {
		name, file, before, after string
	}{
		{"malformed YAML", linux, "name: CI Test Linux", "name: ["},
		{"wrong count", linux, "TEST_SHARD_COUNT: '10'", "TEST_SHARD_COUNT: '9'"},
		{"duplicate shard", linux, "          - 3\n", "          - 2\n"},
		{"missing shard", linux, "          - 3\n", ""},
		{"renamed shard", linux, "name: Acceptance Tests (", "name: Renamed ("},
		{"missing registry route", linux, "go test ./tests -run '^TestTerraformRegistryCache$'", "echo skipped"},
		{"renamed registry", linux, "name: Terraform registry cache test (", "name: Registry ("},
		{"wrong producer", policy, `"parent": "ci-build-linux.yml"`, `"parent": "ci-build-macos.yml"`},
		{"missing required check", policy, `"name": "Acceptance Tests (linux)"`, `"name": "unprotected"`},
		{"missing reported shard", policy, `"Acceptance Tests (linux, shard 3/10)",`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, file := range []string{linux, ".github/workflows/ci-test-macos.yml", ".github/workflows/ci-test-windows.yml", policy} {
				data, err := os.ReadFile(filepath.Join(realRepoRoot(t), file))
				if err != nil {
					t.Fatal(err)
				}
				if file == tc.file {
					if !strings.Contains(string(data), tc.before) {
						t.Fatalf("fixture does not contain %q", tc.before)
					}
					data = []byte(strings.Replace(string(data), tc.before, tc.after, 1))
				}
				path := filepath.Join(root, file)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := verifyWorkflow(root, 10); err == nil {
				t.Fatal("broken workflow passed verification")
			}
		})
	}
}
