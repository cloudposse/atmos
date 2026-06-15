package providers

import (
	"context"
	"errors"
	"testing"

	onepassword "github.com/1password/onepassword-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// fakeOPSDK is an in-memory opSDKAPI for testing sdkClient without the WASM-backed native client.
// It mirrors fakeConnectClient: a vault/item model plus injectable error fields so failure paths
// can be exercised deterministically.
type fakeOPSDK struct {
	// resolveVal maps a secret reference to its value (read by Resolve).
	resolveVal map[string]string
	// vaults is returned verbatim by ListVaults.
	vaults []onepassword.VaultOverview
	// items maps a vault ID to the overviews returned by ListItems.
	items map[string][]onepassword.ItemOverview
	// store maps "vaultID/itemID" to the full item (read by GetItem, written by PutItem).
	store map[string]onepassword.Item

	resolveErr    error
	listVaultsErr error
	listItemsErr  error
	getErr        error
	putErr        error
	createErr     error
	deleteErr     error

	lastCreated *onepassword.ItemCreateParams
	lastPut     *onepassword.Item
	lastDeleted string
}

func newFakeOPSDK() *fakeOPSDK {
	return &fakeOPSDK{
		resolveVal: map[string]string{},
		items:      map[string][]onepassword.ItemOverview{},
		store:      map[string]onepassword.Item{},
	}
}

func (f *fakeOPSDK) key(vaultID, itemID string) string { return vaultID + "/" + itemID }

func (f *fakeOPSDK) Resolve(_ context.Context, reference string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	v, ok := f.resolveVal[reference]
	if !ok {
		return "", errors.New("secret not found")
	}
	return v, nil
}

func (f *fakeOPSDK) ListVaults(_ context.Context) ([]onepassword.VaultOverview, error) {
	if f.listVaultsErr != nil {
		return nil, f.listVaultsErr
	}
	return f.vaults, nil
}

func (f *fakeOPSDK) ListItems(_ context.Context, vaultID string) ([]onepassword.ItemOverview, error) {
	if f.listItemsErr != nil {
		return nil, f.listItemsErr
	}
	return f.items[vaultID], nil
}

func (f *fakeOPSDK) GetItem(_ context.Context, vaultID, itemID string) (onepassword.Item, error) {
	if f.getErr != nil {
		return onepassword.Item{}, f.getErr
	}
	it, ok := f.store[f.key(vaultID, itemID)]
	if !ok {
		return onepassword.Item{}, errors.New("item not found")
	}
	return it, nil
}

func (f *fakeOPSDK) PutItem(_ context.Context, item *onepassword.Item) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.lastPut = item
	f.store[f.key(item.VaultID, item.ID)] = *item
	return nil
}

func (f *fakeOPSDK) CreateItem(_ context.Context, params *onepassword.ItemCreateParams) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.lastCreated = params
	return nil
}

func (f *fakeOPSDK) DeleteItem(_ context.Context, vaultID, itemID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.lastDeleted = f.key(vaultID, itemID)
	delete(f.store, f.key(vaultID, itemID))
	return nil
}

// seedVaultItem registers a vault ("Shared"/"v1") containing an item ("Datadog"/"i1") with the
// given fields, wired so ListVaults/ListItems/GetItem all resolve consistently.
func (f *fakeOPSDK) seedVaultItem(fields ...onepassword.ItemField) {
	f.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}
	f.items["v1"] = []onepassword.ItemOverview{{ID: "i1", Title: "Datadog", VaultID: "v1"}}
	f.store[f.key("v1", "i1")] = onepassword.Item{ID: "i1", VaultID: "v1", Title: "Datadog", Fields: fields}
}

func newTestSDKClient(api opSDKAPI) *sdkClient {
	return &sdkClient{api: api}
}

func concealed(title, value string) onepassword.ItemField {
	return onepassword.ItemField{Title: title, FieldType: onepassword.ItemFieldTypeConcealed, Value: value}
}

func TestSDKClient_Resolve(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.resolveVal["op://Shared/Datadog/api_key"] = "dd-key"
		got, err := newTestSDKClient(fake).Resolve(context.Background(), "op://Shared/Datadog/api_key")
		require.NoError(t, err)
		assert.Equal(t, "dd-key", got)
	})

	t.Run("not found maps to sentinel", func(t *testing.T) {
		_, err := newTestSDKClient(newFakeOPSDK()).Resolve(context.Background(), "op://Shared/Datadog/api_key")
		assert.ErrorIs(t, err, store.ErrOnePasswordNotFound)
	})

	t.Run("transport error propagates", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.resolveErr = errors.New("401 unauthorized")
		_, err := newTestSDKClient(fake).Resolve(context.Background(), "op://Shared/Datadog/api_key")
		require.Error(t, err)
		assert.NotErrorIs(t, err, store.ErrOnePasswordNotFound)
	})
}

func TestSDKClient_Set_UpdatesExistingField(t *testing.T) {
	fake := newFakeOPSDK()
	fake.seedVaultItem(concealed("api_key", "old"))

	require.NoError(t, newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "new"))
	require.NotNil(t, fake.lastPut)
	require.Len(t, fake.lastPut.Fields, 1)
	assert.Equal(t, "new", fake.lastPut.Fields[0].Value)
}

