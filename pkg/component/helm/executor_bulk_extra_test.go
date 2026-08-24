package helm

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthManagerForBulk_NoIdentity(t *testing.T) {
	manager, err := authManagerForBulk(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Nil(t, manager)
}

func TestAffectedHelmComponents(t *testing.T) {
	originalCheckout := executeAffectedWithRefCheckout
	t.Cleanup(func() { executeAffectedWithRefCheckout = originalCheckout })

	var gotStack string
	executeAffectedWithRefCheckout = func(_ *schema.AtmosConfiguration, _, _, _ string, _, _ bool, stack string, _, _ bool, _ []string, _ bool, _ auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		gotStack = stack
		return []schema.Affected{
			{Component: "api", Stack: "dev", ComponentType: "helm"},
			{Component: "web", Stack: "dev", ComponentType: "helm"},
		}, nil, nil, "", nil
	}

	ctx := &component.ExecutionContext{Flags: map[string]any{"ref": "feature"}}
	affected, err := affectedHelmComponents(ctx, &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{Stack: "dev"})
	require.NoError(t, err)
	assert.Equal(t, "dev", gotStack)
	require.Len(t, affected, 2)
	assert.Equal(t, "api", affected[0].Component)
	assert.Equal(t, "web", affected[1].Component)
}

func TestAffectedHelmComponents_PropagatesAuthManagerType(t *testing.T) {
	originalCheckout := executeAffectedWithRefCheckout
	t.Cleanup(func() { executeAffectedWithRefCheckout = originalCheckout })

	executeAffectedWithRefCheckout = func(_ *schema.AtmosConfiguration, _, _, _ string, _, _ bool, _ string, _, _ bool, _ []string, _ bool, mgr auth.AuthManager, _ bool) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		// A non-AuthManager value in info.AuthManager must not be forwarded.
		assert.Nil(t, mgr)
		return nil, nil, nil, "", nil
	}

	_, err := affectedHelmComponents(
		&component.ExecutionContext{Flags: map[string]any{}},
		&schema.AtmosConfiguration{},
		&schema.ConfigAndStacksInfo{AuthManager: "not-a-manager"},
	)
	require.NoError(t, err)
}

func TestGenerateArtifacts_NoOp(t *testing.T) {
	require.NoError(t, (&ComponentProvider{}).GenerateArtifacts(&component.ExecutionContext{}))
}
