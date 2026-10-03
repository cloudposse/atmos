package auth

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// neverAuthenticate fails the test if a target manager is requested.
func neverAuthenticate(t *testing.T) func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
	t.Helper()
	return func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
		t.Error("an invalid target auth block must not authenticate")
		return nil, nil
	}
}

// requireTargetContext asserts the diagnostics name the target, component and stack.
func requireTargetContext(t *testing.T, err error) {
	t.Helper()
	for key, want := range map[string]string{"target": "artifacts", "component": "vpc", "stack": "dev"} {
		got, ok := errUtils.GetContext(err, key)
		require.True(t, ok, "missing context %q", key)
		assert.Equal(t, want, got)
	}
}

// TestResolveTargetAuthStrictBlocks rejects typos and unselected identities instead of
// silently running the target as the component's identity.
func TestResolveTargetAuthStrictBlocks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     any
		requested string
		want      []error
		contains  string
	}{
		{
			name:     "misspelled identity key",
			block:    map[string]any{"identitty": "target"},
			want:     []error{errUtils.ErrProvisionTargetAuthUnknownKey, errUtils.ErrInvalidAuthConfig},
			contains: "identitty",
		},
		{
			name:  "identities without a default",
			block: map[string]any{"identities": map[string]any{"target": map[string]any{}}},
			want:  []error{errUtils.ErrProvisionTargetAuthNoIdentity},
		},
		{
			name: "several target defaults",
			block: map[string]any{"identities": map[string]any{
				"target": map[string]any{"default": true}, "global": map[string]any{"default": true},
			}},
			want:     []error{errUtils.ErrProvisionTargetAuthInvalid, errUtils.ErrMultipleDefaultIdentities},
			contains: "global, target",
		},
		{name: "scalar auth", block: "target", want: []error{errUtils.ErrProvisionTargetAuthInvalid, errUtils.ErrInvalidAuthConfig}},
		{name: "empty identity", block: map[string]any{"identity": ""}, want: []error{errUtils.ErrProvisionTargetAuthInvalid, errUtils.ErrInvalidAuthConfig}},
		{name: "numeric identity", block: map[string]any{"identity": 7}, want: []error{errUtils.ErrProvisionTargetAuthInvalid}},
		{name: "scalar identities", block: map[string]any{"identities": "target"}, want: []error{errUtils.ErrProvisionTargetAuthInvalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.TargetName, options.Info.ComponentFromArg = "artifacts", "vpc"
			options.TargetConfig = map[string]any{"auth": tc.block}
			options.RequestedIdentity = tc.requested
			options.CreateManager = neverAuthenticate(t)

			resolved, err := ResolveTargetAuth(options)
			require.Error(t, err)
			assert.Nil(t, resolved)
			for _, sentinel := range tc.want {
				assert.ErrorIs(t, err, sentinel)
			}
			if tc.contains != "" {
				assert.Contains(t, err.Error()+" "+strings.Join(cockroachErrors.GetAllDetails(err), " "), tc.contains)
			}
			requireTargetContext(t, err)
			// The static validator used by dry runs must agree with the real run.
			staticErr := ValidateTargetAuth(options)
			require.Error(t, staticErr)
			assert.ErrorIs(t, staticErr, tc.want[0])
		})
	}
}

// TestResolveTargetAuthAcceptsSelectedBlocks keeps valid forms working, including a
// declared-but-unselected identity when the caller chose one explicitly.
func TestResolveTargetAuthAcceptsSelectedBlocks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     map[string]any
		requested string
	}{
		{"identity shorthand", map[string]any{"identity": "global"}, ""},
		{"default identity", map[string]any{"identities": map[string]any{"target": map[string]any{"default": true}}}, ""},
		{"providers inherit a default", map[string]any{"providers": map[string]any{}}, ""},
		{"explicit CLI identity selects", map[string]any{"identities": map[string]any{"target": map[string]any{}}}, "target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.TargetConfig = map[string]any{"auth": tc.block}
			options.RequestedIdentity = tc.requested
			require.NoError(t, ValidateTargetAuth(options))
		})
	}
}

