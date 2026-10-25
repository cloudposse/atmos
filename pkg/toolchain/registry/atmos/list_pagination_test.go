package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestInlineRegistryListPagination(t *testing.T) {
	reg := &AtmosRegistry{tools: map[string]*registry.Tool{
		"z/last":   {RepoOwner: "z", RepoName: "last"},
		"a/first":  {RepoOwner: "a", RepoName: "first"},
		"a/second": {RepoOwner: "a", RepoName: "second"},
	}}
	for range 2 {
		page, err := reg.ListAll(context.Background(), registry.WithListOffset(1), registry.WithListLimit(1))
		require.NoError(t, err)
		require.Len(t, page, 1)
		require.Equal(t, "second", page[0].RepoName)
	}
	empty, err := reg.ListAll(context.Background(), registry.WithListOffset(100))
	require.NoError(t, err)
	require.Empty(t, empty)
	all, err := reg.ListAll(context.Background(), registry.WithListOffset(-1))
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "first", all[0].RepoName)
	require.Equal(t, "last", all[2].RepoName)
}
