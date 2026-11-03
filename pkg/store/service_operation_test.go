package store

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceAuthenticationFailureStopsEveryBackendOperation(t *testing.T) {
	for _, method := range []string{"set", "get", "get-key", "delete", "keys"} {
		t.Run(method, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			backend := Store(NewMockStore(ctrl))
			if method == "delete" {
				backend = NewMockDeletableStore(ctrl)
			}
			if method == "keys" {
				backend = NewMockListableStore(ctrl)
			}
			calls := 0
			service := NewService(StoresConfig{}, StoreRegistry{"remote": backend}, WithOperationRunner(func(name string, _ func() (any, error)) (any, error) {
				calls++
				require.Equal(t, "remote", name)
				return nil, ErrAuthContextNotAvailable
			}))
			var err error
			switch method {
			case "set":
				err = service.Set("remote", "dev", "app", "key", "value")
			case "get":
				_, err = service.Get("remote", "dev", "app", "key")
			case "get-key":
				_, err = service.GetKey("remote", "key")
			case "delete":
				err = service.Delete("remote", "dev", "app", "key")
			case "keys":
				_, err = service.Keys("remote", "dev", "app")
			}
			require.ErrorIs(t, err, ErrAuthContextNotAvailable)
			require.Equal(t, 1, calls)
		})
	}
}

func TestServiceIdentityAwareBackendRequiresScopedOperationRunner(t *testing.T) {
	backend := NewMockIdentityAwareStore(gomock.NewController(t))
	service := NewService(StoresConfig{}, StoreRegistry{"remote": backend})
	_, err := service.GetKey("remote", "key")
	require.ErrorIs(t, err, ErrAuthContextNotAvailable)
}
