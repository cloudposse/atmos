package terraform_backend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveBackendProfileOverlay verifies that the S3 backend `profile`
// attribute is honored the way `terraform init` honors it, without overriding
// an Atmos auth context or an explicit `env.AWS_PROFILE`.
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

	t.Run("backend profile becomes AWS_PROFILE when nothing else selects credentials", func(t *testing.T) {
		got := tb.ResolveBackendProfileOverlay(&backendWithProfile, nil, nil)
		assert.Equal(t, map[string]string{"AWS_PROFILE": "labs:tofu_admin"}, got)
	})

	t.Run("no backend profile keeps the default credential chain", func(t *testing.T) {
		assert.Nil(t, tb.ResolveBackendProfileOverlay(&backendWithoutProfile, nil, nil))
	})

	t.Run("explicit env AWS_PROFILE wins over the backend profile", func(t *testing.T) {
		overlay := map[string]string{"AWS_PROFILE": "from-env"}
		got := tb.ResolveBackendProfileOverlay(&backendWithProfile, nil, overlay)
		assert.Equal(t, map[string]string{"AWS_PROFILE": "from-env"}, got)
	})

	t.Run("backend profile is merged into an env overlay that lacks AWS_PROFILE", func(t *testing.T) {
		overlay := map[string]string{"AWS_REGION": "eu-west-1"}
		got := tb.ResolveBackendProfileOverlay(&backendWithProfile, nil, overlay)
		assert.Equal(t, map[string]string{"AWS_REGION": "eu-west-1", "AWS_PROFILE": "labs:tofu_admin"}, got)
		assert.Equal(t, map[string]string{"AWS_REGION": "eu-west-1"}, overlay, "input overlay must not be mutated")
	})

	t.Run("atmos auth AWS context is canonical and untouched", func(t *testing.T) {
		auth := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "atmos-managed"}}
		assert.Nil(t, tb.ResolveBackendProfileOverlay(&backendWithProfile, auth, nil))
	})

	t.Run("auth context without an AWS section still falls back to the backend profile", func(t *testing.T) {
		auth := &schema.AuthContext{}
		got := tb.ResolveBackendProfileOverlay(&backendWithProfile, auth, nil)
		assert.Equal(t, map[string]string{"AWS_PROFILE": "labs:tofu_admin"}, got)
	})
}
