package auth

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestComponentsArgCompletion(t *testing.T) {
	completions, directive := ComponentsArgCompletion(nil, nil, "")

	// Should return nil completions.
	assert.Nil(t, completions)
	// Should have NoFileComp directive.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentsArgCompletion_WithArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	args := []string{"arg1", "arg2"}

	completions, directive := ComponentsArgCompletion(cmd, args, "prefix")

	// Should still return nil completions.
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestIdentityArgCompletion_FirstArg(t *testing.T) {
	// When called for first arg, should attempt to provide completions.
	// Note: This requires config loading which we can't easily mock here.
	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	completions, directive := IdentityArgCompletion(cmd, args, "")

	// Should return NoFileComp directive.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Completions might be nil if config can't be loaded.
	// That's OK - we're testing the function doesn't panic.
	_ = completions
}

func TestIdentityArgCompletion_SecondArg(t *testing.T) {
	// When called for second+ arg, should not provide completions.
	cmd := &cobra.Command{Use: "test"}
	args := []string{"first-arg"}

	completions, directive := IdentityArgCompletion(cmd, args, "")

	// Should return nil completions.
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestProviderArgCompletion_FirstArg(t *testing.T) {
	// When called for first arg, should attempt to provide completions.
	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	completions, directive := ProviderArgCompletion(cmd, args, "")

	// Should return NoFileComp directive.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Completions might be nil if config can't be loaded.
	_ = completions
}

func TestProviderArgCompletion_SecondArg(t *testing.T) {
	// When called for second+ arg, should not provide completions.
	cmd := &cobra.Command{Use: "test"}
	args := []string{"first-arg"}

	completions, directive := ProviderArgCompletion(cmd, args, "")

	// Should return nil completions.
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestCompletionFunctions_DoNotPanic(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
	}{
		{"ComponentsArgCompletion", ComponentsArgCompletion},
		{"IdentityArgCompletion", IdentityArgCompletion},
		{"ProviderArgCompletion", ProviderArgCompletion},
	}

	cmd := &cobra.Command{Use: "test"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_, _ = tt.fn(cmd, nil, "")
			})
		})
	}
}

func TestCompletionFunctions_WithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		fn     func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)
		prefix string
	}{
		{"ComponentsArgCompletion with prefix", ComponentsArgCompletion, "aws"},
		{"IdentityArgCompletion with prefix", IdentityArgCompletion, "prod"},
		{"ProviderArgCompletion with prefix", ProviderArgCompletion, "sso"},
	}

	cmd := &cobra.Command{Use: "test"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, directive := tt.fn(cmd, nil, tt.prefix)
			// All should return NoFileComp.
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
		})
	}
}

// TestIdentityFlagCompletion covers the no-config branch of the flag-level
// completer (the only one reachable without a real atmos.yaml).
func TestIdentityFlagCompletion(t *testing.T) {
	// chdir to an empty tempdir so cfg.InitCliConfig can't find an atmos.yaml.
	t.Chdir(t.TempDir())

	// BuildConfigAndStacksInfo dereferences cmd to read global flags, so we
	// pass a real cobra.Command. It needs no flags registered — completion
	// must remain robust to missing global flags.
	cmd := &cobra.Command{Use: "completion-test"}

	completions, directive := identityFlagCompletion(cmd, nil, "")
	assert.Nil(t, completions, "no atmos.yaml means no identities")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive,
		"completion must always disable file completion to avoid suggesting filenames")
}

// TestAddIdentityCompletion_NoFlag covers the path where the command has no
// --identity flag — the helper must be a no-op rather than panicking.
func TestAddIdentityCompletion_NoFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test-no-identity"}
	// No identity flag registered.
	assert.NotPanics(t, func() {
		AddIdentityCompletion(cmd)
	})
}
