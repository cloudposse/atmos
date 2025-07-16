package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestGetComponentRemoteStateBackendStaticType(t *testing.T) {
	tests := []struct {
		name     string
		sections map[string]any
		want     map[string]any
	}{
		{
			name: "valid static backend with config",
			sections: map[string]any{
				cfg.RemoteStateBackendTypeSectionName: "static",
				cfg.RemoteStateBackendSectionName: map[string]any{
					"bucket": "my-bucket",
					"key":    "path/to/state",
				},
			},
			want: map[string]any{
				"bucket": "my-bucket",
				"key":    "path/to/state",
			},
		},
		{
			name: "non-static backend type",
			sections: map[string]any{
				cfg.RemoteStateBackendTypeSectionName: "s3",
				cfg.RemoteStateBackendSectionName: map[string]any{
					"bucket": "my-bucket",
				},
			},
			want: nil,
		},
		{
			name: "missing backend type",
			sections: map[string]any{
				cfg.RemoteStateBackendSectionName: map[string]any{
					"bucket": "my-bucket",
				},
			},
			want: nil,
		},
		{
			name: "missing backend config",
			sections: map[string]any{
				cfg.RemoteStateBackendTypeSectionName: "static",
			},
			want: nil,
		},
		{
			name: "invalid backend type (not string)",
			sections: map[string]any{
				cfg.RemoteStateBackendTypeSectionName: 123,
				cfg.RemoteStateBackendSectionName: map[string]any{
					"bucket": "my-bucket",
				},
			},
			want: nil,
		},
		{
			name: "invalid backend config (not map)",
			sections: map[string]any{
				cfg.RemoteStateBackendTypeSectionName: "static",
				cfg.RemoteStateBackendSectionName:     "invalid",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetComponentRemoteStateBackendStaticType(tt.sections)
			assert.Equal(t, tt.want, got)
		})
	}
}
