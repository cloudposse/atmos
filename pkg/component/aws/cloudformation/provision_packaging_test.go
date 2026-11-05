package cloudformation

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
)

func TestResolvePackagingTarget_ExplicitSelectorBeforeImplicitDefault(t *testing.T) {
	provisionSection := map[string]any{"targets": map[string]any{
		"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket", "region": "us-east-1"},
		"deploy":    map[string]any{"kind": "aws/cloudformation"},
	}}
	for _, name := range []string{"artifacts", "missing", "deploy"} {
		t.Run(name, func(t *testing.T) {
			selected := &target.SelectedTarget{
				Kind: "aws/cloudformation", Name: "deploy", Config: map[string]any{"packaging": name},
			}
			cfg, err := resolvePackagingTarget(provisionSection, selected)
			if name == "artifacts" {
				require.NoError(t, err)
				require.Equal(t, "my-bucket", cfg.Bucket)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			require.ErrorContains(t, err, name)
			require.Nil(t, cfg)
		})
	}
}
