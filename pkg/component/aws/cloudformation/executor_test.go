package cloudformation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it. Mirrors captureStderr in events_test.go — the
// package's I/O layer resolves os.Stdout dynamically at write time (see
// testmain_test.go), so a simple swap observes data.Write/data.Writeln output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestEventsFor(t *testing.T) {
	tests := []struct {
		operation  Operation
		wantBefore hooks.HookEvent
		wantAfter  hooks.HookEvent
	}{
		{OperationDiff, hooks.BeforeAwsCloudFormationDiff, hooks.AfterAwsCloudFormationDiff},
		{OperationApply, hooks.BeforeAwsCloudFormationApply, hooks.AfterAwsCloudFormationApply},
		{OperationDelete, hooks.BeforeAwsCloudFormationDelete, hooks.AfterAwsCloudFormationDelete},
		{OperationRender, hooks.HookEvent(""), hooks.HookEvent("")},
	}
	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			before, after := eventsFor(tt.operation)
			assert.Equal(t, tt.wantBefore, before)
			assert.Equal(t, tt.wantAfter, after)
		})
	}
}

func TestDeleteOptionsFromFlags(t *testing.T) {
	opts := deleteOptionsFromFlags(map[string]any{
		"retain-resources":               []string{"MyBucket", "MyQueue"},
		"disable-termination-protection": true,
	})
	assert.Equal(t, []string{"MyBucket", "MyQueue"}, opts.RetainResources)
	assert.True(t, opts.DisableTerminationProtection)

	empty := deleteOptionsFromFlags(map[string]any{})
	assert.Empty(t, empty.RetainResources)
	assert.False(t, empty.DisableTerminationProtection)
}

func TestRunDiff(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
		Changes: []cfntypes.Change{
			{Type: cfntypes.ChangeTypeResource},
		},
	}, nil)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	summary, err := runDiff(context.Background(), client, spec, map[string]any{})
	require.NoError(t, err)
	assert.False(t, summary["no_op"].(bool))
	assert.Len(t, summary["changes"].([]cfntypes.Change), 1)
}

func TestRunDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	summary, err := runDelete(context.Background(), client, map[string]any{}, spec, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, string(cfntypes.StackStatusDeleteComplete), summary["final_status"])
}

func TestRunDelete_FailedStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteFailed}},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	_, err := runDelete(context.Background(), client, map[string]any{}, spec, map[string]any{})
	require.Error(t, err)
}

func TestRunOutput(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	outputKey := "VpcId"
	outputVal := "vpc-123"
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{{OutputKey: &outputKey, OutputValue: &outputVal}},
		}},
	}, nil)

	summary, err := runOutput(context.Background(), client, "vpc", map[string]any{"format": "json"}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-123"}, summary["outputs"])
}

// renderDiffSummary must report a no-op changeset without listing any
// per-resource lines.
func TestRenderDiffSummary_NoOp(t *testing.T) {
	out := captureStdout(t, func() {
		renderDiffSummary("vpc", &changeSetResult{NoOp: true})
	})
	assert.Contains(t, out, "vpc: no changes (changeset would be a no-op)")
}

// renderDiffSummary must list one line per resource change, including the
// replacement annotation only when CloudFormation reports a replacement.
func TestRenderDiffSummary_ListsResourceChanges(t *testing.T) {
	resourceType := "AWS::S3::Bucket"
	logicalID := "MyBucket"
	otherType := "AWS::IAM::Role"
	otherID := "MyRole"

	result := &changeSetResult{
		Changes: []cfntypes.Change{
			{
				ResourceChange: &cfntypes.ResourceChange{
					Action:            cfntypes.ChangeActionModify,
					ResourceType:      &resourceType,
					LogicalResourceId: &logicalID,
					Replacement:       cfntypes.ReplacementTrue,
				},
			},
			{
				ResourceChange: &cfntypes.ResourceChange{
					Action:            cfntypes.ChangeActionAdd,
					ResourceType:      &otherType,
					LogicalResourceId: &otherID,
				},
			},
			// A Change with a nil ResourceChange (e.g. a Hook invocation change)
			// must be skipped rather than panicking.
			{ResourceChange: nil},
		},
	}

	out := captureStdout(t, func() {
		renderDiffSummary("vpc", result)
	})
	// The summary count is len(result.Changes) — including the nil-ResourceChange
	// entry that renderDiffSummary's loop skips when printing per-resource lines.
	assert.Contains(t, out, "vpc: 3 resource change(s)")
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, "(replacement: True)")
	assert.Contains(t, out, "MyRole")
	assert.NotContains(t, out, "MyRole (replacement", "an Add with no Replacement must not print a replacement annotation")
}