func TestSDKClient_Set_AppendsNewField(t *testing.T) {
	fake := newFakeOPSDK()
	fake.seedVaultItem(concealed("existing", "x"))

	require.NoError(t, newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "new"))
	require.NotNil(t, fake.lastPut)
	require.Len(t, fake.lastPut.Fields, 2)
	assert.Equal(t, "existing", fake.lastPut.Fields[0].Title)
	assert.Equal(t, "api_key", fake.lastPut.Fields[1].Title)
	assert.Equal(t, "new", fake.lastPut.Fields[1].Value)
}

func TestSDKClient_Set_CreatesItemWhenMissing(t *testing.T) {
	fake := newFakeOPSDK()
	// Vault exists but holds no items, so the item must be created.
	fake.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}

	require.NoError(t, newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "dd-key"))
	require.NotNil(t, fake.lastCreated)
	assert.Equal(t, "v1", fake.lastCreated.VaultID)
	assert.Equal(t, "Datadog", fake.lastCreated.Title)
	assert.Equal(t, onepassword.ItemCategoryAPICredentials, fake.lastCreated.Category)
	require.Len(t, fake.lastCreated.Fields, 1)
	assert.Equal(t, "api_key", fake.lastCreated.Fields[0].Title)
	assert.Equal(t, "dd-key", fake.lastCreated.Fields[0].Value)
}

func TestSDKClient_Set_VaultNotFound(t *testing.T) {
	err := newTestSDKClient(newFakeOPSDK()).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
	assert.ErrorIs(t, err, store.ErrOnePasswordNotFound)
}

func TestSDKClient_Set_VaultByID(t *testing.T) {
	fake := newFakeOPSDK()
	fake.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}
	// Reference the vault by its UUID rather than title.
	require.NoError(t, newTestSDKClient(fake).Set(context.Background(), "op://v1/Datadog/api_key", "k"))
	require.NotNil(t, fake.lastCreated)
	assert.Equal(t, "v1", fake.lastCreated.VaultID)
}

func TestSDKClient_Set_ListErrorsPropagate(t *testing.T) {
	t.Run("vault list error", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.listVaultsErr = errors.New("boom")
		err := newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
		require.Error(t, err)
		assert.NotErrorIs(t, err, store.ErrOnePasswordNotFound)
	})

	t.Run("item list error", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}
		fake.listItemsErr = errors.New("boom")
		err := newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
		require.Error(t, err)
	})
}

func TestSDKClient_Set_CreateAndUpdateErrorsPropagate(t *testing.T) {
	t.Run("create error", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}
		fake.createErr = errors.New("create failed")
		err := newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
		require.Error(t, err)
	})

	t.Run("get error during update", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("api_key", "old"))
		fake.getErr = errors.New("get failed")
		err := newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
		require.Error(t, err)
	})

	t.Run("put error during update", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("api_key", "old"))
		fake.putErr = errors.New("put failed")
		err := newTestSDKClient(fake).Set(context.Background(), "op://Shared/Datadog/api_key", "v")
		require.Error(t, err)
	})
}

func TestSDKClient_Set_InvalidReference(t *testing.T) {
	err := newTestSDKClient(newFakeOPSDK()).Set(context.Background(), "op://Shared/Datadog", "v")
	assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
}

func TestSDKClient_Delete(t *testing.T) {
	t.Run("removes one field, keeps item", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("api_key", "k"), concealed("app_key", "a"))

		require.NoError(t, newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key"))
		require.NotNil(t, fake.lastPut)
		require.Len(t, fake.lastPut.Fields, 1)
		assert.Equal(t, "app_key", fake.lastPut.Fields[0].Title)
		assert.Empty(t, fake.lastDeleted)
	})

	t.Run("deletes item when last field removed", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("api_key", "k"))

		require.NoError(t, newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key"))
		assert.Equal(t, fake.key("v1", "i1"), fake.lastDeleted)
		_, ok := fake.store[fake.key("v1", "i1")]
		assert.False(t, ok)
	})

	t.Run("missing vault is idempotent", func(t *testing.T) {
		assert.NoError(t, newTestSDKClient(newFakeOPSDK()).Delete(context.Background(), "op://Shared/Datadog/api_key"))
	})

	t.Run("missing item is idempotent", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.vaults = []onepassword.VaultOverview{{ID: "v1", Title: "Shared"}}
		assert.NoError(t, newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key"))
	})

	t.Run("missing field is idempotent", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("other", "x"))

		require.NoError(t, newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key"))
		assert.Nil(t, fake.lastPut)
		assert.Empty(t, fake.lastDeleted)
	})

	t.Run("get error propagates", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.seedVaultItem(concealed("api_key", "k"))
		fake.getErr = errors.New("get failed")
		err := newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key")
		require.Error(t, err)
	})

	t.Run("vault list error propagates", func(t *testing.T) {
		fake := newFakeOPSDK()
		fake.listVaultsErr = errors.New("boom")
		err := newTestSDKClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key")
		require.Error(t, err)
	})

	t.Run("invalid reference", func(t *testing.T) {
		err := newTestSDKClient(newFakeOPSDK()).Delete(context.Background(), "op://Shared/Datadog")
		assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
	})
}

func TestSDKClient_ResolveItemByID(t *testing.T) {
	fake := newFakeOPSDK()
	fake.seedVaultItem(concealed("api_key", "old"))
	// Address the item by its UUID; resolveItemID must match on ID.
	require.NoError(t, newTestSDKClient(fake).Set(context.Background(), "op://Shared/i1/api_key", "new"))
	require.NotNil(t, fake.lastPut)
	assert.Equal(t, "new", fake.lastPut.Fields[0].Value)
}
