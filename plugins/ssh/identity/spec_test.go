package identity_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		want    identity.Spec
		wantErr string
	}{
		{
			name: "defaults", raw: map[string]any{"store": "file", "allow": []any{"signing"}},
			want: identity.Spec{Authority: "self", Store: "file", Allow: []agent.Allow{agent.AllowSigning}},
		},
		{
			name: "everything",
			raw: map[string]any{
				"authority": "github", "store": "secure-enclave", "allow": []any{"authentication", "signing", "signing"},
				"require": []any{"unlocked", "unlocked"}, "user": "deploy", "key": "shared",
				"rewrite_https": []any{"github.com", "gitlab.com:8443", "github.com"}, "root": absolute("data"),
			},
			want: identity.Spec{
				Authority: "github", Store: "secure-enclave",
				Allow:   []agent.Allow{agent.AllowSigning, agent.AllowAuthentication},
				Require: []keystore.Requirement{keystore.Unlocked}, User: "deploy", Key: "shared",
				RewriteHTTPS: []string{"github.com", "gitlab.com:8443"}, Root: absolute("data"),
			},
		},
		{
			name: "require as a string", raw: map[string]any{"store": "file", "allow": "signing", "require": "unlocked"},
			want: identity.Spec{Authority: "self", Store: "file", Allow: []agent.Allow{agent.AllowSigning}, Require: []keystore.Requirement{keystore.Unlocked}},
		},
		{name: "unknown field", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "colour": "red"}, wantErr: `unknown field "colour"`},
		{name: "store is required", raw: map[string]any{"allow": []any{"signing"}}, wantErr: "store is required"},
		{name: "store with a path", raw: map[string]any{"store": "../x", "allow": []any{"signing"}}, wantErr: "not a key store name"},
		{name: "allow is required", raw: map[string]any{"store": "file"}, wantErr: "allow is required"},
		{name: "allow is empty", raw: map[string]any{"store": "file", "allow": []any{}}, wantErr: "allow is required"},
		{name: "unknown use", raw: map[string]any{"store": "file", "allow": []any{"decrypt"}}, wantErr: `unknown use "decrypt"`},
		{name: "unknown requirement", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "require": []any{"presence"}}, wantErr: `unknown term "presence"`},
		{name: "wrong type", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "user": 7}, wantErr: "user must be"},
		{name: "list with a number", raw: map[string]any{"store": "file", "allow": []any{"signing", 3}}, wantErr: "allow must contain only"},
		{name: "relative root", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "root": "data"}, wantErr: "absolute"},
		{name: "unsafe key name", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "key": "../k"}, wantErr: "key:"},
		{name: "login name with a space", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "user": "a b"}, wantErr: "not a valid login name"},
		{name: "bad host", raw: map[string]any{"store": "file", "allow": []any{"signing"}, "rewrite_https": []any{"a/b"}}, wantErr: "not a host name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := identity.ParseSpec(tt.raw)
			if tt.wantErr != "" {
				require.ErrorIs(t, err, identity.ErrInvalidSpec)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSpecKeyNameDefaultsToTheIdentity(t *testing.T) {
	assert.Equal(t, "ops", identity.Spec{}.KeyName("ops"))
	assert.Equal(t, "shared", identity.Spec{Key: "shared"}.KeyName("ops"))
}

func TestValidate(t *testing.T) {
	r := newRig(t)
	assert.NoError(t, r.svc.Validate("ops", baseSpec(nil)))
	assert.ErrorIs(t, r.svc.Validate("ops", map[string]any{}), identity.ErrInvalidSpec)
	assert.Error(t, r.svc.Validate("../ops", baseSpec(nil)))
}