// runApply's happy path: deliver, no stack policy configured, then render the
// end-of-deploy Outputs summary.
func TestRunApply_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	outputKey := "VpcId"
	outputVal := "vpc-123"
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status: cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
		}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{
				Outputs: []cfntypes.Output{{OutputKey: &outputKey, OutputValue: &outputVal}},
			}},
		}, nil),
	)

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		Flags:       map[string]any{"format": "json"},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	summary, err := runApply(octx, client, spec, map[string]any{"stack_name": "vpc"})
	require.NoError(t, err)
	assert.False(t, summary["no_op"].(bool))
	assert.Equal(t, map[string]any{"VpcId": "vpc-123"}, summary["outputs"])
}

// runApply must apply the stack policy after a successful deploy when
// spec.StackPolicyBody is set.
func TestRunApply_SetsStackPolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status: cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
		}, nil),
		client.EXPECT().SetStackPolicy(gomock.Any(), gomock.Any()).Return(&cloudformation.SetStackPolicyOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
	)

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		Flags:       map[string]any{},
	}
	spec := &stackSpec{
		StackName:       "vpc",
		TemplateBody:    "AWSTemplateFormatVersion: '2010-09-09'",
		StackPolicyBody: `{"Statement": []}`,
	}

	_, err := runApply(octx, client, spec, map[string]any{})
	require.NoError(t, err)
}

// runApply must propagate a deliverApply error without attempting to set a
// stack policy or describe outputs.
func TestRunApply_DeliverError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl) // no expectations: a target-selection error precedes any client call.

	octx := &opContext{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}},
		Flags:       map[string]any{targetKey: "nonexistent"},
	}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}

	_, err := runApply(octx, client, spec, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNotFound)
}

// stubProvisionAndResolveComponentPath overrides the provisionAndResolveComponentPath
// seam for a single test, bypassing JIT source provisioning entirely and
// returning a fixed directory. Auto-restores on cleanup.
func stubProvisionAndResolveComponentPath(t *testing.T, dir string, err error) {
	t.Helper()
	original := provisionAndResolveComponentPath
	provisionAndResolveComponentPath = func(_ context.Context, _ provisioner.OutputWriters, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string) (string, bool, error) {
		return dir, false, err
	}
	t.Cleanup(func() { provisionAndResolveComponentPath = original })
}

// resolveSpecAndTemplate must skip template/stack-policy loading entirely for
// a delete operation (delete needs no template).
func TestResolveSpecAndTemplate_DeleteSkipsTemplateLoad(t *testing.T) {
	stubProvisionAndResolveComponentPath(t, t.TempDir(), nil)

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{"stack_name": "vpc", "template": "template.yaml"},
	}
	spec, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationDelete)
	require.NoError(t, err)
	assert.Empty(t, spec.TemplateBody, "delete must never load the template body")
}

// resolveSpecAndTemplate's happy path for a non-delete operation: template
// body loaded, NoEcho values registered, stack policy loaded.
func TestResolveSpecAndTemplate_LoadsTemplateAndPolicy(t *testing.T) {
	tempDir := t.TempDir()
	templateBody := "AWSTemplateFormatVersion: '2010-09-09'\nParameters:\n  DbPassword:\n    Type: String\n    NoEcho: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "template.yaml"), []byte(templateBody), 0o644))
	policyBody := `{"Statement": []}`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "policy.json"), []byte(policyBody), 0o644))
	stubProvisionAndResolveComponentPath(t, tempDir, nil)

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"stack_name":   "vpc",
			"template":     "template.yaml",
			"stack_policy": map[string]any{"file": "policy.json"},
			"parameters":   map[string]any{"DbPassword": "supersecret"},
		},
	}
	spec, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationApply)
	require.NoError(t, err)
	assert.Equal(t, templateBody, spec.TemplateBody)
	assert.Equal(t, policyBody, spec.StackPolicyBody)
}

// resolveSpecAndTemplate must propagate a JIT provisioning failure.
func TestResolveSpecAndTemplate_ProvisionError(t *testing.T) {
	sentinel := errors.New("clone failed")
	stubProvisionAndResolveComponentPath(t, "", sentinel)

	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "vpc", "template": "template.yaml"}}
	_, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// resolveSpecAndTemplate must propagate a buildStackSpec validation error
// (e.g. a missing stack_name).
func TestResolveSpecAndTemplate_BuildSpecError(t *testing.T) {
	stubProvisionAndResolveComponentPath(t, t.TempDir(), nil)

	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"template": "template.yaml"}}
	_, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationStackName)
}

