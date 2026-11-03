package exec

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

func TestLegacyStoreReadsShareCompleteOperationLock(t *testing.T) {
	for _, method := range []string{"template", "yaml", "yaml-get"} {
		t.Run(method, func(t *testing.T) {
			backend := store.NewMockIdentityAwareStore(gomock.NewController(t))
			config := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
			firstRead, secondRead, releaseFirst := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(releaseFirst) }) })
			if method == "yaml-get" {
				backend.EXPECT().GetKey("key").DoAndReturn(func(string) (any, error) {
					close(secondRead)
					return "current-identity-value", nil
				})
			} else {
				backend.EXPECT().Get("dev", "app", "key").DoAndReturn(func(string, string, string) (any, error) {
					close(secondRead)
					return "current-identity-value", nil
				})
			}
			firstDone := make(chan error, 1)
			go func() {
				_, err := storedeferred.WithStoreAuth(config, nil, "remote", func() (any, error) {
					close(firstRead)
					<-releaseFirst
					return nil, nil
				})
				firstDone <- err
			}()
			<-firstRead
			secondDone := make(chan any, 1)
			go func() {
				switch method {
				case "template":
					value, err := storeFunc(config, "remote", "dev", "app", "key")
					if err != nil {
						secondDone <- err
						return
					}
					secondDone <- value
				case "yaml":
					secondDone <- processTagStore(config, "!store remote dev app key", "dev")
				case "yaml-get":
					secondDone <- processTagStoreGet(config, "!store.get remote key", "dev")
				}
			}()
			select {
			case <-secondRead:
				t.Error("legacy store read bypassed a complete operation's authentication lock")
			case <-time.After(100 * time.Millisecond):
			}
			release.Do(func() { close(releaseFirst) })
			require.NoError(t, <-firstDone)
			select {
			case value := <-secondDone:
				require.Equal(t, "current-identity-value", value)
			case <-time.After(5 * time.Second):
				t.Fatal("legacy store read did not finish")
			}
		})
	}
}

func TestLegacyStoreTemplateDoesNotHideIdentityFailure(t *testing.T) {
	for _, failure := range []error{store.ErrAuthContextNotAvailable, errUtils.ErrInvalidAuthConfig, errUtils.ErrInvalidIdentityConfig, errUtils.ErrInvalidIdentityKind} {
		t.Run(failure.Error(), func(t *testing.T) {
			backend := store.NewMockIdentityAwareStore(gomock.NewController(t))
			backend.EXPECT().Get("dev", "app", "key").Return(nil, failure)
			config := &schema.AtmosConfiguration{Stores: store.StoreRegistry{"remote": backend}}
			_, err := storeFunc(config, "remote", "dev", "app", "key")
			require.ErrorIs(t, err, failure)
		})
	}
}
