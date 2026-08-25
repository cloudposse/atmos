package exec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubCloudFormationOutputsGetter replaces the cloudFormationOutputsGetter
// seam for a single test, restoring the original on cleanup.
func stubCloudFormationOutputsGetter(t *testing.T, getter CloudFormationOutputsGetter) {
	t.Helper()
	original := cloudFormationOutputsGetter
	cloudFormationOutputsGetter = getter
	t.Cleanup(func() { cloudFormationOutputsGetter = original })
}

// defaultCloudFormationOutputsGetter.GetOutputs must actually delegate to
// pkg/aws/cloudformation.GetOutputs (not silently no-op) — asserted here by
// pointing it at a closed local server (guaranteed to refuse the connection)
// and confirming the underlying network call was actually attempted.
func TestDefaultCloudFormationOutputsGetter_GetOutputs_Delegates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := srv.URL
	srv.Close()

	getter := defaultCloudFormationOutputsGetter{}
	_, err := getter.GetOutputs(context.Background(), "us-east-1", "vpc", &schema.AWSAuthContext{EndpointURL: unreachable})
	require.Error(t, err, "GetOutputs must delegate to pkg/aws/cloudformation and surface the (unreachable) endpoint's failure")
}

// GetCloudFormationOutputs must delegate to the cloudFormationOutputsGetter
// seam, forwarding every argument unchanged.
func TestGetCloudFormationOutputs_DelegatesToSeam(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)

	authContext := &schema.AWSAuthContext{Profile: "dev"}
	want := map[string]any{"VpcId": "vpc-123"}
	mockGetter.EXPECT().GetOutputs(gomock.Any(), "us-west-2", "vpc", authContext).Return(want, nil)

	stubCloudFormationOutputsGetter(t, mockGetter)

	got, err := GetCloudFormationOutputs(context.Background(), "us-west-2", "vpc", authContext)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// GetCloudFormationOutputs must propagate a seam error unchanged.
func TestGetCloudFormationOutputs_PropagatesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)

	sentinel := errors.New("stack not found")
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, sentinel)
	stubCloudFormationOutputsGetter(t, mockGetter)

	_, err := GetCloudFormationOutputs(context.Background(), "us-east-1", "vpc", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// resolveCloudFormationRegion must extract settings.aws_cloudformation.region
// when present, and return "" for every absent-nesting shape.
func TestResolveCloudFormationRegion(t *testing.T) {
	tests := []struct {
		name     string
		sections map[string]any
		want     string
	}{
		{
			name: "region present",
			sections: map[string]any{
				cfg.SettingsSectionName: map[string]any{"aws_cloudformation": map[string]any{"region": "us-west-2"}},
			},
			want: "us-west-2",
		},
		{"no settings section", map[string]any{}, ""},
		{
			name:     "settings present but not a map",
			sections: map[string]any{cfg.SettingsSectionName: "not-a-map"},
			want:     "",
		},
		{
			name:     "settings present without aws_cloudformation",
			sections: map[string]any{cfg.SettingsSectionName: map[string]any{}},
			want:     "",
		},
		{
			name: "aws_cloudformation present but not a map",
			sections: map[string]any{
				cfg.SettingsSectionName: map[string]any{"aws_cloudformation": "not-a-map"},
			},
			want: "",
		},
		{
			name: "aws_cloudformation present without region",
			sections: map[string]any{
				cfg.SettingsSectionName: map[string]any{"aws_cloudformation": map[string]any{}},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveCloudFormationRegion(tt.sections))
		})
	}
}

// cloudFormationOutputsForSections must error when the target section has no
// stack_name, without ever reaching the outputs getter.
func TestCloudFormationOutputsForSections_MissingStackName(t *testing.T) {
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	_, err := cloudFormationOutputsForSections(&schema.AtmosConfiguration{}, "vpc", map[string]any{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationStackName)
	assert.Contains(t, err.Error(), "vpc")
}

// cloudFormationOutputsForSections must resolve stack_name/region from
// sections and forward the AWS-specific slice of a nil AuthContext.
func TestCloudFormationOutputsForSections_NilAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)

	var gotAuth *schema.AWSAuthContext
	mockGetter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "vpc", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, authCtx *schema.AWSAuthContext) (map[string]any, error) {
			gotAuth = authCtx
			return map[string]any{"VpcId": "vpc-123"}, nil
		},
	)
	stubCloudFormationOutputsGetter(t, mockGetter)

	sections := map[string]any{
		cfg.StackNameSectionName: "vpc",
		cfg.SettingsSectionName:  map[string]any{"aws_cloudformation": map[string]any{"region": "us-east-1"}},
	}
	got, err := cloudFormationOutputsForSections(&schema.AtmosConfiguration{}, "vpc", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-123"}, got)
	assert.Nil(t, gotAuth, "a nil AuthContext must forward a nil AWSAuthContext, not panic")
}

// cloudFormationOutputsForSections must forward the populated AuthContext's
// AWS sub-context to the outputs getter.
func TestCloudFormationOutputsForSections_PopulatedAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)

	awsAuth := &schema.AWSAuthContext{Profile: "dev"}
	authContext := &schema.AuthContext{AWS: awsAuth}

	var gotAuth *schema.AWSAuthContext
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), "vpc", gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, authCtx *schema.AWSAuthContext) (map[string]any, error) {
			gotAuth = authCtx
			return map[string]any{}, nil
		},
	)
	stubCloudFormationOutputsGetter(t, mockGetter)

	sections := map[string]any{cfg.StackNameSectionName: "vpc"}
	_, err := cloudFormationOutputsForSections(&schema.AtmosConfiguration{}, "vpc", sections, authContext)
	require.NoError(t, err)
	assert.Same(t, awsAuth, gotAuth)
}
