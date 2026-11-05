package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestDeferredStoreTemplateEmptyAuthTracksGlobalChanges(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "ambient"
		if configured {
			name = "configured identity"
		}
		t.Run(name, func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			ac := newDeferredTemplateStoreConfig(backend)
			if configured {
				ac.Auth.Identities = map[string]schema.Identity{"local": {Kind: "aws/user", Default: true}}
			}
			info := &schema.ConfigAndStacksInfo{Stack: "dev"}
			funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: info}
			backend.EXPECT().Get("global", "target", "id").Return("original", nil).Times(1)
			backend.EXPECT().Get("global", "target", "id").Return("changed global", nil).Times(1)
			backend.EXPECT().Get("global", "target", "id").Return("changed component", nil).Times(1)

			read := func(want string) {
				t.Helper()
				value, err := funcs.Store("remote", "global", "target", "id")
				require.NoError(t, err)
				require.Equal(t, want, value)
			}
			read("original")
			// The stack processor materializes an empty auth map even without
			// overrides. It must share the same scope as an absent auth section.
			info.ComponentSection = map[string]any{"auth": map[string]any{}}
			read("original")
			ac.Auth.Identities = map[string]schema.Identity{"changed": {Kind: "aws/user", Default: true}}
			read("changed global")
			info.ComponentSection["auth"] = map[string]any{"realm": "component"}
			read("changed component")
			read("changed component")
		})
	}
}
