package deferred

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestReadStoreParsingAndSelection(t *testing.T) {
	for _, tc := range []struct {
		input, stack, component, key string
		getKey                       bool
	}{
		{input: "!store remote target key", stack: "dev", component: "target", key: "key"},
		{input: "!store remote prod target key", stack: "prod", component: "target", key: "key"},
		{input: "!store.get remote key", key: "key", getKey: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			s := store.NewMockStore(gomock.NewController(t))
			ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
			if tc.getKey {
				s.EXPECT().GetKey(tc.key).Return("resolved", nil)
			} else {
				s.EXPECT().Get(tc.stack, tc.component, tc.key).Return("resolved", nil)
			}
			value, err := ReadStore(ac, tc.input, "dev", nil)
			require.NoError(t, err)
			require.Equal(t, "resolved", value)
		})
	}
	for _, input := range []string{"!store", "!store.get", "!store.get remote"} {
		_, err := ReadStore(&schema.AtmosConfiguration{}, input, "dev", nil)
		require.Error(t, err)
	}
}

func TestReadStoreErrorsAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, suffix string
		value        any
		readErr      error
		want         any
		wantErr      error
	}{
		{name: "read error", readErr: errUtils.ErrStoreNotFound, wantErr: errUtils.ErrStoreNotFound},
		{name: "fallback on read error", suffix: " | default fallback", readErr: errUtils.ErrStoreNotFound, want: "fallback"},
		{name: "fallback on null", suffix: " | default fallback", want: "fallback"},
		{name: "auth failure never defaults", suffix: " | default fallback", readErr: errUtils.ErrAuthenticationUnavailable, wantErr: errUtils.ErrAuthenticationUnavailable},
		{name: "query", suffix: " | query .id", value: map[string]any{"id": "resolved"}, want: "resolved"},
		{name: "invalid query", suffix: " | query .[", value: map[string]any{"id": "resolved"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := store.NewMockStore(gomock.NewController(t))
			ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
			s.EXPECT().GetKey("key").Return(tc.value, tc.readErr)
			value, err := ReadStore(ac, "!store.get remote key"+tc.suffix, "dev", nil)
			switch {
			case tc.name == "invalid query":
				require.Error(t, err)
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			default:
				require.NoError(t, err)
				require.Equal(t, tc.want, value)
			}
		})
	}
}

func TestLookupStoreGuardsAndTemplateRead(t *testing.T) {
	s := store.NewMockStore(gomock.NewController(t))
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}}
	_, err := LookupStore(ac, nil, StoreOptions{Name: "missing"})
	require.ErrorIs(t, err, errUtils.ErrStoreNotFound)
	ac.StoresConfig = store.StoresConfig{"remote": {Secret: true}}
	_, err = LookupStore(ac, nil, StoreOptions{Name: "remote"})
	require.ErrorIs(t, err, errUtils.ErrStoreIsSecret)
	ac.StoresConfig = nil
	s.EXPECT().Get("dev", "target", "id").Return("resolved", nil)
	value, err := LookupStore(ac, nil, StoreOptions{Name: "remote", Stack: "dev", Component: "target", Key: "id"})
	require.NoError(t, err)
	require.Equal(t, "resolved", value)
}

func TestReadStoreFailedConfiguredIdentityStopsBackend(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := store.NewMockIdentityAwareStore(ctrl)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": s}, DeferredAuth: authdeferred.NewAuthResolver(authdeferred.AuthOptions{Factory: factory})}
	s.EXPECT().ResetAuthContext()
	factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(nil, errUtils.ErrAuthenticationUnavailable)
	_, err := ReadStore(ac, "!store.get remote key | default fallback", "dev", &schema.ConfigAndStacksInfo{Stack: "dev"})
	require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
	// No GetKey expectation: failed authentication must stop before any backend call.
}