// TestResolveTargetAuthDeclaredIdentityWins checks that an output reference follows the
// identity the producer's target declares rather than the caller's selection.
func TestResolveTargetAuthDeclaredIdentityWins(t *testing.T) {
	for _, tc := range []struct {
		name, requested, want string
		block                 map[string]any
		wantSelection         string
	}{
		{
			name: "declared default beats caller", requested: "component", want: "target",
			block:         map[string]any{"identities": map[string]any{"target": map[string]any{"default": true}}},
			wantSelection: "target",
		},
		{
			name: "declared identity beats prompt", requested: cfg.IdentityFlagSelectValue, want: "global",
			block:         map[string]any{"identity": "global"},
			wantSelection: "global",
		},
		{
			name: "no declared identity ignores the caller", requested: "component", want: "global",
			block:         map[string]any{"providers": map[string]any{}},
			wantSelection: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.TargetConfig = map[string]any{"auth": tc.block}
			options.RequestedIdentity, options.DeclaredIdentityWins = tc.requested, true
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetChain().Return([]string{tc.want})
			manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
			var selected string
			options.CreateManager = func(identity string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, _ string) (AuthManager, error) {
				selected = identity
				return manager, nil
			}
			resolved, err := ResolveTargetAuth(options)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSelection, selected)
			assert.Equal(t, tc.want, resolved.Identity)
		})
	}
}

// TestResolveTargetAuthDeclaredIdentityWinsWithoutAuth keeps the caller's credentials
// when the producer's target declares no auth at all.
func TestResolveTargetAuthDeclaredIdentityWinsWithoutAuth(t *testing.T) {
	options := targetOptions()
	options.TargetConfig = map[string]any{"kind": "aws/cloudformation"}
	options.RequestedIdentity, options.DeclaredIdentityWins = "component", true
	options.CreateManager = neverAuthenticate(t)
	resolved, err := ResolveTargetAuth(options)
	require.NoError(t, err)
	assert.Same(t, options.Info, resolved)
}

// TestResolveTargetAuthDisabledWarns names the target and the identity that disabled auth bypasses.
func TestResolveTargetAuthDisabledWarns(t *testing.T) {
	var messages []string
	original := warnTargetAuthBypass
	t.Cleanup(func() { warnTargetAuthBypass = original })
	warnTargetAuthBypass = func(message string) { messages = append(messages, message) }

	for _, tc := range []struct {
		name       string
		block      map[string]any
		target     string
		wantWarned bool
	}{
		{"identity shorthand", map[string]any{"identity": "sandbox"}, "warn-shorthand", true},
		{"default identity", map[string]any{"identities": map[string]any{"sandbox": map[string]any{"default": true}}}, "warn-default", true},
		{"no declared identity", map[string]any{"providers": map[string]any{}}, "warn-none", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages = nil
			options := targetOptions()
			options.TargetName, options.TargetConfig = tc.target, map[string]any{"auth": tc.block}
			options.RequestedIdentity = cfg.IdentityFlagDisabledValue
			options.CreateManager = neverAuthenticate(t)

			for range 2 {
				resolved, err := ResolveTargetAuth(options)
				require.NoError(t, err)
				assert.True(t, resolved.AuthDisabled)
			}
			if !tc.wantWarned {
				assert.Empty(t, messages)
				return
			}
			require.Len(t, messages, 1, "each target is reported once")
			assert.Contains(t, messages[0], `"`+tc.target+`"`)
			assert.Contains(t, messages[0], `"sandbox"`)
			assert.True(t, strings.Contains(messages[0], "ATMOS_IDENTITY=false"))
		})
	}
}

// TestResolveTargetAuthFailureNamesIdentity adds target context and the failing identity to manager errors.
func TestResolveTargetAuthFailureNamesIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cause    error
		wantHint string
	}{
		{"unknown identity", errUtils.ErrIdentityNotFound, "Define the identity"},
		{"authentication denied", errUtils.ErrAuthenticationFailed, "does not fall back"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := targetOptions()
			options.TargetName, options.Info.ComponentFromArg = "artifacts", "vpc"
			options.TargetConfig = map[string]any{"auth": map[string]any{"identity": "missing"}}
			options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
				return nil, tc.cause
			}
			_, err := ResolveTargetAuth(options)
			require.ErrorIs(t, err, errUtils.ErrProvisionTargetAuthFailed)
			assert.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
			assert.ErrorIs(t, err, tc.cause)
			requireTargetContext(t, err)
			identity, _ := errUtils.GetContext(err, "identity")
			assert.Equal(t, "missing", identity)
			hints := strings.Join(cockroachErrors.GetAllHints(err), " ")
			assert.Contains(t, hints, tc.wantHint)
		})
	}
}

// TestResolveTargetAuthNoSelectionNamesTarget reports a manager that selected nothing with full context.
func TestResolveTargetAuthNoSelectionNamesTarget(t *testing.T) {
	options := targetOptions()
	options.TargetName, options.Info.ComponentFromArg = "artifacts", "vpc"
	options.TargetConfig = map[string]any{"auth": map[string]any{"providers": map[string]any{}}}
	options.CreateManager = func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error) {
		return nil, nil
	}
	_, err := ResolveTargetAuth(options)
	require.ErrorIs(t, err, errUtils.ErrProvisionTargetAuthNoIdentity)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitializeAuthManager)
	requireTargetContext(t, err)
	assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "--identity=<name>")
}
