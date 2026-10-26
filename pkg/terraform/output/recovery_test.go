package output

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
)

func TestOutputPoliciesRetryActualInvocations(t *testing.T) {
	recoverytest.Install(t, "terraform.*.error", "input.output.contains('transient')")
	runner := NewMockTerraformRunner(gomock.NewController(t))
	var stderr io.Writer
	runner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	runner.EXPECT().SetStderr(gomock.Any()).Do(func(writer io.Writer) { stderr = writer }).AnyTimes()
	runner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	gomock.InOrder(
		runner.EXPECT().Init(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, ...tfexec.InitOption) error {
			_, err := io.WriteString(stderr, "transient init")
			require.NoError(t, err)
			return errors.New("failed")
		}),
		runner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil),
		runner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil),
		runner.EXPECT().Output(gomock.Any()).DoAndReturn(func(context.Context, ...tfexec.OutputOption) (map[string]tfexec.OutputMeta, error) {
			_, err := io.WriteString(stderr, "transient output")
			require.NoError(t, err)
			return nil, errors.New("failed")
		}),
		runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"id": {Value: []byte(`"vpc-123"`)}}, nil),
	)
	executor := NewExecutor(nil, WithRunnerFactory(func(string, string) (TerraformRunner, error) { return runner, nil }))
	outputs, err := executor.ExecuteWithSections(validAtmosConfig(t), "test-component", "test-stack", validSections(), nil)
	require.NoError(t, err)
	require.Equal(t, "vpc-123", outputs["id"])
}

func TestOutputPoliciesPreservePassVarsInit(t *testing.T) {
	recoverytest.Install(t, "terraform.init.error", "input.stderr.contains('transient')")
	runner := NewMockTerraformRunner(gomock.NewController(t))
	runner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	runner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	runner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	runner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	runner.EXPECT().Output(gomock.Any()).Return(nil, nil)
	attempts := 0
	executor := NewExecutor(
		nil,
		WithRunnerFactory(func(string, string) (TerraformRunner, error) { return runner, nil }),
		WithInitWithVars(func(_ context.Context, request *InitWithVarsRequest) error {
			attempts++
			require.Equal(t, "value", request.Env["TF_VAR_example"])
			if attempts == 1 {
				_, err := io.WriteString(request.Stderr, "transient")
				require.NoError(t, err)
				return errors.New("failed")
			}
			return nil
		}),
	)
	config := validAtmosConfig(t)
	config.Components.Terraform.Init.PassVars = true
	sections := validSections()
	sections["vars"] = map[string]any{"example": "value"}
	_, err := executor.ExecuteWithSections(config, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}
