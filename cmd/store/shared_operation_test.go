package store

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/providers"
)

func TestStoreServicesKeepVaultIdentityBoundThroughCompleteRead(t *testing.T) {
	firstRead, secondRead, releaseFirst := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var release sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/secret/data/dev/app/first":
			assert.Equal(t, "first-token", r.Header.Get("X-Vault-Token"))
			close(firstRead)
			<-releaseFirst
		case "/v1/secret/data/dev/app/second":
			assert.Equal(t, "second-token", r.Header.Get("X-Vault-Token"))
			close(secondRead)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"read-result"},"metadata":{"version":1}}}`))
	}))
	t.Cleanup(func() {
		release.Do(func() { close(releaseFirst) })
		server.Close()
	})
	backend, err := providers.NewVaultStore(&providers.VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "ambient-token"}, "")
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	manager := authtypes.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{}).AnyTimes()
	for _, identity := range []string{"first", "second"} {
		manager.EXPECT().AuthenticateIsolated(gomock.Any(), identity).Return(&schema.AuthContext{
			Environment: map[string]string{"VAULT_TOKEN": identity + "-token"},
		}, nil)
	}
	config := func(identity string) *schema.AtmosConfiguration {
		return &schema.AtmosConfiguration{
			Stores:       pstore.StoreRegistry{"remote": backend},
			StoresConfig: pstore.StoresConfig{"remote": {Identity: identity}},
		}
	}
	first := configuredStoreService(config("first"), manager, storeScope{})
	second := configuredStoreService(config("second"), manager, storeScope{})
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() {
		_, readErr := first.Get("remote", "dev", "app", "first")
		firstDone <- readErr
	}()
	select {
	case <-firstRead:
	case <-time.After(5 * time.Second):
		t.Fatal("first read did not start")
	}
	go func() {
		_, readErr := second.Get("remote", "dev", "app", "second")
		secondDone <- readErr
	}()
	select {
	case <-secondRead:
		t.Error("the second identity entered Vault before the first read completed")
	case <-time.After(100 * time.Millisecond):
	}
	release.Do(func() { close(releaseFirst) })
	for _, done := range []chan error{firstDone, secondDone} {
		select {
		case readErr := <-done:
			require.NoError(t, readErr)
		case <-time.After(5 * time.Second):
			t.Fatal("store read did not finish")
		}
	}
}

func TestStoreServiceMissingIdentityNeverUsesAmbientVaultToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("failed identity reached Vault with fallback credentials")
	}))
	t.Cleanup(server.Close)
	backend, err := providers.NewVaultStore(&providers.VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "ambient-token"}, "")
	require.NoError(t, err)
	config := &schema.AtmosConfiguration{
		Stores:       pstore.StoreRegistry{"remote": backend},
		StoresConfig: pstore.StoresConfig{"remote": {Identity: "required"}},
	}
	service := configuredStoreService(config, nil, storeScope{})
	_, err = service.GetKey("remote", "app")
	require.ErrorIs(t, err, pstore.ErrIdentityNotConfigured)
}

func TestStoreServiceExplicitAuthDisableRestoresStaticVaultToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "static-token", r.Header.Get("X-Vault-Token"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"read-result"},"metadata":{"version":1}}}`))
	}))
	t.Cleanup(server.Close)
	backend, err := providers.NewVaultStore(&providers.VaultStoreOptions{Address: server.URL, Mount: "secret", Token: "static-token"}, "required")
	require.NoError(t, err)
	config := &schema.AtmosConfiguration{
		Stores:       pstore.StoreRegistry{"remote": backend},
		StoresConfig: pstore.StoresConfig{"remote": {Identity: "required"}},
	}
	service := configuredStoreService(config, nil, storeScope{Identity: "false"})
	value, err := service.GetKey("remote", "app")
	require.NoError(t, err)
	assert.Equal(t, "read-result", value)
}
