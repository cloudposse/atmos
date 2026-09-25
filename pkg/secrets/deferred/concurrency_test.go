package deferred

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/auth/types"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/secrets/providers/sops"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

func TestConcurrentSecretReadsKeepTheirComponentCredentials(t *testing.T) {
	for _, input := range []string{"!secret KEY", "!secret KEY | raw", "secret and ordinary store"} {
		t.Run(input, func(t *testing.T) {
			require.NoError(t, iolib.Initialize())
			ctrl := gomock.NewController(t)
			backend := store.NewMockIdentityAwareStore(ctrl)
			factory := authdeferred.NewMockAuthFactory(ctrl)
			ac := &schema.AtmosConfiguration{
				AuthManager:  authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
				Stores:       store.StoreRegistry{"vault": backend, "alias": backend},
				StoresConfig: store.StoresConfig{"vault": {Secret: true}, "alias": {Secret: true}},
			}
			copyConfig := *ac
			if input == "secret and ordinary store" {
				copyConfig.StoresConfig = store.StoresConfig{"alias": {}}
			}
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
			// The generated identity-aware store lacks RawStore, exercising the
			// secret provider's supported string-valued Get fallback for raw reads.
			backend.EXPECT().Get("dev", gomock.Any(), "KEY").DoAndReturn(func(_, component, _ string) (any, error) {
				if component == "first" {
					close(firstRead)
					<-releaseFirst
				}
				return active.Load().(string), nil
			}).Times(2)
			firstResult, secondResult := make(chan any, 1), make(chan any, 1)
			go func() {
				expression := input
				if input == "secret and ordinary store" {
					expression = "!secret KEY"
				}
				value, err := NewValue(ac, expression, "dev", concurrentSecretInfo("vault", "first", "account-one")).Resolve()
				assert.NoError(t, err)
				firstResult <- value
			}()
			select {
			case <-firstRead:
			case <-time.After(5 * time.Second):
				t.Fatal("first secret read did not start")
			}
			secondStarted := make(chan struct{})
			go func() {
				close(secondStarted)
				info := concurrentSecretInfo("alias", "second", "account-two")
				var value any
				var err error
				if input == "secret and ordinary store" {
					value, err = storedeferred.LookupStore(&copyConfig, info, storedeferred.StoreOptions{Name: "alias", Stack: "dev", Component: "second", Key: "KEY"})
				} else {
					value, err = NewValue(&copyConfig, input, "dev", info).Resolve()
				}
				assert.NoError(t, err)
				secondResult <- value
			}()
			<-secondStarted
			select {
			case <-secondBinding:
				t.Error("another lookup rebound the backend while the first secret read was in progress")
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
					t.Fatal("secret read did not complete")
				}
			}
		})
	}
}

func concurrentSecretInfo(name, component, profile string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{Stack: "dev", Component: component, ComponentSection: map[string]any{
		"secrets": map[string]any{"vars": map[string]any{"KEY": map[string]any{"store": name}}},
		"auth": map[string]any{"identities": map[string]any{
			"local": map[string]any{"kind": "aws/user", "default": true, "credentials": map[string]any{"profile": profile}},
		}},
	}}
}

func TestSecretAgeKeyStoreUsesScopedCredentials(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "secrets.enc.yaml")
	writer, err := sops.New(&schema.AtmosConfiguration{}, "encrypted", map[string]any{
		"encrypted": map[string]any{"kind": "sops/age", "spec": map[string]any{
			"file": file, "age_recipients": identity.Recipient().String(), "age_key": identity.String(),
		}},
	})
	require.NoError(t, err)
	require.NoError(t, writer.Set(providers.Coordinate{Stack: "dev", Component: "app", Key: "KEY"}, "decrypted-secret"))
	ctrl := gomock.NewController(t)
	backend := store.NewMockIdentityAwareStore(ctrl)
	factory := authdeferred.NewMockAuthFactory(ctrl)
	ac := &schema.AtmosConfiguration{
		AuthManager: authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory}),
		Stores:      store.StoreRegistry{"keys": backend},
	}
	manager := types.NewMockAuthManager(ctrl)
	manager.EXPECT().GetChain().Return([]string{"local"}).AnyTimes()
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "component-account"},
	}}).AnyTimes()
	gomock.InOrder(
		backend.EXPECT().ResetAuthContext(),
		factory.EXPECT().Create(gomock.Any(), gomock.Cond(func(config *schema.AuthConfig) bool {
			return config.Identities["local"].Credentials["profile"] == "component-account"
		}), "dev").Return(manager, nil),
		backend.EXPECT().SetAuthContext(gomock.Any(), "local").Do(func(resolver store.AuthContextResolver, name string) {
			resolved, resolveErr := resolver.ResolveAWSAuthContext(t.Context(), name)
			require.NoError(t, resolveErr)
			require.Equal(t, "component-account", resolved.Profile)
		}),
		backend.EXPECT().Get("encrypted", "age-key", "encrypted").Return(identity.String(), nil),
	)
	info := concurrentSecretInfo("keys", "app", "component-account")
	info.ComponentSection["secrets"] = map[string]any{
		"vars": map[string]any{"KEY": map[string]any{"sops": "encrypted"}},
		"providers": map[string]any{
			"encrypted": map[string]any{"kind": "sops/age", "spec": map[string]any{
				"file": file, "age_key": map[string]any{"store": "keys"},
			}},
		},
	}
	value, err := NewValue(ac, "!secret KEY", "dev", info).Resolve()
	require.NoError(t, err)
	require.Equal(t, "decrypted-secret", value)
	require.Nil(t, ac.SecretsAuth, "a lookup must not publish its secret auth resolver on the shared configuration")
	require.Same(t, backend, ac.Stores["keys"], "a lookup must not replace the shared store registry")
}
