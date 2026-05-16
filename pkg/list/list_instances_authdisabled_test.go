package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authDisabledFakeProcessor is a hand-written test double that satisfies both
// e.StacksProcessor and the unexported authDisabledStacksProcessor interface
// declared in list_instances.go. We can't use the gomock-generated
// MockStacksProcessor because it only models the StacksProcessor interface —
// the auth-disabled method is only on DefaultStacksProcessor, not on the
// public interface (by design: it's an optional capability discovered via
// type assertion).
type authDisabledFakeProcessor struct {
	// Captured args from whichever method was invoked.
	executeDescribeStacksCalled        bool
	executeDescribeStacksAuthDisabled  bool
	capturedProcessTemplates           bool
	capturedProcessYamlFunctions       bool
	capturedAuthDisabledMethodCallFlag bool // The authDisabled arg passed to the auth-disabled method.
	returnStacks                       map[string]any
	returnErr                          error
}

func (f *authDisabledFakeProcessor) ExecuteDescribeStacks(
	_ *schema.AtmosConfiguration,
	_ string,
	_ []string,
	_ []string,
	_ []string,
	_ bool,
	processTemplates bool,
	processYamlFunctions bool,
	_ bool,
	_ []string,
	_ auth.AuthManager,
) (map[string]any, error) {
	f.executeDescribeStacksCalled = true
	f.capturedProcessTemplates = processTemplates
	f.capturedProcessYamlFunctions = processYamlFunctions
	if f.returnStacks == nil {
		return map[string]any{}, f.returnErr
	}
	return f.returnStacks, f.returnErr
}

func (f *authDisabledFakeProcessor) ExecuteDescribeStacksWithAuthDisabled(
	_ *schema.AtmosConfiguration,
	_ string,
	_ []string,
	_ []string,
	_ []string,
	_ bool,
	processTemplates bool,
	processYamlFunctions bool,
	_ bool,
	_ []string,
	_ auth.AuthManager,
	authDisabled bool,
) (map[string]any, error) {
	f.executeDescribeStacksAuthDisabled = true
	f.capturedProcessTemplates = processTemplates
	f.capturedProcessYamlFunctions = processYamlFunctions
	f.capturedAuthDisabledMethodCallFlag = authDisabled
	if f.returnStacks == nil {
		return map[string]any{}, f.returnErr
	}
	return f.returnStacks, f.returnErr
}

// nonAuthDisabledFakeProcessor satisfies only e.StacksProcessor — used to prove
// executeDescribeStacksForInstances falls back to ExecuteDescribeStacks when
// the processor doesn't implement the optional auth-disabled interface, even
// when authDisabled=true was requested by the caller.
type nonAuthDisabledFakeProcessor struct {
	called bool
}

func (f *nonAuthDisabledFakeProcessor) ExecuteDescribeStacks(
	_ *schema.AtmosConfiguration,
	_ string,
	_ []string,
	_ []string,
	_ []string,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	_ []string,
	_ auth.AuthManager,
) (map[string]any, error) {
	f.called = true
	return map[string]any{}, nil
}

// TestExecuteDescribeStacksForInstances_AuthDisabledDispatchesToAuthDisabledMethod
// is the regression guard for the optional-interface dispatch in
// executeDescribeStacksForInstances: when authDisabled=true AND the processor
// implements authDisabledStacksProcessor, the auth-disabled method must be
// invoked (so per-component auth is short-circuited downstream).
func TestExecuteDescribeStacksForInstances_AuthDisabledDispatchesToAuthDisabledMethod(t *testing.T) {
	fake := &authDisabledFakeProcessor{}
	atmosConfig := &schema.AtmosConfiguration{}

	result, err := executeDescribeStacksForInstances(
		atmosConfig,
		fake,
		nil,  // authManager
		true, // processTemplates
		true, // processYamlFunctions
		true, // authDisabled — the bit under test.
	)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, fake.executeDescribeStacksAuthDisabled,
		"authDisabled=true with a capable processor MUST call ExecuteDescribeStacksWithAuthDisabled")
	assert.False(t, fake.executeDescribeStacksCalled,
		"the regular ExecuteDescribeStacks must NOT be called when the auth-disabled path is taken")
	assert.True(t, fake.capturedAuthDisabledMethodCallFlag,
		"the authDisabled flag must be forwarded into the auth-disabled method")
	assert.True(t, fake.capturedProcessTemplates)
	assert.True(t, fake.capturedProcessYamlFunctions)
}

