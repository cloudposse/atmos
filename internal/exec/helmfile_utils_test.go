package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	mockTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckHelmfileConfig(t *testing.T) {
	tests := []struct {
		name          string
		atmosConfig   schema.AtmosConfiguration
		expectedError error
	}{
		{
			name: "valid config without UseEKS",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "/path/to/helmfile/components",
						UseEKS:   false,
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "valid config with UseEKS and deprecated patterns",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                true,
						KubeconfigPath:        "/path/to/kubeconfig",
						HelmAwsProfilePattern: "cp-{namespace}-{tenant}-gbl-{stage}-helm",
						ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "valid config with UseEKS and new template",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:            "/path/to/helmfile/components",
						UseEKS:              true,
						KubeconfigPath:      "/path/to/kubeconfig",
						ClusterNameTemplate: "{{ .vars.namespace }}-{{ .vars.stage }}-eks",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "valid config with UseEKS and explicit cluster name",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:       "/path/to/helmfile/components",
						UseEKS:         true,
						KubeconfigPath: "/path/to/kubeconfig",
						ClusterName:    "my-eks-cluster",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "missing BasePath",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						UseEKS: false,
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileBasePath,
		},
		{
			name: "empty BasePath",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "",
						UseEKS:   false,
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileBasePath,
		},
		{
			name: "UseEKS true but missing KubeconfigPath",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                true,
						KubeconfigPath:        "",
						HelmAwsProfilePattern: "cp-{namespace}-{tenant}-gbl-{stage}-helm",
						ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileKubeconfigPath,
		},
		{
			name: "UseEKS false with missing EKS-specific fields (should pass)",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                false,
						KubeconfigPath:        "",
						HelmAwsProfilePattern: "",
						ClusterNamePattern:    "",
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "UseEKS true with all fields missing except BasePath - only KubeconfigPath validated at config time",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                true,
						KubeconfigPath:        "",
						HelmAwsProfilePattern: "",
						ClusterNamePattern:    "",
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileKubeconfigPath,
		},
		{
			name: "UseEKS true without cluster name or AWS profile - passes config validation (runtime validates these)",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:       "/path/to/helmfile/components",
						UseEKS:         true,
						KubeconfigPath: "/path/to/kubeconfig",
						// No ClusterName, ClusterNameTemplate, ClusterNamePattern, or HelmAwsProfilePattern.
						// These are validated at runtime since they can be provided via CLI flags.
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkHelmfileConfig(&tt.atmosConfig)

			if tt.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			}
		})
	}
}

func BenchmarkCheckHelmfileConfig(b *testing.B) {
	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				BasePath:            "/path/to/helmfile/components",
				UseEKS:              true,
				KubeconfigPath:      "/path/to/kubeconfig",
				ClusterNameTemplate: "{{ .vars.namespace }}-{{ .vars.stage }}-eks",
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = checkHelmfileConfig(&atmosConfig)
	}
}

func TestPrepareHelmfileAuthEnvironment(t *testing.T) {
	baseEnv := []string{"PATH=/bin"}

	t.Run("nil manager returns original env", func(t *testing.T) {
		got, err := prepareHelmfileAuthEnvironment(nil, "dev", baseEnv)
		require.NoError(t, err)
		assert.Equal(t, baseEnv, got)
	})

	t.Run("disabled identity returns original env", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)

		got, err := prepareHelmfileAuthEnvironment(manager, cfg.IdentityFlagDisabledValue, baseEnv)
		require.NoError(t, err)
		assert.Equal(t, baseEnv, got)
	})

	t.Run("explicit identity prepares env", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)
		prepared := []string{"PATH=/bin", "AWS_PROFILE=dev"}
		manager.EXPECT().
			PrepareShellEnvironment(gomock.Any(), "dev", baseEnv).
			Return(prepared, nil)

		got, err := prepareHelmfileAuthEnvironment(manager, "dev", baseEnv)
		require.NoError(t, err)
		assert.Equal(t, prepared, got)
	})

	t.Run("empty identity uses default identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)
		prepared := []string{"PATH=/bin", "AWS_PROFILE=default"}
		manager.EXPECT().GetDefaultIdentity(false).Return("default", nil)
		manager.EXPECT().
			PrepareShellEnvironment(gomock.Any(), "default", baseEnv).
			Return(prepared, nil)

		got, err := prepareHelmfileAuthEnvironment(manager, "", baseEnv)
		require.NoError(t, err)
		assert.Equal(t, prepared, got)
	})

	t.Run("empty identity without default keeps original env", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)
		manager.EXPECT().GetDefaultIdentity(false).Return("", errors.New("no default"))

		got, err := prepareHelmfileAuthEnvironment(manager, "", baseEnv)
		require.NoError(t, err)
		assert.Equal(t, baseEnv, got)
	})

	t.Run("select identity requires default identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)
		manager.EXPECT().GetDefaultIdentity(false).Return("", errors.New("no default"))

		got, err := prepareHelmfileAuthEnvironment(manager, cfg.IdentityFlagSelectValue, baseEnv)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	})

	t.Run("prepare failure is wrapped", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		manager := mockTypes.NewMockAuthManager(ctrl)
		prepareErr := errors.New("prepare failed")
		manager.EXPECT().
			PrepareShellEnvironment(gomock.Any(), "dev", baseEnv).
			Return(nil, prepareErr)

		got, err := prepareHelmfileAuthEnvironment(manager, "dev", baseEnv)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		assert.ErrorIs(t, err, prepareErr)
	})
}
