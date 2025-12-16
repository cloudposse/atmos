package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestTerraformFlags(t *testing.T) {
	registry := TerraformFlags()

	// Should have common flags (stack, dry-run) + Terraform-specific flags including identity.
	// Note: from-plan is defined in apply.go and deploy.go with NoOptDefVal, not here.
	assert.GreaterOrEqual(t, registry.Count(), 12)

	// Should include common flags.
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("dry-run"))

	// Should include identity flag for terraform commands.
	assert.True(t, registry.Has("identity"), "identity should be in TerraformFlags for terraform commands")

	// Should include Terraform-specific flags.
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("skip-init"))
	// Note: from-plan is not in shared TerraformFlags - it's defined in apply.go/deploy.go.
	assert.True(t, registry.Has("init-pass-vars"))
	assert.True(t, registry.Has("append-user-agent"))
	assert.True(t, registry.Has("process-templates"))
	assert.True(t, registry.Has("process-functions"))
	assert.True(t, registry.Has("skip"))
	assert.True(t, registry.Has("query"))
	assert.True(t, registry.Has("components"))

	// Check upload-status flag.
	uploadFlag := registry.Get("upload-status")
	require.NotNil(t, uploadFlag)
	boolFlag, ok := uploadFlag.(*flags.BoolFlag)
	require.True(t, ok)
	assert.Equal(t, false, boolFlag.Default)
}

func TestTerraformAffectedFlags(t *testing.T) {
	registry := TerraformAffectedFlags()

	// Should have 7 affected flags.
	assert.Equal(t, 7, registry.Count())

	// Should include all affected flags.
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("sha"))
	assert.True(t, registry.Has("ssh-key"))
	assert.True(t, registry.Has("ssh-key-password"))
	assert.True(t, registry.Has("include-dependents"))
	assert.True(t, registry.Has("clone-target-ref"))

	// Check repo-path flag.
	repoPathFlag := registry.Get("repo-path")
	require.NotNil(t, repoPathFlag)
	strFlag, ok := repoPathFlag.(*flags.StringFlag)
	require.True(t, ok)
	assert.Equal(t, "", strFlag.Default)
	assert.Equal(t, []string{"ATMOS_REPO_PATH"}, strFlag.EnvVars)
}

func TestWithTerraformFlags(t *testing.T) {
	// Create a standard parser with terraform flags.
	parser := flags.NewStandardParser(
		WithTerraformFlags(),
	)

	registry := parser.Registry()

	// Should have all terraform flags.
	assert.GreaterOrEqual(t, registry.Count(), 12)
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))
}

func TestWithTerraformAffectedFlags(t *testing.T) {
	// Create a standard parser with affected flags.
	parser := flags.NewStandardParser(
		WithTerraformAffectedFlags(),
	)

	registry := parser.Registry()

	// Should have all affected flags.
	assert.Equal(t, 7, registry.Count())
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("include-dependents"))
}

func TestCombinedTerraformFlags(t *testing.T) {
	// Create a standard parser with both terraform and affected flags.
	parser := flags.NewStandardParser(
		WithTerraformFlags(),
		WithTerraformAffectedFlags(),
	)

	registry := parser.Registry()

	// Should have all flags from both registries.
	assert.GreaterOrEqual(t, registry.Count(), 19)

	// Should include terraform flags.
	assert.True(t, registry.Has("stack"))
	assert.True(t, registry.Has("upload-status"))
	assert.True(t, registry.Has("identity"))

	// Should include affected flags.
	assert.True(t, registry.Has("repo-path"))
	assert.True(t, registry.Has("ref"))
	assert.True(t, registry.Has("include-dependents"))
}
