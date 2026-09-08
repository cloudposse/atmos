package secret

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestRunSecretSet_StackFlagNotShadowedByEarlierCommand guards against the flake that failed
// `[race] non-acceptance test suite` on main and PR runs under -shuffle: parseScopeStack used to
// write the resolved stack into viper's override layer (viper.Set), which outranks every later
// flag parse, so once any secret command had run with --stack prod, a following command's
// --stack dev was silently ignored and its global-scope lookup found nothing. Two commands in one
// process must each see their own --stack.
func TestRunSecretSet_StackFlagNotShadowedByEarlierCommand(t *testing.T) {
	svc := newFakeSecretService()
	svc.declared = map[string]bool{"SHARED_CLIENT_SECRET": true}
	svc.scopes = map[string]secrets.Scope{"SHARED_TOKEN": secrets.ScopeGlobal}
	installService(t, svc, nil)

	require.NoError(t, runSecretSubcommand(t, "import", "SHARED_CLIENT_SECRET",
		"--from-stack", "atmos", "--from-component", "shared",
		"--stack", "prod", "--component", "api"))

	originalLoadService := loadServiceFn
	var loadedScope secretScope
	loadServiceFn = func(scope secretScope) (secretService, error) {
		loadedScope = scope
		return originalLoadService(scope)
	}
	t.Cleanup(func() { loadServiceFn = originalLoadService })
	overrideEnumerateScopes(t, []scopeEntry{
		{
			Stack:     "dev",
			Component: "example-service",
			Section: secretDeclarationSection("SHARED_TOKEN", map[string]any{
				"store": "example-secrets",
				"scope": "global",
			}),
		},
	}, nil)

	require.NoError(t, runSecretSubcommand(t, "set", "SHARED_TOKEN=v1", "--stack", "dev"))
	assert.Equal(t, "dev", loadedScope.Stack, "the second command must use its own --stack, not the first command's")
	require.Len(t, svc.setCalls, 1)
}

// TestAdoptPromptedStack_VisibleThroughFlagBinding guards the reason the override existed: a
// stack chosen at the interactive prompt must still be visible to the component completion,
// which reads the selected stack through viper's binding to the --stack flag. Setting the flag
// itself (not a viper override) satisfies that without leaking across commands.
func TestAdoptPromptedStack_VisibleThroughFlagBinding(t *testing.T) {
	resetSecretFlags(t)
	// Cobra merges inherited persistent flags into a subcommand's flag set lazily; trigger it the
	// same way command execution does so the --stack flag is addressable on setCmd.
	_ = setCmd.InheritedFlags()
	require.NoError(t, secretParser.BindFlagsToViper(setCmd, viper.GetViper()))

	require.NoError(t, adoptPromptedStack(setCmd, "staging"))
	assert.Equal(t, "staging", viper.GetString("stack"))

	// After the flag reset every other test relies on, nothing lingers.
	resetSecretFlags(t)
	assert.Empty(t, viper.GetString("stack"))
}

// TestAdoptPromptedStack_EmptyChoiceIsNoOp guards the non-interactive path: when the prompt
// yields nothing (no TTY, or no stacks to choose from), the flag stays unset so the standard
// required-flag error still fires instead of an empty stack being adopted.
func TestAdoptPromptedStack_EmptyChoiceIsNoOp(t *testing.T) {
	resetSecretFlags(t)
	_ = setCmd.InheritedFlags()

	require.NoError(t, adoptPromptedStack(setCmd, ""))
	assert.False(t, setCmd.Flags().Changed("stack"))
}
