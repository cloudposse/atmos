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
