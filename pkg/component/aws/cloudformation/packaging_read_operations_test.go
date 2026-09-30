package cloudformation

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func previewContext() *opContext {
	return &opContext{Ctx: context.Background(), AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{
		Stack: "dev", ComponentFromArg: "vpc", ComponentSection: map[string]any{cfg.ProvisionSectionName: map[string]any{
			"default": "deploy", "targets": map[string]any{
				"deploy":    map[string]any{"kind": cfg.CloudFormationComponentType},
				"artifacts": map[string]any{"kind": kindAwsS3, "bucket": "templates", "region": "us-east-1"},
			},
		}},
	}, Flags: map[string]any{}}
}

func expectPreviewRequest(t *testing.T, client *MockCloudFormationClient, operation Operation, spec *stackSpec) {
	t.Helper()
	if operation == OperationValidate {
		client.EXPECT().ValidateTemplate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *cloudformation.ValidateTemplateInput, _ ...func(*cloudformation.Options)) (*cloudformation.ValidateTemplateOutput, error) {
			assertTemplateInput(t, spec, input.TemplateBody, input.TemplateURL)
			return &cloudformation.ValidateTemplateOutput{}, nil
		})
		return
	}
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
		assertTemplateInput(t, spec, input.TemplateBody, input.TemplateURL)
		return &cloudformation.CreateChangeSetOutput{}, nil
	})
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil)
}

func assertTemplateInput(t *testing.T, spec *stackSpec, body, url *string) {
	t.Helper()
	if spec.TemplateURL != "" {
		require.Nil(t, body)
		require.NotNil(t, url)
		assert.Equal(t, spec.TemplateURL, *url)
	} else {
		require.Nil(t, url)
		require.NotNil(t, body)
		assert.Equal(t, spec.TemplateBody, *body)
	}
}

func runPreview(octx *opContext, client CloudFormationClient, operation Operation, spec *stackSpec) (map[string]any, error) {
	if operation == OperationValidate {
		return runValidate(octx, client, spec, map[string]any{})
	}
	return runDiff(octx, client, spec, map[string]any{})
}

func TestPreviewTemplatePackaging(t *testing.T) {
	for _, operation := range []Operation{OperationDiff, OperationValidate} {
		for _, mode := range []string{"inline-limit", "large", "already-packaged"} {
			t.Run(string(operation)+"/"+mode, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				client := NewMockCloudFormationClient(ctrl)
				backend := artifact.NewMockBackend(ctrl)
				stubNewS3Backend(t, backend, nil)
				spec := &stackSpec{StackName: "vpc", TemplateBody: strings.Repeat("a", templateInlineSizeLimit)}
				octx := previewContext()
				if mode == "large" {
					spec.TemplateBody += "a"
					backend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), int64(len(spec.TemplateBody)), gomock.Any()).DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ *artifact.Metadata) error {
						uploaded, err := io.ReadAll(r)
						require.NoError(t, err)
						assert.Equal(t, spec.TemplateBody, string(uploaded))
						return nil
					})
				} else {
					// Inline and already-uploaded templates need no packaging target.
					octx.Info.ComponentSection = nil
					if mode == "already-packaged" {
						spec.TemplateBody += "a"
						spec.TemplateURL = "https://templates.s3.us-east-1.amazonaws.com/existing.yaml"
					}
				}
				expectPreviewRequest(t, client, operation, spec)
				summary, err := runPreview(octx, client, operation, spec)
				require.NoError(t, err)
				if mode == "large" {
					assert.Contains(t, spec.TemplateURL, "https://templates.s3.us-east-1.amazonaws.com/")
					assert.Equal(t, spec.TemplateURL, summary["package_url"])
					assert.NotEmpty(t, summary["package_sha256"])
				} else {
					assert.NotContains(t, summary, "package_url")
				}
			})
		}
	}
}

func TestPreviewPackagingErrorsPreventAPICalls(t *testing.T) {
	for _, operation := range []Operation{OperationDiff, OperationValidate} {
		for _, failure := range []string{"missing-target", "ambiguous-target", "unknown-target", "upload"} {
			t.Run(string(operation)+"/"+failure, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				client := NewMockCloudFormationClient(ctrl)
				octx := previewContext()
				spec := &stackSpec{StackName: "vpc", TemplateBody: strings.Repeat("a", templateInlineSizeLimit+1)}
				sentinel := errors.New("upload failed")
				switch failure {
				case "missing-target":
					octx.Info.ComponentSection = nil
				case "ambiguous-target":
					provision := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
					provision["targets"].(map[string]any)["other"] = map[string]any{"kind": kindAwsS3, "bucket": "other", "region": "us-east-1"}
				case "unknown-target":
					octx.Flags[targetKey] = "missing"
				case "upload":
					backend := artifact.NewMockBackend(ctrl)
					backend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(sentinel)
					stubNewS3Backend(t, backend, nil)
				}
				_, err := runPreview(octx, client, operation, spec)
				require.Error(t, err)
				assert.Empty(t, spec.TemplateURL)
				if failure == "upload" {
					assert.ErrorIs(t, err, sentinel)
				}
				if failure == "missing-target" || failure == "ambiguous-target" {
					assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
				}
			})
		}
	}
}
