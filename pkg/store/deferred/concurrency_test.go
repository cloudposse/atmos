package deferred

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestConcurrentStoreReadsKeepTheirComponentCredentials(t *testing.T) {
	for _, method := range []string{"get", "get-key", "template"} {
		t.Run(method, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			backend := store.NewMockIdentityAwareStore(ctrl)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				AuthManager: authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
				Stores:      store.StoreRegistry{"remote": backend, "alias": backend},
			}
			copyConfig := *ac
			firstRead, releaseFirst := make(chan struct{}), make(chan struct{})
			secondBinding := make(chan struct{})
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(releaseFirst) }) })
			var active atomic.Value
			var resets atomic.Int32
			backend.EXPECT().ResetAuthContext().Do(func() {
				active.Store("")
				if resets.Add(1) == 2 {
					close(secondBinding)
				}
			}).Times(2)
			backend.EXPECT().SetAuthContext(gomock.Any(), "local").Do(func(resolver store.AuthContextResolver, identity string) {
				resolved, err := resolver.ResolveAWSAuthContext(t.Context(), identity)
				assert.NoError(t, err)
				active.Store(resolved.Profile)
			}).Times(2)
			for _, profile := range []string{"account-one", "account-two"} {
				manager := types.NewMockAuthManager(ctrl)
				manager.EXPECT().GetChain().Return([]string{"local"}).AnyTimes()
				manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{
					AWS: &schema.AWSAuthContext{Profile: profile},
				}}).AnyTimes()
				factory.EXPECT().Create(gomock.Any(), gomock.Cond(func(config *schema.AuthConfig) bool {
					return config.Identities["local"].Credentials["profile"] == profile
				}), "dev").Return(manager, nil)
			}
			read := func(key string) string {
				if key == "first" {
					close(firstRead)
					<-releaseFirst
				}
				return active.Load().(string)
			}
			if method == "get-key" {
				backend.EXPECT().GetKey(gomock.Any()).DoAndReturn(func(key string) (any, error) {
					return read(key), nil
				}).Times(2)
			} else {
				backend.EXPECT().Get("dev", "app", gomock.Any()).DoAndReturn(func(_, _, key string) (any, error) {
					return read(key), nil
				}).Times(2)
			}
			lookup := func(config *schema.AtmosConfiguration, name, key, profile string) (any, error) {
				info := concurrentStoreInfo(profile)
				switch method {
				case "get-key":
					return ReadStore(config, "!store.get "+name+" "+key, "dev", info)
				case "template":
					return LookupStore(config, info, StoreOptions{Name: name, Stack: "dev", Component: "app", Key: key})
				default:
					return ReadStore(config, "!store "+name+" dev app "+key, "dev", info)
				}
			}
			firstResult, secondResult := make(chan any, 1), make(chan any, 1)
			go func() {
				value, err := lookup(ac, "remote", "first", "account-one")
				assert.NoError(t, err)
				firstResult <- value
			}()
			select {
			case <-firstRead:
			case <-time.After(5 * time.Second):
				t.Fatal("first store read did not start")
			}
			secondStarted := make(chan struct{})
			go func() {
				close(secondStarted)
				value, err := lookup(&copyConfig, "alias", "second", "account-two")
				assert.NoError(t, err)
				secondResult <- value
			}()
			<-secondStarted
			select {
			case <-secondBinding:
				t.Error("another lookup rebound the backend while the first read was in progress")
			case <-time.After(100 * time.Millisecond):
			}
			release.Do(func() { close(releaseFirst) })
			for _, result := range []struct {
				channel chan any
				profile string
			}{{firstResult, "account-one"}, {secondResult, "account-two"}} {
				select {
				case got := <-result.channel:
					require.Equal(t, result.profile, got)
				case <-time.After(5 * time.Second):
					t.Fatal("store read did not complete")
				}
			}
		})
	}
}

func concurrentStoreInfo(profile string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: map[string]any{
		"auth": map[string]any{"identities": map[string]any{
			"local": map[string]any{"kind": "aws/user", "default": true, "credentials": map[string]any{"profile": profile}},
		}},
	}}
}
