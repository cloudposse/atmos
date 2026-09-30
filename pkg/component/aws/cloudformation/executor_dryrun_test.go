package cloudformation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunOperation_DryRunSkipsConfirmationAndRequests(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `<ErrorResponse><Error><Code>ValidationError</Code><Message>unexpected dry-run request</Message></Error></ErrorResponse>`)
	}))
	defer server.Close()
	oldConfirm, oldTerminal := confirmOperation, stdinIsTerminal
	t.Cleanup(func() { confirmOperation = oldConfirm; stdinIsTerminal = oldTerminal })
	var confirmations int
	stdinIsTerminal = func() bool { return true }
	confirmOperation = func(string) (bool, error) { confirmations++; return false, nil }
	for _, operation := range []Operation{OperationApply, OperationDelete, OperationDiff, OperationValidate, OperationOutput} {
		for _, autoApprove := range []bool{false, true} {
			t.Run(string(operation)+map[bool]string{false: "/interactive", true: "/auto-approved"}[autoApprove], func(t *testing.T) {
				info := &schema.ConfigAndStacksInfo{DryRun: true, AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{EndpointURL: server.URL}}}
				summary, err := runOperation(&opContext{Ctx: context.Background(), Info: info, Flags: map[string]any{"auto-approve": autoApprove}}, operation, &stackSpec{StackName: "dry-run-stack", TemplateBody: "Resources: {}"})
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"stack_name": "dry-run-stack"}, summary)
			})
		}
	}
	assert.Zero(t, confirmations, "dry-run must not request mutation approval")
	assert.Zero(t, requests.Load(), "dry-run must not send CloudFormation requests")
}

func TestExecute_DryRunSkipsAuthProvisioningAndHooks(t *testing.T) {
	for _, operation := range []Operation{OperationApply, OperationDelete, OperationDiff, OperationValidate, OperationOutput} {
		t.Run(string(operation), func(t *testing.T) {
			installExecutorSeamStubs(t, executorSeamStubs{
				initCliConfig: func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, nil
				},
				processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
					info.ComponentIsEnabled = true
					info.ComponentSection = map[string]any{"stack_name": "example", "template": "unprovisioned.yaml"}
					return info, nil
				},
				setupComponentAuthForCLI: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
					t.Fatal("dry-run must not authenticate")
					return nil, nil
				},
				propagateAuth: func(*schema.ConfigAndStacksInfo, auth.AuthManager) { t.Fatal("dry-run must not propagate auth") },
				provisionAndResolveComponentPath: func(context.Context, provisioner.OutputWriters, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
					t.Fatal("dry-run must not provision sources")
					return "", false, nil
				},
				getHooks: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*hooks.Hooks, error) {
					t.Fatal("dry-run must not load/run hooks")
					return nil, nil
				},
			})
			require.NoError(t, Execute(&component.ExecutionContext{ConfigAndStacksInfo: schema.ConfigAndStacksInfo{DryRun: true, ComponentFromArg: "example"}}, operation))
		})
	}
}

func TestExecuteSingle_DryRunStillValidatesComponent(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{"stack_name": "example"}
			return info, nil
		},
	})
	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{DryRun: true}, OperationApply)
	require.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationTemplate)
}

func TestAuthManagerForBulk_DryRunSkipsUnknownIdentity(t *testing.T) {
	manager, err := authManagerForBulk(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{DryRun: true, Identity: "unconfigured"})
	require.NoError(t, err)
	assert.Nil(t, manager)
}

func TestRunOperation_DryRunRenderStillReturnsTemplate(t *testing.T) {
	spec := &stackSpec{StackName: "example", TemplateBody: "Resources: {}"}
	summary, err := runOperation(&opContext{Ctx: context.Background(), Info: &schema.ConfigAndStacksInfo{DryRun: true}}, OperationRender, spec)
	require.NoError(t, err)
	assert.Equal(t, spec.TemplateBody, summary["template"])
}

func TestExecuteSingle_DryRunRenderStillLoadsTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte("Resources: {}"), 0o644))
	var provisions int
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{"stack_name": "example", "template": "template.yaml"}
			return info, nil
		},
		provisionAndResolveComponentPath: func(context.Context, provisioner.OutputWriters, *schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, string, string) (string, bool, error) {
			provisions++
			return dir, false, nil
		},
		getHooks: noopGetHooks,
	})
	require.NoError(t, executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{DryRun: true}, OperationRender))
	assert.Equal(t, 1, provisions)
}

func TestExecuteSingle_DryRunStillValidatesParameters(t *testing.T) {
	installExecutorSeamStubs(t, executorSeamStubs{
		processStacks: func(_ *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, _, _, _ bool, _ []string, _ auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
			info.ComponentIsEnabled = true
			info.ComponentSection = map[string]any{"stack_name": "example", "template": "template.yaml", "parameters": map[string]any{"invalid": map[string]any{"nested": "value"}}}
			return info, nil
		},
	})
	err := executeSingle(&component.ExecutionContext{}, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{DryRun: true}, OperationApply)
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}
