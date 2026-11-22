package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveLogoutTarget(t *testing.T) {
	tests := []struct {
		name string
		sel  logoutSelection
		want logoutTarget
	}{
		{name: "nothing selected prompts", sel: logoutSelection{}, want: logoutTarget{kind: logoutTargetInteractive}},
		{
			name: "--identity without a value prompts instead of looking up the sentinel",
			sel:  logoutSelection{identity: IdentityFlagSelectValue},
			want: logoutTarget{kind: logoutTargetInteractive},
		},
		{name: "--identity with a value", sel: logoutSelection{identity: "prod"}, want: logoutTarget{kind: logoutTargetIdentity, identity: "prod"}},
		{name: "positional identity wins over --identity", sel: logoutSelection{args: []string{"dev"}, identity: "prod"}, want: logoutTarget{kind: logoutTargetIdentity, identity: "dev"}},
		{name: "positional identity wins over a bare --identity", sel: logoutSelection{args: []string{"dev"}, identity: IdentityFlagSelectValue}, want: logoutTarget{kind: logoutTargetIdentity, identity: "dev"}},
		{name: "--all", sel: logoutSelection{all: true, identity: "prod"}, want: logoutTarget{kind: logoutTargetAll}},
		{name: "--provider", sel: logoutSelection{provider: "sso", identity: "prod"}, want: logoutTarget{kind: logoutTargetProvider}},
		{name: "--tags", sel: logoutSelection{tags: []string{"production"}}, want: logoutTarget{kind: logoutTargetTags}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveLogoutTarget(&tt.sel)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveLogoutTarget_TagsAreExclusive(t *testing.T) {
	for name, sel := range map[string]logoutSelection{
		"with --provider":          {tags: []string{"production"}, provider: "sso"},
		"with --all":               {tags: []string{"production"}, all: true},
		"with --identity":          {tags: []string{"production"}, identity: "prod"},
		"with positional identity": {tags: []string{"production"}, args: []string{"prod"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveLogoutTarget(&sel)
			assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
		})
	}
}
