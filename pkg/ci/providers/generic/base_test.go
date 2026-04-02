package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveBase_WithBranchRef(t *testing.T) {
	t.Setenv("ATMOS_CI_BASE_REF", "main")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "refs/remotes/origin/main", res.Ref)
	assert.Empty(t, res.SHA)
	assert.Equal(t, "ATMOS_CI_BASE_REF", res.Source)
}

func TestResolveBase_WithSHA(t *testing.T) {
	t.Setenv("ATMOS_CI_BASE_REF", "abc123def456789012345678901234567890abcd")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Ref)
	assert.Equal(t, "abc123def456789012345678901234567890abcd", res.SHA)
	assert.Equal(t, "ATMOS_CI_BASE_REF", res.Source)
}

func TestResolveBase_NoEnvVar(t *testing.T) {
	t.Setenv("ATMOS_CI_BASE_REF", "")

	p := NewProvider()
	res, err := p.ResolveBase()

	require.NoError(t, err)
	assert.Nil(t, res)
}
