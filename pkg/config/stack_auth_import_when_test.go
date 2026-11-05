package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadStackAuthDefaults_ConditionalImports(t *testing.T) {
	for _, tc := range []struct {
		name string
		when string
		ci   bool
		want bool
	}{
		{"ci skipped locally", "ci", false, false},
		{"ci active in CI", "ci", true, true},
		{"local active locally", "local", false, true},
		{"local skipped in CI", "local", true, false},
		{"always", "always", false, true},
		{"never", "never", false, false},
		{"CEL matches", `!cel env.ATMOS_TEST_AUTH_IMPORT_ENV == "prod"`, false, true},
		{"CEL does not match", `!cel env.ATMOS_TEST_AUTH_IMPORT_ENV == "dev"`, false, false},
		{"CEL missing variable", `!cel env.ATMOS_TEST_AUTH_IMPORT_MISSING == "prod"`, false, false},
		{"compound condition", "{all: [local, always]}", false, true},
		{"invalid condition", "sometimes", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(ci.SwapRegistryForTest())
			if tc.ci {
				ci.Register(github.NewProvider())
				t.Setenv("GITHUB_ACTIONS", "true")
			}
			t.Setenv("ATMOS_TEST_AUTH_IMPORT_ENV", "prod")
			t.Setenv("ATMOS_TEST_AUTH_IMPORT_MISSING", "")
			require.NoError(t, os.Unsetenv("ATMOS_TEST_AUTH_IMPORT_MISSING"))

			dir := t.TempDir()
			// Follow an unconditional import before reaching a conditional nested import.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("import:\n  - parent\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "parent.yaml"), []byte(fmt.Sprintf("import:\n  - path: identity\n    when: %s\n", tc.when)), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "identity.yaml"), []byte("auth:\n  identities:\n    pipeline:\n      default: true\n"), 0o600))
			config := &schema.AtmosConfiguration{
				StacksBaseAbsolutePath:    dir,
				IncludeStackAbsolutePaths: []string{filepath.Join(dir, "manifest.yaml")},
			}
			defaults, err := LoadStackAuthDefaults(config)
			require.NoError(t, err)
			if tc.want {
				assert.Equal(t, map[string]bool{"pipeline": true}, defaults)
			} else {
				assert.Empty(t, defaults)
			}
		})
	}
}

func TestLoadStackAuthDefaults_ConditionalIdentitySelection(t *testing.T) {
	for _, inCI := range []bool{false, true} {
		t.Run(fmt.Sprintf("ci=%t", inCI), func(t *testing.T) {
			t.Cleanup(ci.SwapRegistryForTest())
			if inCI {
				ci.Register(github.NewProvider())
				t.Setenv("GITHUB_ACTIONS", "true")
			}
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(`import:
  - path: local
    when: never
  - path: local
    when: local
  - path: pipeline
    when: ci
`), 0o600))
			for _, identity := range []string{"local", "pipeline"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, identity+".yaml"), []byte(fmt.Sprintf("auth:\n  identities:\n    %s:\n      default: true\n", identity)), 0o600))
			}
			config := &schema.AtmosConfiguration{
				StacksBaseAbsolutePath:    dir,
				IncludeStackAbsolutePaths: []string{filepath.Join(dir, "manifest.yaml")},
			}
			defaults, err := LoadStackAuthDefaults(config)
			require.NoError(t, err)
			want := "local"
			if inCI {
				want = "pipeline"
			}
			assert.Equal(t, map[string]bool{want: true}, defaults, "inactive imports must not cause conflicting defaults or mark files visited")
		})
	}
}
