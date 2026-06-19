package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// fakeOPClient is an in-memory onePasswordClient keyed by fully-rendered op:// reference.
type fakeOPClient struct {
	values map[string]string
	// err, when set, is returned by Resolve regardless of reference (auth/transport failure).
	err error
	// lastReference records the reference passed to the most recent Resolve call.
	lastReference string
}

func newFakeOPClient(values map[string]string) *fakeOPClient {
	return &fakeOPClient{values: values}
}

func (f *fakeOPClient) Resolve(_ context.Context, reference string) (string, error) {
	f.lastReference = reference
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.values[reference]
	if !ok {
		return "", store.ErrOnePasswordNotFound
	}
	return v, nil
}

func (f *fakeOPClient) Set(_ context.Context, reference, value string) error {
	f.lastReference = reference
	if f.err != nil {
		return f.err
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[reference] = value
	return nil
}

func (f *fakeOPClient) Delete(_ context.Context, reference string) error {
	f.lastReference = reference
	if f.err != nil {
		return f.err
	}
	delete(f.values, reference)
	return nil
}

func newTestOPStore(client onePasswordClient, vault string) *OnePasswordStore {
	return &OnePasswordStore{client: client, vault: vault}
}

func TestOnePasswordStore_Get_RendersTemplateAndResolves(t *testing.T) {
	fake := newFakeOPClient(map[string]string{
		"op://prod-ue1/api/password": "s3cr3t",
	})
	s := newTestOPStore(fake, "")

	got, err := s.Get("prod-ue1", "api", "op://{{ .atmos_stack }}/{{ .atmos_component }}/password")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t", got)
	assert.Equal(t, "op://prod-ue1/api/password", fake.lastReference)
}

func TestOnePasswordStore_Get_LiteralReference(t *testing.T) {
	fake := newFakeOPClient(map[string]string{
		"op://Shared/Datadog/api_key": "dd-key",
	})
	s := newTestOPStore(fake, "")

	got, err := s.Get("prod", "api", "op://Shared/Datadog/api_key")
	require.NoError(t, err)
	assert.Equal(t, "dd-key", got)
}

func TestOnePasswordStore_Get_VaultRelativeReference(t *testing.T) {
	fake := newFakeOPClient(map[string]string{
		"op://Production/Datadog/api_key": "dd-key",
	})
	s := newTestOPStore(fake, "Production")

	got, err := s.Get("prod", "api", "Datadog/api_key")
	require.NoError(t, err)
	assert.Equal(t, "dd-key", got)
	assert.Equal(t, "op://Production/Datadog/api_key", fake.lastReference)
}

func TestOnePasswordStore_Get_InvalidReferenceNoVault(t *testing.T) {
	s := newTestOPStore(newFakeOPClient(nil), "")
	_, err := s.Get("prod", "api", "Datadog/api_key")
	assert.ErrorIs(t, err, store.ErrOnePasswordInvalidReference)
}

func TestOnePasswordStore_Get_BadTemplate(t *testing.T) {
	s := newTestOPStore(newFakeOPClient(nil), "")
	_, err := s.Get("prod", "api", "op://{{ .atmos_stack ")
	assert.ErrorIs(t, err, store.ErrOnePasswordReferenceTemplate)
}

func TestOnePasswordStore_Get_UnknownTemplateVar(t *testing.T) {
	// missingkey=error must reject references that interpolate undefined vars.
	s := newTestOPStore(newFakeOPClient(nil), "")
	_, err := s.Get("prod", "api", "op://{{ .nope }}/item/field")
	assert.ErrorIs(t, err, store.ErrOnePasswordReferenceTemplate)
}

func TestOnePasswordStore_Get_EmptyKey(t *testing.T) {
	s := newTestOPStore(newFakeOPClient(nil), "")
	_, err := s.Get("prod", "api", "")
	assert.ErrorIs(t, err, store.ErrEmptyKey)
}

func TestOnePasswordStore_Has(t *testing.T) {
	fake := newFakeOPClient(map[string]string{
		"op://Shared/Datadog/api_key": "dd-key",
	})
	s := newTestOPStore(fake, "")

	has, err := s.Has("prod", "api", "op://Shared/Datadog/api_key")
	require.NoError(t, err)
	assert.True(t, has)

	has, err = s.Has("prod", "api", "op://Shared/Missing/api_key")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestOnePasswordStore_Has_PropagatesAuthError(t *testing.T) {
	authErr := errors.New("401 unauthorized")
	fake := newFakeOPClient(nil)
	fake.err = authErr
	s := newTestOPStore(fake, "")

	_, err := s.Has("prod", "api", "op://Shared/Datadog/api_key")
	require.Error(t, err)
	assert.NotErrorIs(t, err, store.ErrOnePasswordNotFound)
	assert.ErrorIs(t, err, store.ErrOnePasswordResolve)
}

func TestOnePasswordStore_GetKey(t *testing.T) {
	fake := newFakeOPClient(map[string]string{
		"op://Shared/Datadog/api_key": "dd-key",
	})
	s := newTestOPStore(fake, "")

	got, err := s.GetKey("op://Shared/Datadog/api_key")
	require.NoError(t, err)
	assert.Equal(t, "dd-key", got)
}

func TestOnePasswordStore_SetGetDeleteHas(t *testing.T) {
	fake := newFakeOPClient(map[string]string{})
	s := newTestOPStore(fake, "")
	const ref = "op://Shared/Datadog/api_key"

	require.NoError(t, s.Set("prod", "api", ref, "dd-key"))

	got, err := s.Get("prod", "api", ref)
	require.NoError(t, err)
	assert.Equal(t, "dd-key", got)

	has, err := s.Has("prod", "api", ref)
	require.NoError(t, err)
	assert.True(t, has)

	require.NoError(t, s.Delete("prod", "api", ref))

	has, err = s.Has("prod", "api", ref)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestOnePasswordStore_Set_TemplatedReference(t *testing.T) {
	fake := newFakeOPClient(map[string]string{})
	s := newTestOPStore(fake, "")

	require.NoError(t, s.Set("prod-ue1", "api", "op://{{ .atmos_stack }}/postgres/password", "p@ss"))
	assert.Equal(t, "op://prod-ue1/postgres/password", fake.lastReference)
	assert.Equal(t, "p@ss", fake.values["op://prod-ue1/postgres/password"])
}

func TestOnePasswordStore_Set_NilValue(t *testing.T) {
	s := newTestOPStore(newFakeOPClient(map[string]string{}), "")
	assert.ErrorIs(t, s.Set("prod", "api", "op://V/i/f", nil), store.ErrNilValue)
}

func TestOnePasswordStore_Set_NonStringValue(t *testing.T) {
	fake := newFakeOPClient(map[string]string{})
	s := newTestOPStore(fake, "")
	require.NoError(t, s.Set("prod", "api", "op://V/i/f", map[string]any{"a": 1}))
	assert.JSONEq(t, `{"a":1}`, fake.values["op://V/i/f"])
}

func TestOnePasswordStore_Delete_Idempotent(t *testing.T) {
	// Deleting a reference that was never set is not an error.
	s := newTestOPStore(newFakeOPClient(map[string]string{}), "")
	assert.NoError(t, s.Delete("prod", "api", "op://Shared/Missing/api_key"))
}

func TestOnePasswordStore_ImplementsInterfaces(t *testing.T) {
	var s store.Store = newTestOPStore(newFakeOPClient(nil), "")
	_, ok := s.(store.StatusStore)
	assert.True(t, ok)
	// 1Password now supports full CRUD, including deletion.
	_, ok = s.(store.DeletableStore)
	assert.True(t, ok)
}

func TestParseOPReference(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		vault     string
		item      string
		field     string
		wantErr   bool
	}{
		{name: "vault/item/field", reference: "op://Prod/db/password", vault: "Prod", item: "db", field: "password"},
		{name: "with section", reference: "op://Prod/db/conn/password", vault: "Prod", item: "db", field: "password"},
		{name: "too few parts", reference: "op://Prod/db", wantErr: true},
		{name: "too many parts", reference: "op://a/b/c/d/e", wantErr: true},
		{name: "empty segment", reference: "op://Prod//password", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseOPReference(tt.reference)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.vault, ref.vault)
			assert.Equal(t, tt.item, ref.item)
			assert.Equal(t, tt.field, ref.field)
		})
	}
}