// TestExecuteDescribeStacksForInstances_AuthDisabledFalseUsesRegularPath
// proves the regular ExecuteDescribeStacks is still chosen when authDisabled
// is false, even if the processor implements the optional interface. This
// matches the behavior list instances had before --identity=false existed.
func TestExecuteDescribeStacksForInstances_AuthDisabledFalseUsesRegularPath(t *testing.T) {
	fake := &authDisabledFakeProcessor{}
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := executeDescribeStacksForInstances(
		atmosConfig,
		fake,
		nil,
		true,
		false,
		false, // authDisabled=false — regular path expected.
	)
	require.NoError(t, err)
	assert.True(t, fake.executeDescribeStacksCalled,
		"authDisabled=false MUST call ExecuteDescribeStacks")
	assert.False(t, fake.executeDescribeStacksAuthDisabled,
		"the auth-disabled method must NOT be called when authDisabled=false")
}

// TestExecuteDescribeStacksForInstances_FallsBackWhenInterfaceNotImplemented
// is the safety-net guard: even with authDisabled=true, a processor that
// doesn't implement authDisabledStacksProcessor must fall back to the
// regular ExecuteDescribeStacks rather than panic on a failed type assertion.
func TestExecuteDescribeStacksForInstances_FallsBackWhenInterfaceNotImplemented(t *testing.T) {
	fake := &nonAuthDisabledFakeProcessor{}
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := executeDescribeStacksForInstances(
		atmosConfig,
		fake,
		nil,
		false,
		false,
		true, // authDisabled=true but processor doesn't implement the optional interface.
	)
	require.NoError(t, err)
	assert.True(t, fake.called,
		"a processor that doesn't implement authDisabledStacksProcessor must still be called via the regular path")
}

// TestProcessInstancesWithDepsAuthDisabled_PropagatesAuthDisabledFlag verifies
// the test seam: processInstancesWithDepsAuthDisabled must reach the
// auth-disabled method on a capable processor and surface no error. The
// previous TestProcessInstancesWithDeps_* coverage hit the authDisabled=false
// path implicitly; this test pins authDisabled=true → auth-disabled method.
func TestProcessInstancesWithDepsAuthDisabled_PropagatesAuthDisabledFlag(t *testing.T) {
	fake := &authDisabledFakeProcessor{
		returnStacks: map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"metadata": map[string]any{"type": "real"},
							"vars":     map[string]any{"region": "us-east-1"},
						},
					},
				},
			},
		},
	}
	atmosConfig := &schema.AtmosConfiguration{}

	instances, err := processInstancesWithDepsAuthDisabled(
		atmosConfig,
		fake,
		nil,
		true,
		false,
		true, // authDisabled
	)
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Equal(t, "vpc", instances[0].Component)
	assert.Equal(t, "dev", instances[0].Stack)
	assert.True(t, fake.executeDescribeStacksAuthDisabled,
		"authDisabled=true must route through the auth-disabled method")
}

// TestProcessInstancesWithAuthDisabled_ConstructsDefaultProcessor is a smoke
// test for the production wrapper: it must construct a DefaultStacksProcessor
// and not error when there are no stacks. The function is small but it's the
// path the CLI actually takes, so a smoke test catches a future signature
// drift between wrapper and helper.
//
// Anchoring BasePath/Stacks.BasePath/Components.Terraform.BasePath to a
// per-test temp directory makes the assertion CWD-independent. With an empty
// AtmosConfiguration the underlying FindStacksMap resolves CWD-relative empty
// paths, which is the kind of ambient-state dependency CLAUDE.md flags. This
// mirrors the convention in TestDefaultStacksProcessor_ExecuteDescribeStacks.
func TestProcessInstancesWithAuthDisabled_ConstructsDefaultProcessor(t *testing.T) {
	testDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: testDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	instances, err := processInstancesWithAuthDisabled(
		atmosConfig,
		nil,
		false,
		false,
		true,
	)
	// With no stacks configured, ExecuteDescribeStacksWithAuthDisabled returns
	// an empty map; the upstream collector then returns an empty slice. The
	// important assertion is that the wrapper plumbs through cleanly.
	require.NoError(t, err)
	assert.Empty(t, instances)
}
