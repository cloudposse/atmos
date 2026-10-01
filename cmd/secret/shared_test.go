package secret

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestParseScope_MissingComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// --stack present but --component absent → "set" resolves a component-less name via
	// findGlobalSetContext (not requireScopeComponent; see TestParseScope_MissingComponentViaGet
	// for that path), and fails here because no global declaration is enumerable in this fake
	// setup (the existing MissingScope tests omit both flags and stop at the stack check).
	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

// TestParseScope_MissingComponentViaGet exercises parseScope → requireScopeComponent directly:
// unlike "set" above, "get" always requires an explicit --component, so --stack present but
// --component absent reaches requireScopeComponent's own "component is required" error.
func TestParseScope_MissingComponentViaGet(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "get", "API_KEY", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestParseScopeStack_DoesNotLeakAcrossInvocations pins a regression: parseScopeStack used to call
// viper.Set unconditionally after resolving --stack, which installs a permanent override that
// outranks the bound flag for the rest of the process. Once any invocation resolved one stack that
// way, every later invocation in the same process silently ignored its own --stack flag and kept
// resolving the first one — exactly what made this package's tests order-dependent under
// `-shuffle=on` (a "prod"-stack test running before a "dev"-stack test poisoned the latter).
func TestParseScopeStack_DoesNotLeakAcrossInvocations(t *testing.T) {
	svc := newFakeSecretService()
	svc.scopes = map[string]secrets.Scope{"SHARED_TOKEN": secrets.ScopeGlobal}
	installService(t, svc, nil)

	// Record the secretScope passed to loadServiceFn on each call. Asserting only the set values
	// below would pass even if a bug resolved both invocations to the same (wrong) stack, since
	// the fake service ignores scope entirely — the stack itself must be asserted too.
	originalLoadService := loadServiceFn
	var scopes []secretScope
	loadServiceFn = func(scope secretScope) (secretService, error) {
		scopes = append(scopes, scope)
		return originalLoadService(scope)
	}
	t.Cleanup(func() { loadServiceFn = originalLoadService })

	overrideEnumerateScopes(t, []scopeEntry{
		{
			Stack: "prod", Component: "example-service", ComponentType: "helm",
			Section: secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"}),
		},
		{
			Stack: "dev", Component: "example-service", ComponentType: "helm",
			Section: secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"}),
		},
	}, nil)

	require.NoError(t, runSecretSubcommand(t, "set", "SHARED_TOKEN=v1", "--stack", "prod"))
	require.NoError(t, runSecretSubcommand(t, "set", "SHARED_TOKEN=v2", "--stack", "dev"))

	require.Len(t, svc.setCalls, 2)
	assert.Equal(t, "v1", svc.setCalls[0].value)
	assert.Equal(t, "v2", svc.setCalls[1].value)

	require.Len(t, scopes, 2)
	assert.Equal(t, "prod", scopes[0].Stack, "first invocation must resolve its own --stack, not leak into the second")
	assert.Equal(t, "dev", scopes[1].Stack, "second invocation must resolve its own --stack, not the first invocation's")
}

// TestCredentialFreeSkip pins the set of YAML functions that credential-free secret listing must
// skip. These all perform an authenticated backend read; if any were evaluated with auth disabled
// (as `secret list` does) the read would fall back to the default AWS chain and fail at the EC2
// IMDS endpoint. Referencing the u.AtmosYamlFunc* constants makes a rename a compile error here.
func TestCredentialFreeSkip(t *testing.T) {
	got := credentialFreeSkip()

	want := []string{
		strings.TrimPrefix(u.AtmosYamlFuncSecret, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStore, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncStoreGet, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),
	}
	require.ElementsMatch(t, want, got, "credentialFreeSkip must skip every credentialed read function")

	// skipFunc compares against the tag with the "!" trimmed, so tokens must be bare.
	for _, tok := range got {
		require.NotEmpty(t, tok)
		assert.Falsef(t, strings.HasPrefix(tok, "!"), "skip token %q must not include the ! prefix", tok)
	}

	// The two functions that triggered the IMDS-fallback regression must be present.
	assert.Contains(t, got, "terraform.state")
	assert.Contains(t, got, "terraform.output")
}
