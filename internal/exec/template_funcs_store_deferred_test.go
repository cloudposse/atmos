package exec

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

// A value-backed store has no stable pointer identity for the cache key.
type valueBackedTemplateStore struct{ reads *int }

func (s valueBackedTemplateStore) Set(_, _, _ string, _ any) error { return nil }
func (s valueBackedTemplateStore) Get(_, _, _ string) (any, error) {
	(*s.reads)++
	return *s.reads, nil
}
func (s valueBackedTemplateStore) GetKey(_ string) (any, error) { return nil, nil }

func newDeferredTemplateStoreConfig(backend store.Store) *schema.AtmosConfiguration {
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
	authdeferred.ConfigureAuth(ac, "")
	return ac
}

func TestDeferredStoreTemplateMemoizesAcrossComponents(t *testing.T) {
	backend := store.NewMockStore(gomock.NewController(t))
	ac := newDeferredTemplateStoreConfig(backend)
	for key := range 11 {
		component := fmt.Sprintf("target-%d", key)
		backend.EXPECT().Get("global", component, "id").Return(component, nil).Times(1)
	}

	for call := range 2200 {
		key := call % 11
		component := fmt.Sprintf("target-%d", key)
		funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{
			Stack: "dev", Component: fmt.Sprintf("consumer-%d", call),
		}}
		value, err := funcs.Store("remote", "global", component, "id")
		require.NoError(t, err)
		require.Equal(t, component, value)
	}
}

func TestDeferredStoreTemplateCacheSkipsRepeatedAuthBinding(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := store.NewMockIdentityAwareStore(ctrl)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	manager := types.NewMockAuthManager(ctrl)
	ac := newDeferredTemplateStoreConfig(backend)
	setDeferredAuthFactory(ac, factory)
	manager.EXPECT().GetChain().Return([]string{"local"}).AnyTimes()
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "local"},
	}}).AnyTimes()
	factory.EXPECT().Create(gomock.Any(), gomock.Any(), "dev").Return(manager, nil).Times(1)
	backend.EXPECT().ResetAuthContext().Times(1)
	backend.EXPECT().SetAuthContext(gomock.Any(), "local").Times(1)
	backend.EXPECT().Get("global", "target", "id").Return("resolved", nil).Times(1)
	funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "dev"}}
	for range 2 {
		value, err := funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "resolved", value)
	}
}

func TestDeferredStoreTemplateCacheScopes(t *testing.T) {
	t.Run("effective credentials", func(t *testing.T) {
		backend := store.NewMockStore(gomock.NewController(t))
		ac := newDeferredTemplateStoreConfig(backend)
		backend.EXPECT().Get("global", "target", "id").Return("first", nil)
		backend.EXPECT().Get("global", "target", "id").Return("second", nil)
		for index, profile := range []string{"account-one", "account-two"} {
			info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: map[string]any{
				"auth": map[string]any{"identities": map[string]any{
					"local": map[string]any{"kind": "aws/user", "default": true, "credentials": map[string]any{"profile": profile}},
				}},
			}}
			value, err := (AtmosFuncs{atmosConfig: ac, configAndStacksInfo: info}).Store("remote", "global", "target", "id")
			require.NoError(t, err)
			require.Equal(t, []string{"first", "second"}[index], value)
		}
	})

	t.Run("configured identity and disabled state", func(t *testing.T) {
		backend := store.NewMockStore(gomock.NewController(t))
		ac := newDeferredTemplateStoreConfig(backend)
		for _, value := range []string{"first", "second", "third"} {
			backend.EXPECT().Get("global", "target", "id").Return(value, nil)
		}
		info := &schema.ConfigAndStacksInfo{Stack: "dev"}
		funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: info}
		ac.StoresConfig = store.StoresConfig{"remote": {Identity: "account-one"}}
		value, err := funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "first", value)
		ac.StoresConfig["remote"] = store.StoreConfig{Identity: "account-two"}
		value, err = funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "second", value)
		info.AuthDisabled = true
		value, err = funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "third", value)
	})

	t.Run("backend and invocation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		firstBackend, secondBackend := store.NewMockStore(ctrl), store.NewMockStore(ctrl)
		ac := newDeferredTemplateStoreConfig(firstBackend)
		firstBackend.EXPECT().Get("global", "target", "id").Return("first", nil)
		secondBackend.EXPECT().Get("global", "target", "id").Return("second", nil)
		secondBackend.EXPECT().Get("global", "target", "id").Return("fresh", nil)
		funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "dev"}}
		value, err := funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "first", value)
		ac.Stores["remote"] = secondBackend
		value, err = funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "second", value)
		authdeferred.ConfigureAuth(ac, "")
		value, err = funcs.Store("remote", "global", "target", "id")
		require.NoError(t, err)
		require.Equal(t, "fresh", value)
	})
}

