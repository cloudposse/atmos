package cloudformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
)

func TestFindS3Targets(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
			"aws":       map[string]any{"kind": "aws/cloudformation"},
			"gitops":    map[string]any{"kind": "git"},
		},
	}

	s3Targets := findS3Targets(provisionSection)
	require.Len(t, s3Targets, 1)
	assert.Equal(t, "my-bucket", s3Targets["artifacts"]["bucket"])
}

func TestFindS3Targets_NoTargets(t *testing.T) {
	assert.Empty(t, findS3Targets(nil))
	assert.Empty(t, findS3Targets(map[string]any{}))
}

func TestS3ConfigFromTarget(t *testing.T) {
	cfg, err := s3ConfigFromTarget("artifacts", map[string]any{
		"bucket": "my-bucket",
		"prefix": "vpc/",
		"region": "us-east-2",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
	assert.Equal(t, "vpc/", cfg.Prefix)
	assert.Equal(t, "us-east-2", cfg.Region)
}

func TestS3ConfigFromTarget_MissingBucket(t *testing.T) {
	_, err := s3ConfigFromTarget("artifacts", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestResolvePackagingTarget_DirectS3Selection(t *testing.T) {
	selected := &target.SelectedTarget{
		Kind:   "aws/s3",
		Name:   "artifacts",
		Config: map[string]any{"bucket": "my-bucket"},
	}
	cfg, err := resolvePackagingTarget(nil, selected)
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
}

func TestResolvePackagingTarget_NoS3TargetDeclared(t *testing.T) {
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops"}
	_, err := resolvePackagingTarget(nil, selected)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestResolvePackagingTarget_ImplicitSingleS3Target(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"artifacts": map[string]any{"kind": "aws/s3", "bucket": "my-bucket"},
			"gitops":    map[string]any{"kind": "git"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops"}
	cfg, err := resolvePackagingTarget(provisionSection, selected)
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", cfg.Bucket)
}

func TestResolvePackagingTarget_AmbiguousWithoutDisambiguator(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"bucket-a": map[string]any{"kind": "aws/s3", "bucket": "bucket-a"},
			"bucket-b": map[string]any{"kind": "aws/s3", "bucket": "bucket-b"},
			"gitops":   map[string]any{"kind": "git"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops", Config: map[string]any{"kind": "git"}}
	_, err := resolvePackagingTarget(provisionSection, selected)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestResolvePackagingTarget_DisambiguatedWithPackagingField(t *testing.T) {
	provisionSection := map[string]any{
		"targets": map[string]any{
			"bucket-a": map[string]any{"kind": "aws/s3", "bucket": "bucket-a"},
			"bucket-b": map[string]any{"kind": "aws/s3", "bucket": "bucket-b"},
			"gitops":   map[string]any{"kind": "git", "packaging": "bucket-b"},
		},
	}
	selected := &target.SelectedTarget{Kind: "git", Name: "gitops", Config: map[string]any{"kind": "git", "packaging": "bucket-b"}}
	cfg, err := resolvePackagingTarget(provisionSection, selected)
	require.NoError(t, err)
	assert.Equal(t, "bucket-b", cfg.Bucket)
}