// resolveSpecAndTemplate must propagate a missing-template-file error with
// the resolved path.
func TestResolveSpecAndTemplate_MissingTemplateFile(t *testing.T) {
	stubProvisionAndResolveComponentPath(t, t.TempDir(), nil)

	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "vpc", "template": "missing.yaml"}}
	_, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationTemplate)
}

// resolveSpecAndTemplate must propagate a missing stack-policy-file error.
func TestResolveSpecAndTemplate_MissingStackPolicyFile(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "template.yaml"), []byte("AWSTemplateFormatVersion: '2010-09-09'"), 0o644))
	stubProvisionAndResolveComponentPath(t, tempDir, nil)

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"stack_name":   "vpc",
			"template":     "template.yaml",
			"stack_policy": map[string]any{"file": "missing-policy.json"},
		},
	}
	_, err := resolveSpecAndTemplate(&schema.AtmosConfiguration{}, info, OperationApply)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-policy.json")
}

// stubExecutorSeams overrides every executor.go seam var with a happy-path
// stub and returns a restore func the caller can defer. Individual fields on
// the returned struct let a test override just the seam it wants to exercise.
type executorSeamStubs struct {
	initCliConfig                    func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error)
	processStacks                    func(*schema.AtmosConfiguration, schema.ConfigAndStacksInfo, bool, bool, bool, []string, auth.AuthManager) (schema.ConfigAndStacksInfo, error)
	setupComponentAuthForCLI         func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error)
	propagateAuth                    func(*schema.ConfigAndStacksInfo, auth.AuthManager)
	provisionAndResolveComponentPath func(context.Context, provisioner.OutputWriters, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error)
	getHooks                         func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error)
}

// installExecutorSeamStubs overrides the package-level seam vars in executor.go
// with the given stubs, restoring the originals on test cleanup.
func installExecutorSeamStubs(t *testing.T, stubs executorSeamStubs) {
	t.Helper()

	origInitCliConfig := initCliConfig
	origProcessStacks := processStacks
	origSetupAuth := setupComponentAuthForCLI
	origPropagateAuth := propagateAuth
	origProvision := provisionAndResolveComponentPath
	origGetHooks := getHooks

	initCliConfig = stubs.initCliConfig
	processStacks = stubs.processStacks
	setupComponentAuthForCLI = stubs.setupComponentAuthForCLI
	propagateAuth = stubs.propagateAuth
	provisionAndResolveComponentPath = stubs.provisionAndResolveComponentPath
	getHooks = stubs.getHooks

	t.Cleanup(func() {
		initCliConfig = origInitCliConfig
		processStacks = origProcessStacks
		setupComponentAuthForCLI = origSetupAuth
		propagateAuth = origPropagateAuth
		provisionAndResolveComponentPath = origProvision
		getHooks = origGetHooks
	})
}

// noopGetHooks forces the empty-Hooks shortcut in hooks.GetHooks (no
// ComponentFromArg/Stack), regardless of what the caller's info carries — a
// safe way to exercise runWithHooks without a live ExecuteDescribeComponent.
func noopGetHooks(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
	return hooks.GetHooks(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
}

// Execute must propagate an initCliConfig failure without dispatching to
// either the bulk or single path.
func TestExecute_InitCliConfigError(t *testing.T) {
	sentinel := errors.New("config load failed")
	installExecutorSeamStubs(t, executorSeamStubs{
		initCliConfig: func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, sentinel
		},
		processStacks: func(_ *schema.AtmosConfiguration, _ schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			t.Fatal("processStacks must not be called when initCliConfig fails")
			return schema.ConfigAndStacksInfo{}, nil
		},
	})

	ctx := &component.ExecutionContext{ConfigAndStacksInfo: schema.ConfigAndStacksInfo{}}
	err := Execute(ctx, OperationRender)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// Execute must dispatch to the bulk path (executeBulk) when info.All is set,
// short-circuiting on the (stubbed) executeDescribeStacks failure — proving
// dispatch happened without needing a full bulk-execution round trip.
func TestExecute_DispatchesToBulk(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{
		initCliConfig: func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
	})

	sentinel := errors.New("describe stacks failed")
	origDescribe := executeDescribeStacks
	executeDescribeStacks = func(_ *schema.AtmosConfiguration, _ string, _, _, _ []string, _, _, _, _ bool, _ []string, _ auth.AuthManager) (map[string]any, error) {
		return nil, sentinel
	}
	t.Cleanup(func() { executeDescribeStacks = origDescribe })

	ctx := &component.ExecutionContext{ConfigAndStacksInfo: schema.ConfigAndStacksInfo{All: true}}
	err := Execute(ctx, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "the bulk path must have been dispatched to reach executeDescribeStacks's error")
}

