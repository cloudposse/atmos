package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
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
			name: "valid config with UseEKS",
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
			name: "UseEKS true but missing HelmAwsProfilePattern",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                true,
						KubeconfigPath:        "/path/to/kubeconfig",
						HelmAwsProfilePattern: "",
						ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileAwsProfilePattern,
		},
		{
			name: "UseEKS true but missing ClusterNamePattern",
			atmosConfig: schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath:              "/path/to/helmfile/components",
						UseEKS:                true,
						KubeconfigPath:        "/path/to/kubeconfig",
						HelmAwsProfilePattern: "cp-{namespace}-{tenant}-gbl-{stage}-helm",
						ClusterNamePattern:    "",
					},
				},
			},
			expectedError: errUtils.ErrMissingHelmfileClusterNamePattern,
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
			name: "UseEKS true with all fields missing except BasePath",
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
				BasePath:              "/path/to/helmfile/components",
				UseEKS:                true,
				KubeconfigPath:        "/path/to/kubeconfig",
				HelmAwsProfilePattern: "cp-{namespace}-{tenant}-gbl-{stage}-helm",
				ClusterNamePattern:    "{namespace}-{tenant}-{environment}-{stage}-eks-cluster",
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = checkHelmfileConfig(&atmosConfig)
	}
}
