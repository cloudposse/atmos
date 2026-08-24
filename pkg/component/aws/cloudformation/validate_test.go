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
			config:  map[string]any{"template": "template.yaml"},
			wantErr: errUtils.ErrMissingAwsCloudFormationStackName,
		},
		{
			name:   "valid",
			config: map[string]any{"template": "template.yaml", "stack_name": "vpc"},
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

func TestValidateTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ValidateTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.ValidateTemplateOutput{}, nil)

	err := validateTemplate(context.Background(), client, "AWSTemplateFormatVersion: '2010-09-09'")
	require.NoError(t, err)
}

func TestValidateTemplate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ValidateTemplate(gomock.Any(), gomock.Any()).Return(nil, errors.New("invalid template"))

	err := validateTemplate(context.Background(), client, "not a template")
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