// Execute's full single-component happy path for a render (the only
// operation that never touches the AWS API — see runOperation), exercising
// Execute, executeSingle, resolveSpecAndTemplate, and runWithHooks together.
func TestExecute_Single_Render_Success(t *testing.T) {
	tempDir := t.TempDir()
	templateBody := "AWSTemplateFormatVersion: '2010-09-09'"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "template.yaml"), []byte(templateBody), 0o644))

	installExecutorSeamStubs(t, executorSeamStubs{
		initCliConfig: func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		},
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{"stack_name": "vpc", "template": "template.yaml"}
			return info, nil
		},
		setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			t.Fatal("render must never set up AWS auth")
			return nil, nil
		},
		propagateAuth: func(_ *schema.ConfigAndStacksInfo, _ auth.AuthManager) {
			t.Fatal("render must never propagate auth")
		},
		provisionAndResolveComponentPath: func(_ context.Context, _ provisioner.OutputWriters, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, _, _ string) (string, bool, error) {
			return tempDir, false, nil
		},
		getHooks: noopGetHooks,
	})

	ctx := &component.ExecutionContext{ConfigAndStacksInfo: schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}}
	err := Execute(ctx, OperationRender)
	require.NoError(t, err)
}

// executeSingle must skip validation/auth/resolution entirely and return nil
// when the discovered component is disabled.
func TestExecuteSingle_ComponentDisabled(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = false
			return info, nil
		},
		setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			t.Fatal("a disabled component must never reach auth setup")
			return nil, nil
		},
	})

	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationApply)
	require.NoError(t, err)
}

// executeSingle must propagate a processStacks failure.
func TestExecuteSingle_ProcessStacksError(t *testing.T) {
	sentinel := errors.New("stack discovery failed")
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, _ schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, sentinel
		},
	})

	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// executeSingle must reject an invalid component config (ValidateComponent)
// before ever reaching auth setup.
func TestExecuteSingle_ValidateComponentError(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{} // missing template/stack_name.
			return info, nil
		},
		setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			t.Fatal("an invalid component config must never reach auth setup")
			return nil, nil
		},
	})

	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationApply)
	require.Error(t, err)
}

// executeSingle must call auth setup for a mutating operation (anything but
// render) and propagate a failure from it.
func TestExecuteSingle_AuthSetupError(t *testing.T) {
	sentinel := errors.New("auth setup failed")
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{"stack_name": "vpc", "template": "template.yaml"}
			return info, nil
		},
		setupComponentAuthForCLI: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			return nil, sentinel
		},
	})

	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationApply)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// runWithHooks must propagate a getHooks failure without attempting to
// dispatch the operation.
func TestRunWithHooks_GetHooksError(t *testing.T) {
	sentinel := errors.New("hooks lookup failed")
	installExecutorSeamStubs(t, executorSeamStubs{
		getHooks: func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
			return nil, sentinel
		},
	})

	ctx := &component.ExecutionContext{}
	err := runWithHooks(ctx, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, OperationRender, &stackSpec{StackName: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// runWithHooks must propagate a runOperation failure (opErr) and never
// attempt the "after" hook — asserted here via an unrecognized operation,
// which runOperation rejects deterministically without any AWS API call.
func TestRunWithHooks_OpErrPropagates(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{getHooks: noopGetHooks})

	ctx := &component.ExecutionContext{}
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	err := runWithHooks(ctx, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, Operation("bogus"), spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
}

// runOperation must reject apply/delete when the confirmation prompt is
// declined, never reaching buildAWSConfig/newClient.
func TestRunOperation_Apply_ConfirmationDeclined(t *testing.T) {
	stubConfirmOperation(t, false, nil)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	octx := &opContext{Ctx: context.Background(), Info: &schema.ConfigAndStacksInfo{}, Flags: map[string]any{}}
	_, err := runOperation(octx, OperationApply, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestRunOperation_Render_NoAPICalls(t *testing.T) {
	// Render must never touch the AWS API — no mock expectations set means any
	// call would fail the test via gomock's unexpected-call panic.
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	octx := &opContext{Ctx: context.Background()}
	summary, err := runOperation(octx, OperationRender, spec)
	require.NoError(t, err)
	assert.Equal(t, spec.TemplateBody, summary["template"])
}
