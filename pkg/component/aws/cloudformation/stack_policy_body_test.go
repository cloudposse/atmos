package cloudformation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the schema field fails the build here.
var _ = schema.StackPolicy{Body: "{}"}

var stackPolicyDocument = map[string]any{
	"Statement": []any{map[string]any{"Effect": "Allow", "Action": "Update:*", "Principal": "*", "Resource": "*"}},
}

// TestBuildStackSpec_StackPolicy covers the file and inline body forms and their exclusivity.
func TestBuildStackSpec_StackPolicy(t *testing.T) {
	const inline = `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`
	tests := []struct {
		name     string
		policy   any
		wantFile string
		wantBody string
		wantErr  error
	}{
		{name: "neither", policy: nil},
		{name: "empty section", policy: map[string]any{}},
		{name: "file only", policy: map[string]any{"file": "policy.json"}, wantFile: "policy.json"},
		{name: "body string verbatim", policy: map[string]any{"body": inline}, wantBody: inline},
		{name: "body map serialized to JSON", policy: map[string]any{"body": stackPolicyDocument}, wantBody: inline},
		{name: "empty body with file", policy: map[string]any{"file": "policy.json", "body": ""}, wantFile: "policy.json"},
		{name: "empty body map with file", policy: map[string]any{"file": "policy.json", "body": map[string]any{}}, wantFile: "policy.json"},
		{name: "file and body string", policy: map[string]any{"file": "policy.json", "body": inline}, wantErr: errUtils.ErrAwsCloudFormationStackPolicyFileAndBodyMutuallyExclusive},
		{name: "file and body map", policy: map[string]any{"file": "policy.json", "body": stackPolicyDocument}, wantErr: errUtils.ErrAwsCloudFormationStackPolicyFileAndBodyMutuallyExclusive},
		{name: "unsupported body type", policy: map[string]any{"body": 42}, wantErr: errUtils.ErrInvalidAwsCloudFormationSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			section := map[string]any{"stack_name": "vpc", "template": "Resources: {}"}
			if tt.policy != nil {
				section["stack_policy"] = tt.policy
			}
			spec, err := buildStackSpec(section)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFile, spec.StackPolicyFile)
			if tt.wantBody == "" {
				assert.Empty(t, spec.StackPolicyBody)
				return
			}
			assert.JSONEq(t, tt.wantBody, spec.StackPolicyBody)
		})
	}
}

// TestMutuallyExclusiveStackPolicyHint checks the error tells the user how to fix it.
func TestMutuallyExclusiveStackPolicyHint(t *testing.T) {
	_, err := buildStackSpec(map[string]any{
		"stack_name": "vpc", "template": "Resources: {}",
		"stack_policy": map[string]any{"file": "policy.json", "body": "{}"},
	})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackPolicyFileAndBodyMutuallyExclusive)
	assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "stack_policy.body")
}

// TestInlineStackPolicyBodyNeedsNoComponentDirectory verifies the inline policy is used without any
// component path resolution, for named execution and for operations that load a template.
func TestInlineStackPolicyBodyNeedsNoComponentDirectory(t *testing.T) {
	stubProvisionAndResolveComponentPath(t, "", errors.New("component path must not be resolved"))
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"stack_name": "vpc", "path": "absent-template.yaml",
		"stack_policy": map[string]any{"body": stackPolicyDocument},
	}}
	spec, err := resolveSpecAndTemplate(t.Context(), &schema.AtmosConfiguration{}, info, OperationChangesetExecute)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(spec.StackPolicyBody), &decoded))
	assert.Equal(t, stackPolicyDocument["Statement"].([]any)[0].(map[string]any)["Effect"], decoded["Statement"].([]any)[0].(map[string]any)["Effect"])
	assert.Empty(t, spec.TemplateBody, "named execution keeps its reviewed template")
}

// TestInlineStackPolicyBodySurvivesLoad verifies a template-loading operation keeps the inline policy.
func TestInlineStackPolicyBodySurvivesLoad(t *testing.T) {
	spec := &stackSpec{StackPolicyBody: `{"Statement":[]}`}
	body, err := loadStackPolicyBody(t.TempDir(), spec)
	require.NoError(t, err)
	assert.Equal(t, spec.StackPolicyBody, body)
}

// TestInlineStackPolicyBodyReachesSetStackPolicy sends the serialized inline policy to the API unchanged.
func TestInlineStackPolicyBodyReachesSetStackPolicy(t *testing.T) {
	spec, err := buildStackSpec(map[string]any{
		"stack_name": "vpc", "template": "Resources: {}",
		"stack_policy": map[string]any{"body": stackPolicyDocument},
	})
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().SetStackPolicy(gomock.Any(), &cloudformation.SetStackPolicyInput{
		StackName: awsString("vpc"), StackPolicyBody: awsString(spec.StackPolicyBody),
	}).Return(&cloudformation.SetStackPolicyOutput{}, nil)
	require.NoError(t, setStackPolicy(context.Background(), client, spec))
	assert.JSONEq(t, `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`, spec.StackPolicyBody)
}

// TestDryRunRejectsStackPolicyFileAndBody verifies dry-run validation reports the conflict before any AWS call.
func TestDryRunRejectsStackPolicyFileAndBody(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{DryRun: true, ComponentSection: map[string]any{
		"stack_name": "vpc", "template": "Resources: {}",
		"stack_policy": map[string]any{"file": "policy.json", "body": "{}"},
	}}
	for _, operation := range []Operation{OperationApply, OperationChangesetExecute} {
		t.Run(string(operation), func(t *testing.T) {
			err := validateDryRun(&schema.AtmosConfiguration{}, info, nil, operation)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackPolicyFileAndBodyMutuallyExclusive)
		})
	}
}

// TestDryRunAcceptsInlineStackPolicyBody verifies an inline policy validates statically.
func TestDryRunAcceptsInlineStackPolicyBody(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{DryRun: true, ComponentSection: map[string]any{
		"stack_name": "vpc", "template": "Resources: {}",
		"stack_policy": map[string]any{"body": stackPolicyDocument},
	}}
	require.NoError(t, validateDryRun(&schema.AtmosConfiguration{}, info, nil, OperationApply))
}

// TestWorkdirRequiresSource verifies provision.workdir.enabled is rejected without a source.
func TestWorkdirRequiresSource(t *testing.T) {
	workdir := func(enabled any) map[string]any {
		return map[string]any{"workdir": map[string]any{"enabled": enabled}}
	}
	source := map[string]any{"uri": "https://example.com/template.yaml"}
	tests := []struct {
		name      string
		provision any
		source    any
		wantErr   bool
	}{
		{name: "enabled without source", provision: workdir(true), wantErr: true},
		{name: "enabled with source", provision: workdir(true), source: source},
		{name: "disabled without source", provision: workdir(false)},
		{name: "workdir without enabled", provision: map[string]any{"workdir": map[string]any{}}},
		{name: "provision absent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			section := map[string]any{"stack_name": "vpc", "template": "Resources: {}"}
			if tt.provision != nil {
				section["provision"] = tt.provision
			}
			if tt.source != nil {
				section["source"] = tt.source
			}
			err := validateComponentConfig(section)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			assert.Contains(t, errUtils.Format(err, errUtils.FormatterConfig{}), "source:")

			info := &schema.ConfigAndStacksInfo{DryRun: true, ComponentSection: section}
			require.ErrorIs(t, validateDryRun(&schema.AtmosConfiguration{}, info, nil, OperationApply), errUtils.ErrInvalidAwsCloudFormationSettings)
		})
	}
}
