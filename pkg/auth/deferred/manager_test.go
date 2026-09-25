package deferred

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManagerConstructionDoesNotResolveCredentials(t *testing.T) {
	factory := NewMockAuthFactory(gomock.NewController(t))
	for _, disabled := range []bool{false, true} {
		manager := NewManager(AuthOptions{Factory: factory, Disabled: disabled})
		require.True(t, IsDeferred(manager))
		require.Equal(t, disabled, AuthDisabled(manager))
		for range 2 {
			require.Nil(t, manager.GetStackInfo())
			require.Empty(t, manager.GetChain())
		}
		require.False(t, manager.baseReady, "metadata access must not initialize providers or credential storage")
	}
	require.False(t, AuthDisabled(nil))
	require.False(t, IsDeferred(types.NewMockAuthManager(gomock.NewController(t))))
}

// Exercise the entire AuthManager contract with the generated mock. Each method
// must forward its arguments and results unchanged, including provider errors.
// Reflecting over the interface also makes newly added methods part of this test.
func TestManagerDelegatesAuthOperations(t *testing.T) {
	contract := reflect.TypeFor[types.AuthManager]()
	for i := range contract.NumMethod() {
		method := contract.Method(i)
		t.Run(method.Name, func(t *testing.T) {
			for _, failed := range []bool{false, true} {
				delegate := types.NewMockAuthManager(gomock.NewController(t))
				manager := NewManager(AuthOptions{})
				manager.baseManager, manager.baseReady = delegate, true
				args := managerMethodArguments(method.Type)
				results := managerMethodResults(method.Type, failed)
				call := reflect.ValueOf(delegate.EXPECT()).MethodByName(method.Name).Call(args)[0].Interface().(*gomock.Call)
				call.Return(results...).Times(1)
				got := reflect.ValueOf(manager).MethodByName(method.Name).Call(args)
				for j, value := range got {
					require.Equal(t, results[j], value.Interface(), "return %d", j)
				}
			}
		})
	}
}

func managerMethodArguments(method reflect.Type) []reflect.Value {
	args := make([]reflect.Value, method.NumIn())
	for i := range args {
		if method.In(i) == reflect.TypeFor[context.Context]() {
			args[i] = reflect.ValueOf(context.Background())
		} else {
			args[i] = managerContractValue(method.In(i))
		}
	}
	return args
}

func managerMethodResults(method reflect.Type, failed bool) []any {
	results := make([]any, method.NumOut())
	for i := range results {
		if method.Out(i) == reflect.TypeFor[error]() {
			if failed {
				results[i] = errUtils.ErrAuthenticationUnavailable
			}
		} else {
			results[i] = managerContractValue(method.Out(i)).Interface()
		}
	}
	return results
}

func managerContractValue(kind reflect.Type) reflect.Value {
	value := reflect.New(kind).Elem()
	switch kind.Kind() {
	case reflect.String:
		value.SetString("selected")
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Pointer:
		value.Set(reflect.New(kind.Elem()))
	case reflect.Slice:
		value.Set(reflect.MakeSlice(kind, 1, 1))
		value.Index(0).Set(managerContractValue(kind.Elem()))
	case reflect.Map:
		value.Set(reflect.MakeMap(kind))
		value.SetMapIndex(managerContractValue(kind.Key()), managerContractValue(kind.Elem()))
	case reflect.Interface:
		value.Set(reflect.ValueOf("principal setting"))
	}
	return value
}

func TestManagerUnavailableDoesNotCallDelegate(t *testing.T) {
	contract := reflect.TypeFor[types.AuthManager]()
	for _, disabled := range []bool{false, true} {
		manager := NewManager(AuthOptions{Disabled: disabled})
		for i := range contract.NumMethod() {
			method := contract.Method(i)
			results := reflect.ValueOf(manager).MethodByName(method.Name).Call(managerMethodArguments(method.Type))
			for j, result := range results {
				if method.Type.Out(j) == reflect.TypeFor[error]() {
					want := errUtils.ErrInvalidAuthConfig
					if disabled {
						want = errUtils.ErrAuthenticationUnavailable
					}
					require.ErrorIs(t, result.Interface().(error), want, method.Name)
				} else {
					require.True(t, result.IsZero(), method.Name)
				}
			}
		}
	}
}

func TestManagerInitializesDelegateOnce(t *testing.T) {
	config := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}}}
	manager := NewManager(AuthOptions{Config: config})
	first, err := manager.base()
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := manager.base()
	require.NoError(t, err)
	require.Same(t, first, second)
	require.Equal(t, "memory", manager.CredentialStoreType())
}
