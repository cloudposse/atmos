package providers

import (
	"context"
	"errors"
	"testing"

	connect "github.com/1Password/connect-sdk-go/connect"
	connectop "github.com/1Password/connect-sdk-go/onepassword"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// fakeConnectClient is an in-memory connect.Client for testing connectClient. It embeds the
// connect.Client interface so unimplemented methods panic if accidentally used; only the methods
// the store exercises are overridden below.
type fakeConnectClient struct {
	connect.Client

	// items is keyed by "vault/item" and holds the stored item.
	items map[string]*connectop.Item
	// vaultsByID maps a vault UUID query to its resolved UUID (read by GetVault).
	vaultsByID map[string]string
	// vaultsByTitle maps a vault title query to its resolved UUID (read by GetVaultByTitle).
	vaultsByTitle map[string]string

	getErr    error // returned by GetItem when set.
	updateErr error // returned by UpdateItem when set.
	createErr error // returned by CreateItem when set.
	deleteErr error // returned by DeleteItem when set.

	lastCreated *connectop.Item
	lastUpdated *connectop.Item
	lastDeleted *connectop.Item
}

func newFakeConnectClient() *fakeConnectClient {
	return &fakeConnectClient{
		items:         map[string]*connectop.Item{},
		vaultsByID:    map[string]string{},
		vaultsByTitle: map[string]string{},
	}
}

func (f *fakeConnectClient) key(item, vault string) string { return vault + "/" + item }

func (f *fakeConnectClient) GetItem(itemQuery, vaultQuery string) (*connectop.Item, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	it, ok := f.items[f.key(itemQuery, vaultQuery)]
	if !ok {
		return nil, errors.New("item not found")
	}
	return it, nil
}

func (f *fakeConnectClient) CreateItem(item *connectop.Item, vaultQuery string) (*connectop.Item, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.lastCreated = item
	f.items[f.key(item.Title, vaultQuery)] = item
	return item, nil
}

func (f *fakeConnectClient) UpdateItem(item *connectop.Item, vaultQuery string) (*connectop.Item, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.lastUpdated = item
	f.items[f.key(item.Title, vaultQuery)] = item
	return item, nil
}

func (f *fakeConnectClient) DeleteItem(item *connectop.Item, vaultQuery string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.lastDeleted = item
	delete(f.items, f.key(item.Title, vaultQuery))
	return nil
}

func (f *fakeConnectClient) GetVault(uuid string) (*connectop.Vault, error) {
	if id, ok := f.vaultsByID[uuid]; ok {
		return &connectop.Vault{ID: id}, nil
	}
	return nil, errors.New("vault not found")
}

func (f *fakeConnectClient) GetVaultByTitle(title string) (*connectop.Vault, error) {
	if id, ok := f.vaultsByTitle[title]; ok {
		return &connectop.Vault{ID: id}, nil
	}
	return nil, errors.New("vault not found")
}

func newTestConnectClient(fake connect.Client) *connectClient {
	return &connectClient{client: fake}
}

func itemWith(fields ...*connectop.ItemField) *connectop.Item {
	return &connectop.Item{
		Title:    "Datadog",
		Vault:    connectop.ItemVault{ID: "v1"},
		Category: connectop.ApiCredential,
		Fields:   fields,
	}
}

func TestConnectClient_Resolve(t *testing.T) {
	fake := newFakeConnectClient()
	fake.items[fake.key("Datadog", "Shared")] = itemWith(
		&connectop.ItemField{Label: "api_key", Value: "dd-key"},
		&connectop.ItemField{ID: "field-id", Label: "other", Value: "by-id"},
	)
	c := newTestConnectClient(fake)

	t.Run("found by label", func(t *testing.T) {
		got, err := c.Resolve(context.Background(), "op://Shared/Datadog/api_key")
		require.NoError(t, err)
		assert.Equal(t, "dd-key", got)
	})

	t.Run("found by field ID", func(t *testing.T) {
		got, err := c.Resolve(context.Background(), "op://Shared/Datadog/field-id")
		require.NoError(t, err)
		assert.Equal(t, "by-id", got)
	})

	t.Run("field absent returns not found", func(t *testing.T) {
		_, err := c.Resolve(context.Background(), "op://Shared/Datadog/missing")
		assert.ErrorIs(t, err, store.ErrOnePasswordNotFound)
	})

	t.Run("item not found maps to not found", func(t *testing.T) {
		_, err := c.Resolve(context.Background(), "op://Shared/Nope/api_key")
		assert.ErrorIs(t, err, store.ErrOnePasswordNotFound)
	})

	t.Run("invalid reference", func(t *testing.T) {
		_, err := c.Resolve(context.Background(), "op://Shared/Datadog")
		assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
	})

	t.Run("transport error propagates", func(t *testing.T) {
		boom := newFakeConnectClient()
		boom.getErr = errors.New("503 service unavailable")
		_, err := newTestConnectClient(boom).Resolve(context.Background(), "op://Shared/Datadog/api_key")
		require.Error(t, err)
		assert.NotErrorIs(t, err, store.ErrOnePasswordNotFound)
	})
}

func TestConnectClient_Set_UpdatesExistingField(t *testing.T) {
	fake := newFakeConnectClient()
	fake.items[fake.key("Datadog", "Shared")] = itemWith(
		&connectop.ItemField{Label: "api_key", Value: "old"},
	)
	c := newTestConnectClient(fake)

	require.NoError(t, c.Set(context.Background(), "op://Shared/Datadog/api_key", "new"))
	require.NotNil(t, fake.lastUpdated)
	assert.Equal(t, "new", fake.lastUpdated.Fields[0].Value)
}

func TestConnectClient_Set_AppendsNewField(t *testing.T) {
	fake := newFakeConnectClient()
	fake.items[fake.key("Datadog", "Shared")] = itemWith(
		&connectop.ItemField{Label: "existing", Value: "x"},
	)
	c := newTestConnectClient(fake)

	require.NoError(t, c.Set(context.Background(), "op://Shared/Datadog/api_key", "new"))
	require.NotNil(t, fake.lastUpdated)
	require.Len(t, fake.lastUpdated.Fields, 2)
	assert.Equal(t, "existing", fake.lastUpdated.Fields[0].Label)
	assert.Equal(t, "api_key", fake.lastUpdated.Fields[1].Label)
	assert.Equal(t, "new", fake.lastUpdated.Fields[1].Value)
}

func TestConnectClient_Set_CreatesItemWhenMissing(t *testing.T) {
	fake := newFakeConnectClient()
	fake.vaultsByID["Shared"] = "vault-uuid"
	c := newTestConnectClient(fake)

	require.NoError(t, c.Set(context.Background(), "op://Shared/Datadog/api_key", "dd-key"))
	require.NotNil(t, fake.lastCreated)
	assert.Equal(t, "Datadog", fake.lastCreated.Title)
	assert.Equal(t, "vault-uuid", fake.lastCreated.Vault.ID)
	assert.Equal(t, connectop.ApiCredential, fake.lastCreated.Category)
	require.Len(t, fake.lastCreated.Fields, 1)
	assert.Equal(t, "api_key", fake.lastCreated.Fields[0].Label)
	assert.Equal(t, "dd-key", fake.lastCreated.Fields[0].Value)
}

func TestConnectClient_Set_CreateItemVaultResolveFallsBackToTitle(t *testing.T) {
	fake := newFakeConnectClient()
	// GetVault fails (not a UUID) but GetVaultByTitle resolves, forcing the fallback branch.
	fake.vaultsByTitle["Shared"] = "title-uuid"
	c := newTestConnectClient(fake)

	require.NoError(t, c.Set(context.Background(), "op://Shared/Datadog/api_key", "dd-key"))
	require.NotNil(t, fake.lastCreated)
	assert.Equal(t, "title-uuid", fake.lastCreated.Vault.ID)
}

func TestConnectClient_Set_CreateItemVaultUnresolvable(t *testing.T) {
	fake := newFakeConnectClient() // no vaults registered.
	c := newTestConnectClient(fake)

	err := c.Set(context.Background(), "op://Shared/Datadog/api_key", "dd-key")
	assert.ErrorIs(t, err, store.ErrOnePasswordNotFound)
}

func TestConnectClient_Set_TransportErrorPropagates(t *testing.T) {
	fake := newFakeConnectClient()
	fake.getErr = errors.New("500 internal error")
	c := newTestConnectClient(fake)

	err := c.Set(context.Background(), "op://Shared/Datadog/api_key", "v")
	require.Error(t, err)
}

func TestConnectClient_Set_InvalidReference(t *testing.T) {
	c := newTestConnectClient(newFakeConnectClient())
	err := c.Set(context.Background(), "op://Shared/Datadog", "v")
	assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
}

func TestConnectClient_Delete(t *testing.T) {
	t.Run("removes one field, keeps item", func(t *testing.T) {
		fake := newFakeConnectClient()
		fake.items[fake.key("Datadog", "Shared")] = itemWith(
			&connectop.ItemField{Label: "api_key", Value: "k"},
			&connectop.ItemField{Label: "app_key", Value: "a"},
		)
		c := newTestConnectClient(fake)

		require.NoError(t, c.Delete(context.Background(), "op://Shared/Datadog/api_key"))
		require.NotNil(t, fake.lastUpdated)
		require.Len(t, fake.lastUpdated.Fields, 1)
		assert.Equal(t, "app_key", fake.lastUpdated.Fields[0].Label)
		assert.Nil(t, fake.lastDeleted)
	})

	t.Run("deletes item when last field removed", func(t *testing.T) {
		fake := newFakeConnectClient()
		fake.items[fake.key("Datadog", "Shared")] = itemWith(
			&connectop.ItemField{Label: "api_key", Value: "k"},
		)
		c := newTestConnectClient(fake)

		require.NoError(t, c.Delete(context.Background(), "op://Shared/Datadog/api_key"))
		require.NotNil(t, fake.lastDeleted)
		_, ok := fake.items[fake.key("Datadog", "Shared")]
		assert.False(t, ok)
	})

	t.Run("missing item is idempotent", func(t *testing.T) {
		c := newTestConnectClient(newFakeConnectClient())
		assert.NoError(t, c.Delete(context.Background(), "op://Shared/Datadog/api_key"))
	})

	t.Run("missing field is idempotent", func(t *testing.T) {
		fake := newFakeConnectClient()
		fake.items[fake.key("Datadog", "Shared")] = itemWith(
			&connectop.ItemField{Label: "other", Value: "x"},
		)
		c := newTestConnectClient(fake)

		require.NoError(t, c.Delete(context.Background(), "op://Shared/Datadog/api_key"))
		assert.Nil(t, fake.lastUpdated)
		assert.Nil(t, fake.lastDeleted)
	})

	t.Run("transport error propagates", func(t *testing.T) {
		fake := newFakeConnectClient()
		fake.getErr = errors.New("500 internal error")
		err := newTestConnectClient(fake).Delete(context.Background(), "op://Shared/Datadog/api_key")
		require.Error(t, err)
	})

	t.Run("invalid reference", func(t *testing.T) {
		c := newTestConnectClient(newFakeConnectClient())
		err := c.Delete(context.Background(), "op://Shared/Datadog")
		assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
	})
}

func TestFieldMatches(t *testing.T) {
	assert.True(t, fieldMatches("API_KEY", "id-1", "api_key"), "label match is case-insensitive")
	assert.True(t, fieldMatches("other", "API_KEY", "api_key"), "id match is case-insensitive")
	assert.False(t, fieldMatches("other", "id-1", "api_key"))
}

func TestIsOPNotFound(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{message: "item not found", want: true},
		{message: "field foo isn't a field in this item", want: true},
		{message: "no item matching the query", want: true},
		{message: "couldn't find vault", want: true},
		{message: "vault doesn't exist", want: true},
		{message: "no matching items", want: true},
		{message: "401 unauthorized", want: false},
		{message: "connection refused", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			assert.Equal(t, tt.want, isOPNotFound(tt.message))
		})
	}
}