func TestNewOnePasswordClient_Selection(t *testing.T) {
	tests := []struct {
		name    string
		opts    OnePasswordStoreOptions
		wantErr bool
		// wantType is the concrete client type expected (empty when wantErr).
		wantConnect bool
		wantSDK     bool
	}{
		{
			name:    "no creds",
			opts:    OnePasswordStoreOptions{},
			wantErr: true,
		},
		{
			name:        "auto prefers connect",
			opts:        OnePasswordStoreOptions{ConnectHost: "https://op.example", ConnectToken: "ct", Token: "sa"},
			wantConnect: true,
		},
		{
			name:    "auto falls back to service account",
			opts:    OnePasswordStoreOptions{Token: "sa"},
			wantSDK: true,
		},
		{
			name:    "explicit connect without creds errors",
			opts:    OnePasswordStoreOptions{Mode: opModeConnect, Token: "sa"},
			wantErr: true,
		},
		{
			name:    "explicit service-account",
			opts:    OnePasswordStoreOptions{Mode: opModeServiceAccount, Token: "sa"},
			wantSDK: true,
		},
		{
			name:        "explicit connect",
			opts:        OnePasswordStoreOptions{Mode: opModeConnect, ConnectHost: "https://op.example", ConnectToken: "ct"},
			wantConnect: true,
		},
		{
			name:    "unknown mode",
			opts:    OnePasswordStoreOptions{Mode: "bogus", Token: "sa"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure ambient OP_* env doesn't leak into selection.
			t.Setenv("OP_CONNECT_HOST", "")
			t.Setenv("OP_CONNECT_TOKEN", "")
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")

			opts := tt.opts
			client, err := newOnePasswordClient(&opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			switch {
			case tt.wantConnect:
				_, ok := client.(*connectClient)
				assert.True(t, ok, "expected connectClient, got %T", client)
			case tt.wantSDK:
				_, ok := client.(*sdkClient)
				assert.True(t, ok, "expected sdkClient, got %T", client)
			}
		})
	}
}

func TestNewOnePasswordClient_EnvFallback(t *testing.T) {
	t.Run("service account token from env", func(t *testing.T) {
		t.Setenv("OP_CONNECT_HOST", "")
		t.Setenv("OP_CONNECT_TOKEN", "")
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "env-sa-token")

		client, err := newOnePasswordClient(&OnePasswordStoreOptions{Mode: opModeServiceAccount})
		require.NoError(t, err)
		_, ok := client.(*sdkClient)
		assert.True(t, ok)
	})

	t.Run("connect credentials from env", func(t *testing.T) {
		t.Setenv("OP_CONNECT_HOST", "https://connect.example")
		t.Setenv("OP_CONNECT_TOKEN", "env-connect-token")
		t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")

		client, err := newOnePasswordClient(&OnePasswordStoreOptions{Mode: opModeConnect})
		require.NoError(t, err)
		_, ok := client.(*connectClient)
		assert.True(t, ok)
	})
}

func TestNewOnePasswordClient_ExplicitConnectDoesNotFallbackToServiceAccount(t *testing.T) {
	t.Setenv("OP_CONNECT_HOST", "")
	t.Setenv("OP_CONNECT_TOKEN", "")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "env-sa-token")

	_, err := newOnePasswordClient(&OnePasswordStoreOptions{Mode: opModeConnect})
	assert.ErrorIs(t, err, store.ErrOnePasswordNoAuth)
}
