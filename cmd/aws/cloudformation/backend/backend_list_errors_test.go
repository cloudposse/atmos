package backend

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errMistypedIdentity is the cause the fake resolver returns for the broken target.
var errMistypedIdentity = errors.New("identity \"devv\" not found")

// brokenAndHealthyComponentConfig declares two aws/s3 targets: a-broken sorts first so that
// an implementation that aborts on the first failure would hide b-good.
func brokenAndHealthyComponentConfig() map[string]any {
	return map[string]any{
		"provision": map[string]any{
			"targets": map[string]any{
				"a-broken": map[string]any{"kind": "aws/s3", "bucket": "broken-bucket", "region": "us-east-1"},
				"b-good":   map[string]any{"kind": "aws/s3", "bucket": "good-bucket", "region": "us-east-1"},
			},
		},
	}
}

// stubTargetAuth replaces the target auth seam, counting calls per target and failing for the named targets.
func stubTargetAuth(t *testing.T, failing ...string) map[string]int {
	t.Helper()
	original := resolveTargetAuth
	t.Cleanup(func() { resolveTargetAuth = original })

	calls := map[string]int{}
	resolveTargetAuth = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, targetName string, _ map[string]any, _ string) (*schema.ConfigAndStacksInfo, error) {
		calls[targetName]++
		for _, name := range failing {
			if name == targetName {
				return nil, errMistypedIdentity
			}
		}
		return &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{}}, nil
	}
	return calls
}

func useFakeS3(t *testing.T, exists bool) {
	t.Helper()
	t.Cleanup(backend.ResetS3ClientFactory)
	backend.SetS3ClientFactory(func(aws.Config, ...func(*s3.Options)) backend.S3ClientAPI {
		return &fakeS3Client{exists: exists}
	})
}

// TestDefaultProvisioner_ListBackends_BrokenTargetStillRendersHealthyOnes verifies a target that cannot
// authenticate is rendered as a named error row, the healthy target still prints, and the command fails.
func TestDefaultProvisioner_ListBackends_BrokenTargetStillRendersHealthyOnes(t *testing.T) {
	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, out string)
	}{
		{
			name:   "table",
			format: "table",
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "a-broken")
				assert.Contains(t, out, "error: "+errMistypedIdentity.Error())
				assert.Contains(t, out, "b-good")
				assert.Contains(t, out, "good-bucket")
				assert.Contains(t, out, "does not exist")
			},
		},
		{
			name:   "yaml",
			format: "yaml",
			check: func(t *testing.T, out string) {
				assert.Contains(t, out, "name: a-broken")
				assert.Contains(t, out, "error: ")
				assert.Contains(t, out, "devv")
				assert.Contains(t, out, "name: b-good")
				assert.Contains(t, out, "bucket: good-bucket")
			},
		},
		{
			name:   "json",
			format: "json",
			check: func(t *testing.T, out string) {
				var rows []map[string]any
				require.NoError(t, json.Unmarshal([]byte(out), &rows))
				require.Len(t, rows, 2)
				first, _ := rows[0]["target"].(map[string]any)
				last, _ := rows[1]["target"].(map[string]any)
				assert.Equal(t, "a-broken", first["name"])
				assert.Equal(t, errMistypedIdentity.Error(), rows[0]["error"])
				assert.Equal(t, "b-good", last["name"])
				assert.NotContains(t, rows[1], "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubTargetAuth(t, "a-broken")
			useFakeS3(t, false)

			p := &defaultProvisioner{}
			var err error
			out := captureStdout(t, func() {
				err = p.ListBackends(context.Background(), &ListBackendsParams{
					AtmosConfig:     &schema.AtmosConfiguration{},
					Component:       "vpc",
					Stack:           "dev",
					ComponentConfig: brokenAndHealthyComponentConfig(),
					Format:          tt.format,
				})
			})

			tt.check(t, out)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendTargetsFailed)
			assert.ErrorIs(t, err, errMistypedIdentity, "the failing target's cause stays in the chain")
			assert.Contains(t, err.Error(), `target "a-broken"`)
			assert.NotContains(t, err.Error(), "b-good")
		})
	}
}

// TestDefaultProvisioner_ListBackends_AllHealthyReturnsNil is the negative path: no failure error without a failed target.
func TestDefaultProvisioner_ListBackends_AllHealthyReturnsNil(t *testing.T) {
	calls := stubTargetAuth(t)
	useFakeS3(t, true)

	p := &defaultProvisioner{}
	var err error
	out := captureStdout(t, func() {
		err = p.ListBackends(context.Background(), &ListBackendsParams{
			AtmosConfig:     &schema.AtmosConfiguration{},
			ComponentConfig: brokenAndHealthyComponentConfig(),
			Format:          "table",
		})
	})
	require.NoError(t, err)
	assert.Contains(t, out, "a-broken")
	assert.Contains(t, out, "b-good")
	assert.NotContains(t, out, "error:")
	assert.Equal(t, 1, calls["a-broken"])
	assert.Equal(t, 1, calls["b-good"])
}

