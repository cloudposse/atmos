package cloudformation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestS3BackendDryRunSummary(t *testing.T) {
	s3Target := func(block map[string]any) map[string]any {
		block["kind"] = kindAwsS3
		return map[string]any{"provision": map[string]any{"targets": map[string]any{"artifacts": block}}}
	}
	tests := []struct {
		name       string
		config     map[string]any
		flagTarget string
		action     string
		want       string
		wantErr    error
	}{
		{
			name:   "create names bucket and region",
			config: s3Target(map[string]any{"bucket": "my-bucket", "region": "us-east-2"}),
			action: "create",
			want:   "would create bucket my-bucket in region us-east-2",
		},
		{
			name:   "delete",
			config: s3Target(map[string]any{"bucket": "my-bucket", "region": "us-east-2"}),
			action: "delete",
			want:   "would delete bucket my-bucket in region us-east-2",
		},
		{
			name: "region falls back to settings",
			config: func() map[string]any {
				c := s3Target(map[string]any{"bucket": "my-bucket"})
				c["settings"] = map[string]any{"aws_cloudformation": map[string]any{"region": "eu-west-1"}}
				return c
			}(),
			action: "create",
			want:   "would create bucket my-bucket in region eu-west-1",
		},
		{
			name:   "unresolved region is described, not blank",
			config: s3Target(map[string]any{"bucket": "my-bucket"}),
			action: "create",
			want:   "would create bucket my-bucket in region the region of the active identity",
		},
		{
			name:       "explicit target flag",
			config:     s3Target(map[string]any{"bucket": "my-bucket", "region": "us-east-2"}),
			flagTarget: "artifacts",
			action:     "create",
			want:       "would create bucket my-bucket in region us-east-2",
		},
		{name: "no s3 target", config: map[string]any{}, action: "create", wantErr: errUtils.ErrInvalidAwsCloudFormationSettings},
		{
			name:       "unknown target flag",
			config:     s3Target(map[string]any{"bucket": "my-bucket", "region": "us-east-2"}),
			flagTarget: "other",
			action:     "create",
			wantErr:    errUtils.ErrInvalidAwsCloudFormationSettings,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := S3BackendDryRunSummary(tt.config, tt.flagTarget, tt.action)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Backend describe/list output uses the snake_case keys of the rest of the cfn output.
func TestS3BackendStatusUsesSnakeCaseKeys(t *testing.T) {
	status := &S3BackendStatus{
		Target: &targetS3Config{Name: "artifacts", Bucket: "my-bucket", Prefix: "templates", Region: "us-east-2"},
		Region: "us-east-2",
		Exists: true,
	}
	want := map[string]any{
		"target": map[string]any{"name": "artifacts", "bucket": "my-bucket", "prefix": "templates", "region": "us-east-2"},
		"region": "us-east-2",
		"exists": true,
	}

	raw, err := json.Marshal(status)
	require.NoError(t, err)
	var fromJSON map[string]any
	require.NoError(t, json.Unmarshal(raw, &fromJSON))
	assert.Equal(t, want, fromJSON)

	raw, err = yaml.Marshal(status)
	require.NoError(t, err)
	var fromYAML map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &fromYAML))
	assert.Equal(t, want, fromYAML)
}

// ProvisionS3BackendTarget must report the "settings will be replaced" warning before the
// final success line, so the last line describes the completed result.
func TestProvisionS3BackendTarget_WarnsBeforeSuccess(t *testing.T) {
	tests := []struct {
		name         string
		client       *fakeS3Client
		wantWarnings bool
	}{
		{name: "existing bucket warns", client: &fakeS3Client{}, wantWarnings: true},
		{name: "new bucket does not warn", client: &fakeS3Client{headBucketErr: &types.NotFound{}}, wantWarnings: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(backend.ResetS3ClientFactory)
			backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI { return tt.client })

			var err error
			out := captureStderr(t, func() {
				err = ProvisionS3BackendTarget(context.Background(), &ProvisionS3BackendParams{
					AtmosConfig:     &schema.AtmosConfiguration{},
					Target:          &targetS3Config{Name: "artifacts", Bucket: "my-bucket", Region: "us-east-1"},
					ComponentConfig: map[string]any{},
					Component:       "vpc",
					Stack:           "dev",
				})
			})
			require.NoError(t, err)

			text := normalizeUIOutput(out)
			success := strings.Index(text, "Provisioned S3 backend")
			require.GreaterOrEqual(t, success, 0, text)
			warning := strings.Index(text, "Tags will be replaced")
			if tt.wantWarnings {
				require.GreaterOrEqual(t, warning, 0, text)
				assert.Less(t, warning, success, "warnings must print before the success line")
			} else {
				assert.Equal(t, -1, warning)
			}
		})
	}
}
