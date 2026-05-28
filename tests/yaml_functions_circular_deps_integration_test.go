package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
)

// TestYAMLFunctionsCrossComponentCycle is a regression test for issue #2457.
//
// Before the fix in PR for #2457, two components referencing each other via
// !terraform.state caused infinite recursion through
// ProcessCustomYamlTags → !terraform.state → GetTerraformState →
// ExecuteDescribeComponent → ProcessStacks → ProcessCustomYamlTags (fresh
// context). The cycle detector lived behind a per-call ResolutionContext that
// was reset on every ProcessCustomYamlTags re-entry, so it could never see
// the outer walk's in-progress components. The result was a goroutine stack
// overflow, not a clean error.
//
// The fix makes ProcessCustomYamlTags reuse the goroutine-local
// ResolutionContext so cycle detection survives across nested walks. This
// test exercises the full ExecuteDescribeComponent path on a fixture with a
// direct A↔B cycle and asserts ErrCircularDependency comes back instead.
//
// Note: this test does not require any real Terraform state — the cycle is
// detected during the YAML-function resolution pass, before any backend read
// is attempted.
func TestYAMLFunctionsCrossComponentCycle(t *testing.T) {
	t.Chdir(filepath.Join(".", "fixtures", "scenarios", "yaml-functions-circular-deps"))

	e.ClearResolutionContext()
	t.Cleanup(e.ClearResolutionContext)

	_, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            "component-a",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})

	require.Error(t, err, "cross-component !terraform.state cycle must produce an error")
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency,
		"expected ErrCircularDependency; got: %v", err,
	)
	// The depth safety net (ErrYamlFuncMaxResolutionDepth) is defense-in-depth.
	// If it fires here, the proper cycle detector regressed and the visited
	// map is being wiped between nested ProcessCustomYamlTags entries again.
	assert.NotErrorIs(t, err, errUtils.ErrYamlFuncMaxResolutionDepth,
		"depth safeguard should not be needed when cycle detection works; the visited map is being wiped between nested walks again",
	)
}
