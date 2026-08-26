package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateComponentConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr error
	}{
		{name: "nil config is valid (unset)", config: nil},
		{
			name:   "abstract component needs nothing",
			config: map[string]any{"metadata": map[string]any{"type": "abstract"}},
		},
		{
			name:    "missing template",
			config:  map[string]any{"stack_name": "vpc"},
			wantErr: errUtils.ErrMissingAwsCloudFormationTemplate,
		},
		{
			name:    "missing stack_name",
			config:  map[string]any{"path": "template.yaml"},
			wantErr: errUtils.ErrMissingAwsCloudFormationStackName,
		},
		{
			name:   "valid with path",
			config: map[string]any{"path": "template.yaml", "stack_name": "vpc"},
		},
		{
			name: "valid with inline string template",
			config: map[string]any{
				"template":   "AWSTemplateFormatVersion: '2010-09-09'\nResources: {}\n",
				"stack_name": "vpc",
			},
		},
		{
			name: "valid with inline map template",
			config: map[string]any{
				"template": map[string]any{
					"Resources": map[string]any{
						"Bucket": map[string]any{"Type": "AWS::S3::Bucket"},
					},
				},
				"stack_name": "vpc",
			},
		},
		{
			name: "template and path both set",
			config: map[string]any{
				"template":   "Resources: {}\n",
				"path":       "template.yaml",
				"stack_name": "vpc",
			},
			wantErr: errUtils.ErrAwsCloudFormationTemplateAndPathMutuallyExclusive,
		},
		{
			name: "inline template invalid yaml",
			config: map[string]any{
				"template":   "template.yaml",
				"stack_name": "vpc",
			},
			wantErr: errUtils.ErrInvalidAwsCloudFormationSettings,
		},
		{
			name: "inline template missing Resources",
			config: map[string]any{
				"template":   "AWSTemplateFormatVersion: '2010-09-09'\n",
				"stack_name": "vpc",
			},
			wantErr: errUtils.ErrAwsCloudFormationTemplateMissingResources,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateComponentConfig(tt.config)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// validateTemplate must print a success confirmation naming the stack — a
// successful validation otherwise produced zero output, indistinguishable
// from a hang short of checking the exit code.
func TestValidateTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ValidateTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.ValidateTemplateOutput{}, nil)

	out := captureStderr(t, func() {
		err := validateTemplate(context.Background(), client, "vpc", "AWSTemplateFormatVersion: '2010-09-09'")
		require.NoError(t, err)
	})
	assert.Contains(t, out, "vpc")
	assert.Contains(t, out, "template is valid")
}

func TestValidateTemplate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ValidateTemplate(gomock.Any(), gomock.Any()).Return(nil, errors.New("invalid template"))

	err := validateTemplate(context.Background(), client, "vpc", "not a template")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidSpecificAwsCloudFormationComponent)
}

func TestSetStackPolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().SetStackPolicy(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.SetStackPolicyInput, _ ...func(*cloudformation.Options)) (*cloudformation.SetStackPolicyOutput, error) {
			assert.Equal(t, "vpc", *input.StackName)
			assert.Equal(t, `{"Statement":[]}`, *input.StackPolicyBody)
			return &cloudformation.SetStackPolicyOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc", StackPolicyBody: `{"Statement":[]}`}
	err := setStackPolicy(context.Background(), client, spec)
	require.NoError(t, err)
}

// applyTerminationProtection must reconcile the stack's actual termination-protection
// state with spec.TerminationProtection on every apply — CreateChangeSet/ExecuteChangeSet
// have no termination-protection parameter, so this is a follow-up UpdateTerminationProtection
// call, the same shape setStackPolicy already uses for stack policy.
func TestApplyTerminationProtection_Enables(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.UpdateTerminationProtectionInput, _ ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error) {
			assert.Equal(t, "vpc", *input.StackName)
			assert.True(t, *input.EnableTerminationProtection)
			return &cloudformation.UpdateTerminationProtectionOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := applyTerminationProtection(context.Background(), client, spec)
	require.NoError(t, err)
}

// applyTerminationProtection must also actively disable protection when config no
// longer requests it — config is always the source of truth, so removing
// termination_protection: true from a component must take effect on the next apply,
// not just stop being enforced by Atmos's own `delete` command.
func TestApplyTerminationProtection_Disables(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.UpdateTerminationProtectionInput, _ ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error) {
			assert.Equal(t, "vpc", *input.StackName)
			assert.False(t, *input.EnableTerminationProtection)
			return &cloudformation.UpdateTerminationProtectionOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: false}
	err := applyTerminationProtection(context.Background(), client, spec)
	require.NoError(t, err)
}

func TestApplyTerminationProtection_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(nil, errors.New("api failure"))

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := applyTerminationProtection(context.Background(), client, spec)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}