// TestBackendTargetsFailedError verifies the aggregate error names every failed target, counts them, and hints at describe.
func TestBackendTargetsFailedError(t *testing.T) {
	assert.NoError(t, backendTargetsFailedError(2, nil))

	errOther := errors.New("access denied")
	err := backendTargetsFailedError(3, []targetFailure{
		{name: "a", err: errMistypedIdentity},
		{name: "c", err: errOther},
	})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationBackendTargetsFailed)
	require.ErrorIs(t, err, errMistypedIdentity)
	require.ErrorIs(t, err, errOther)
	assert.Contains(t, err.Error(), `target "a"`)
	assert.Contains(t, err.Error(), `target "c"`)
}

// TestExecuteCreateOrUpdate_ResolvesTargetAuthOnce verifies create/update authenticate the target one time,
// whether or not the existence check runs before provisioning.
func TestExecuteCreateOrUpdate_ResolvesTargetAuthOnce(t *testing.T) {
	tests := []struct {
		name        string
		verb        string
		autoApprove bool
		exists      bool
		wantErr     error
		wantCreate  bool
	}{
		{name: "create without auto-approve, bucket missing", verb: verbCreate, exists: false, wantCreate: true},
		{name: "update without auto-approve, bucket missing", verb: verbUpdate, exists: false, wantCreate: true},
		{name: "create without auto-approve, bucket exists stops at the prompt", verb: verbCreate, exists: true, wantErr: errUtils.ErrInteractiveNotAvailable},
		{name: "create with auto-approve, bucket exists", verb: verbCreate, autoApprove: true, exists: true, wantCreate: true},
		{name: "update with auto-approve, bucket missing", verb: verbUpdate, autoApprove: true, exists: false, wantCreate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(ResetDependencies)
			t.Cleanup(backend.ResetRegistryForTesting)
			calls := stubTargetAuth(t)
			useFakeS3(t, tt.exists)

			created := false
			backend.RegisterBackendCreate("s3", func(_ context.Context, _ *schema.AtmosConfiguration, _ map[string]any, _ *schema.AuthContext) (*backend.ProvisionResult, error) {
				created = true
				return &backend.ProvisionResult{}, nil
			})

			ctrl := gomock.NewController(t)
			mockCI := NewMockConfigInitializer(ctrl)
			atmosConfig := &schema.AtmosConfiguration{}
			info := &schema.ConfigAndStacksInfo{}
			mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(atmosConfig, info, nil)
			mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(singleS3TargetComponentConfig(), nil)
			SetConfigInitializer(mockCI)
			SetProvisioner(&defaultProvisioner{})

			err := executeCreateOrUpdate(t.Context(), createOrUpdateArgs{
				Verb: tt.verb, Component: "vpc", Stack: "dev", AutoApprove: tt.autoApprove,
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCreate, created)
			assert.Equal(t, 1, calls["artifacts"], "target auth must be resolved exactly once")
		})
	}
}

// TestResolveBackendTargetAuth_Caching verifies the per-params cache is scoped to one target on one params value,
// and that a failed resolution is retried rather than cached.
func TestResolveBackendTargetAuth_Caching(t *testing.T) {
	newParams := func() *CreateBackendParams {
		return &CreateBackendParams{
			AtmosConfig: &schema.AtmosConfiguration{}, Component: "vpc", Stack: "dev",
			ComponentConfig: brokenAndHealthyComponentConfig(),
		}
	}

	t.Run("same params and target resolve once", func(t *testing.T) {
		calls := stubTargetAuth(t)
		params := newParams()
		first, err := resolveBackendTargetAuth(params, "b-good")
		require.NoError(t, err)
		second, err := resolveBackendTargetAuth(params, "b-good")
		require.NoError(t, err)
		assert.Same(t, first, second)
		assert.Equal(t, 1, calls["b-good"])
	})

	t.Run("different targets and fresh params resolve independently", func(t *testing.T) {
		calls := stubTargetAuth(t)
		params := newParams()
		_, err := resolveBackendTargetAuth(params, "a-broken")
		require.NoError(t, err)
		_, err = resolveBackendTargetAuth(params, "b-good")
		require.NoError(t, err)
		_, err = resolveBackendTargetAuth(newParams(), "b-good")
		require.NoError(t, err)
		assert.Equal(t, 1, calls["a-broken"])
		assert.Equal(t, 2, calls["b-good"])
	})

	t.Run("failures are not cached", func(t *testing.T) {
		calls := stubTargetAuth(t, "a-broken")
		params := newParams()
		for range 2 {
			_, err := resolveBackendTargetAuth(params, "a-broken")
			require.ErrorIs(t, err, errMistypedIdentity)
		}
		assert.Equal(t, 2, calls["a-broken"])
	})
}
