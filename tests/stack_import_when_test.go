package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
)

// TestStackImportWhen verifies that a stack manifest's `import` entries honor their `when`
// condition: an import whose condition does not match is never read, so its variables are absent.
func TestStackImportWhen(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-stack-import-when")

	describeVars := func(t *testing.T) map[string]any {
		t.Helper()
		e.ClearFindStacksMapCache()
		t.Cleanup(e.ClearFindStacksMapCache)

		section, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component: "probe", Stack: "import-when",
			ProcessTemplates: true, ProcessYamlFunctions: true,
		})
		require.NoError(t, err)
		vars, ok := section["vars"].(map[string]any)
		require.True(t, ok, "vars section must be a map")
		return vars
	}

	t.Run("off CI keeps always and local imports and skips never, ci, and a non-matching expression", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())
		t.Setenv("ATMOS_TEST_IMPORT_ENV", "dev")

		vars := describeVars(t)
		assert.Equal(t, true, vars["from_always"])
		assert.Equal(t, true, vars["from_local_only"])
		assert.NotContains(t, vars, "from_never")
		assert.NotContains(t, vars, "from_ci_only")
		assert.NotContains(t, vars, "from_env_prod")
	})

	// Negative path of the case above: the same manifest in CI flips the ci and local imports.
	t.Run("in CI keeps always and ci imports and skips local", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())
		ci.Register(github.NewProvider())
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ATMOS_TEST_IMPORT_ENV", "dev")

		vars := describeVars(t)
		assert.Equal(t, true, vars["from_always"])
		assert.Equal(t, true, vars["from_ci_only"])
		assert.NotContains(t, vars, "from_local_only")
		assert.NotContains(t, vars, "from_never")
	})

	t.Run("a matching CEL expression over the environment keeps the import", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())
		t.Setenv("ATMOS_TEST_IMPORT_ENV", "prod")

		assert.Equal(t, true, describeVars(t)["from_env_prod"])
	})
}
