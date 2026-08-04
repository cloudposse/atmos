package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pstore "github.com/cloudposse/atmos/pkg/store"
)

func TestDescriptorsToRows(t *testing.T) {
	rows := descriptorsToRows([]pstore.Descriptor{
		{Name: "app-metadata", Kind: pstore.KindAWSSSM, Secret: false, Deletable: true, HasStatus: true, Local: false, Listable: true},
	})
	require.Len(t, rows, 1)
	assert.Equal(t, "app-metadata", rows[0]["name"])
	assert.Equal(t, pstore.KindAWSSSM, rows[0]["kind"])
	assert.Equal(t, true, rows[0]["deletable"])
	assert.Equal(t, true, rows[0]["listable"])
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

// TestRunStoreList_DoesNotUseAuthenticatedLoad proves `store list` loads its service via
// loadServiceForListFn, not loadServiceFn -- list only reads store configuration and capability
// metadata, so it must not attempt authentication (which can trigger an unwanted interactive
// identity prompt when a config declares multiple identities with no default).
func TestRunStoreList_DoesNotUseAuthenticatedLoad(t *testing.T) {
	setupIO(t)

	origAuth := loadServiceFn
	loadServiceFn = func(_ storeScope) (storeService, error) {
		t.Fatal("store list must not call loadServiceFn (the authenticated loader)")
		return nil, nil
	}
	t.Cleanup(func() { loadServiceFn = origAuth })

	svc := newFakeStoreService()
	svc.descs = []pstore.Descriptor{{Name: "app-metadata", Kind: pstore.KindAWSSSM}}
	origList := loadServiceForListFn
	loadServiceForListFn = func(_ storeScope) (storeService, error) { return svc, nil }
	t.Cleanup(func() { loadServiceForListFn = origList })

	err := runStoreSubcommand(t, "list")
	require.NoError(t, err)
}

func TestRunStoreListKeyValues(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.keyValues = []pstore.KeyValue{
		{Key: "image_tag", Value: "v1.2.3"},
		{Key: "region", Value: "us-east-1"},
	}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata", "--stack", "dev", "--component", "vpc", "--format", "json")
	require.NoError(t, err)

	require.Len(t, svc.listKeyValuesCalls, 1)
	assert.Equal(t, "app-metadata", svc.listKeyValuesCalls[0].name)
	assert.Equal(t, "dev", svc.listKeyValuesCalls[0].stack)
	assert.Equal(t, "vpc", svc.listKeyValuesCalls[0].component)
	assert.Contains(t, stdout.String(), "image_tag")
	assert.Contains(t, stdout.String(), "v1.2.3")
}

func TestRunStoreListKeyValues_SecretStoreRegistersMaskedValues(t *testing.T) {
	setupIO(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.secret = true
	svc.keyValues = []pstore.KeyValue{{Key: "password", Value: "hunter2"}}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-secrets", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, []any{"hunter2"}, *registered)
}

func TestRunStoreListKeyValues_NonSecretStoreDoesNotRegisterValues(t *testing.T) {
	setupIO(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.keyValues = []pstore.KeyValue{{Key: "image_tag", Value: "v1.2.3"}}
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata", "--format", "json")
	require.NoError(t, err)
	assert.Empty(t, *registered)
}

func TestRunStoreListKeyValues_Empty(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.NoError(t, err)
}

func TestRunStoreListKeyValues_NotSupported(t *testing.T) {
	setupIO(t)
	svc := newFakeStoreService()
	svc.listKeyValuesErr = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.ErrorIs(t, err, assert.AnError)
}

// TestRunStoreListKeyValues_UsesAuthenticatedLoad proves `store list STORE` loads its service via
// loadServiceFn (the authenticated loader), not loadServiceForListFn -- unlike the bare
// `store list`, listing a store's contents needs real backend access.
func TestRunStoreListKeyValues_UsesAuthenticatedLoad(t *testing.T) {
	setupIO(t)

	origList := loadServiceForListFn
	loadServiceForListFn = func(_ storeScope) (storeService, error) {
		t.Fatal("store list STORE must not call loadServiceForListFn (the credential-free loader)")
		return nil, nil
	}
	t.Cleanup(func() { loadServiceForListFn = origList })

	svc := newFakeStoreService()
	origAuth := loadServiceFn
	loadServiceFn = func(_ storeScope) (storeService, error) { return svc, nil }
	t.Cleanup(func() { loadServiceFn = origAuth })

	err := runStoreSubcommand(t, "list", "app-metadata")
	require.NoError(t, err)
}
