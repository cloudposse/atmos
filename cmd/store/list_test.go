package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pstore "github.com/cloudposse/atmos/pkg/store"
)

func TestDescriptorsToRows(t *testing.T) {
	rows := descriptorsToRows([]pstore.Descriptor{
		{Name: "app-metadata", Kind: pstore.KindAWSSSM, Secret: false, Deletable: true, HasStatus: true, Local: false},
	})
	require.Len(t, rows, 1)
	assert.Equal(t, "app-metadata", rows[0]["name"])
	assert.Equal(t, pstore.KindAWSSSM, rows[0]["kind"])
	assert.Equal(t, true, rows[0]["deletable"])
}

func TestRunStoreList(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.descs = []pstore.Descriptor{
		{Name: "app-metadata", Kind: pstore.KindAWSSSM},
		{Name: "vault", Kind: pstore.KindHashicorpVault, Secret: true},
	}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "--format", "json")
	require.NoError(t, err)
}

func TestRunStoreList_Empty(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list")
	require.NoError(t, err)
}

func TestRunStoreList_ServiceError(t *testing.T) {
	setupIO(t)
	installService(t, nil, assert.AnError)

	err := runStoreSubcommand(t, "list")
	require.ErrorIs(t, err, assert.AnError)
}
