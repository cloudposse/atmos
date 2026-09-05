package terraform_backend_test

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveBackendProfileOverlay verifies that the S3 backend `profile`
// attribute is honored the way `terraform init` honors it, without overriding
// an Atmos auth context or an explicit `env.AWS_PROFILE`, and without mutating
// the caller's overlay.
func TestResolveBackendProfileOverlay(t *testing.T) {
	backendWithProfile := map[string]any{
		"bucket":  "my-tfstate",
		"region":  "us-west-2",
		"profile": "labs:tofu_admin",
	}
	backendWithoutProfile := map[string]any{
		"bucket": "my-tfstate",
		"region": "us-west-2",
	}

	tests := []struct {
		name        string
		backend     map[string]any
		authContext *schema.AuthContext
		envOverlay  map[string]string
		want        map[string]string
	}{
		{
			name:    "backend profile becomes AWS_PROFILE when nothing else selects credentials",
			backend: backendWithProfile,
			want:    map[string]string{"AWS_PROFILE": "labs:tofu_admin"},
		},
		{
			name:    "no backend profile keeps the default credential chain",
			backend: backendWithoutProfile,
			want:    nil,
		},
		{
			name:       "explicit env AWS_PROFILE wins over the backend profile",
			backend:    backendWithProfile,
			envOverlay: map[string]string{"AWS_PROFILE": "from-env"},
			want:       map[string]string{"AWS_PROFILE": "from-env"},
		},
		{
			name:       "backend profile is merged into an env overlay that lacks AWS_PROFILE",
			backend:    backendWithProfile,
			envOverlay: map[string]string{"AWS_REGION": "eu-west-1"},
			want:       map[string]string{"AWS_REGION": "eu-west-1", "AWS_PROFILE": "labs:tofu_admin"},
		},
		{
			name:        "atmos auth AWS context is canonical and untouched",
			backend:     backendWithProfile,
			authContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "atmos-managed"}},
			want:        nil,
		},
		{
			name:        "auth context without an AWS section still falls back to the backend profile",
			backend:     backendWithProfile,
			authContext: &schema.AuthContext{},
			want:        map[string]string{"AWS_PROFILE": "labs:tofu_admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := maps.Clone(tt.envOverlay)

			got := tb.ResolveBackendProfileOverlay(&tt.backend, tt.authContext, tt.envOverlay)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, before, tt.envOverlay, "input overlay must not be mutated")
		})
	}
}
