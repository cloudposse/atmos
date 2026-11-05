package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestDeferredStoreTemplateCallerStackScope(t *testing.T) {
	for _, tc := range []struct {
		name          string
		auth          schema.AuthConfig
		storeIdentity string
		section       map[string]any
		reads         int
	}{
		{name: "ambient credentials share callers", reads: 1},
		{name: "configured auth keeps callers isolated", auth: schema.AuthConfig{Identities: map[string]schema.Identity{"local": {Kind: "aws/user", Default: true}}}, reads: 2},
		{name: "component auth keeps callers isolated", section: map[string]any{"auth": map[string]any{"identities": map[string]any{"local": map[string]any{"kind": "aws/user", "default": true}}}}, reads: 2},
		{name: "explicit store identity keeps callers isolated", storeIdentity: "local", reads: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := store.NewMockStore(gomock.NewController(t))
			ac := newDeferredTemplateStoreConfig(backend)
			ac.Auth = tc.auth
			ac.StoresConfig = store.StoresConfig{"remote": {Identity: tc.storeIdentity}}
			backend.EXPECT().Get("global", "target", "id").Return("value", nil).Times(tc.reads)
			for _, stack := range []string{"dev", "prod"} {
				funcs := AtmosFuncs{atmosConfig: ac, configAndStacksInfo: &schema.ConfigAndStacksInfo{Stack: stack, ComponentSection: tc.section}}
				value, err := funcs.Store("remote", "global", "target", "id")
				require.NoError(t, err)
				require.Equal(t, "value", value)
			}
		})
	}
}