func TestDeferredStoreTemplateRetriesNilAndErrors(t *testing.T) {
	backend := store.NewMockStore(gomock.NewController(t))
	ac := newDeferredTemplateStoreConfig(backend)
	funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "dev"}}
	backend.EXPECT().Get("global", "target", "nil").Return(nil, nil)
	backend.EXPECT().Get("global", "target", "nil").Return("found", nil)
	backend.EXPECT().Get("global", "target", "error").Return(nil, errors.New("temporary failure"))
	backend.EXPECT().Get("global", "target", "error").Return("recovered", nil)

	value, err := funcs.Store("remote", "global", "target", "nil")
	require.NoError(t, err)
	require.Nil(t, value)
	for range 2 {
		value, err = funcs.Store("remote", "global", "target", "nil")
		require.NoError(t, err)
		require.Equal(t, "found", value)
	}
	_, err = funcs.Store("remote", "global", "target", "error")
	require.ErrorContains(t, err, "temporary failure")
	for range 2 {
		value, err = funcs.Store("remote", "global", "target", "error")
		require.NoError(t, err)
		require.Equal(t, "recovered", value)
	}
}

func TestDeferredStoreTemplateConcurrentMissesShareRead(t *testing.T) {
	backend := store.NewMockStore(gomock.NewController(t))
	ac := newDeferredTemplateStoreConfig(backend)
	funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: "dev"}}
	entered, release := make(chan struct{}), make(chan struct{})
	backend.EXPECT().Get("global", "target", "id").DoAndReturn(func(_, _, _ string) (any, error) {
		close(entered)
		<-release
		return "shared", nil
	}).Times(1)
	const readers = 16
	results := make(chan any, readers)
	errs := make(chan error, readers)
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := funcs.Store("remote", "global", "target", "id")
			results <- value
			errs <- err
		}()
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("store read did not start")
	}
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for value := range results {
		require.Equal(t, "shared", value)
	}
}

func TestDeferredStoreTemplateBypassesUnsafeCacheKeys(t *testing.T) {
	opts := storedeferred.StoreOptions{Name: "remote", Stack: "global", Component: "target", Key: "id"}

	t.Run("missing backend", func(t *testing.T) {
		ac := newDeferredTemplateStoreConfig(nil)
		for range 2 {
			_, err := deferredStoreFunc(ac, nil, opts)
			require.ErrorIs(t, err, errUtils.ErrStoreNotFound)
		}
	})

	t.Run("typed nil backend", func(t *testing.T) {
		var backend *store.MockStore
		ac := newDeferredTemplateStoreConfig(backend)
		_, ok := deferredStoreCacheKey(ac, nil, opts)
		require.False(t, ok)
	})

	t.Run("value backed backend", func(t *testing.T) {
		reads := 0
		ac := newDeferredTemplateStoreConfig(valueBackedTemplateStore{reads: &reads})
		for want := 1; want <= 2; want++ {
			value, err := deferredStoreFunc(ac, nil, opts)
			require.NoError(t, err)
			require.Equal(t, want, value)
		}
		require.Equal(t, 2, reads)
	})

	t.Run("secret backend", func(t *testing.T) {
		backend := store.NewMockStore(gomock.NewController(t))
		ac := newDeferredTemplateStoreConfig(backend)
		ac.StoresConfig = store.StoresConfig{"remote": {Secret: true}}
		for range 2 {
			_, err := deferredStoreFunc(ac, nil, opts)
			require.ErrorIs(t, err, errUtils.ErrStoreIsSecret)
		}
	})

	t.Run("unsupported store config", func(t *testing.T) {
		backend := store.NewMockStore(gomock.NewController(t))
		ac := newDeferredTemplateStoreConfig(backend)
		ac.StoresConfig = store.StoresConfig{"remote": {Options: map[string]any{"unsupported": make(chan int)}}}
		backend.EXPECT().Get("global", "target", "id").Return("first", nil)
		backend.EXPECT().Get("global", "target", "id").Return("second", nil)
		for _, want := range []string{"first", "second"} {
			value, err := deferredStoreFunc(ac, nil, opts)
			require.NoError(t, err)
			require.Equal(t, want, value)
		}
	})

	t.Run("invalid effective auth", func(t *testing.T) {
		backend := store.NewMockStore(gomock.NewController(t))
		ac := newDeferredTemplateStoreConfig(backend)
		info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
			"auth": map[string]any{"identities": map[string]any{
				"undefined": map[string]any{"default": true},
			}},
		}}
		_, ok := deferredStoreCacheKey(ac, info, opts)
		require.False(t, ok)
	})
}
