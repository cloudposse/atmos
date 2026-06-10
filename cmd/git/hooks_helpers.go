package git

import (
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// hookNotConfiguredErr builds an ErrGitHookNotConfigured error with a hint
// listing configured hook names.
func hookNotConfiguredErr(hookName string, hooks map[string]schema.GitHookEntry) error {
	b := errUtils.Build(errUtils.ErrGitHookNotConfigured).
		WithHintf("Hook %q is not configured under git.hooks in atmos.yaml.", hookName).
		WithExitCode(2)

	if len(hooks) == 0 {
		b = b.WithHint("No hooks are currently configured under git.hooks.")
	} else {
		names := sortedKeys(hooks)
		b = b.WithHintf("Configured hooks: %s.", strings.Join(names, ", "))
	}

	return b.Err()
}

// sortedKeys returns the map keys in ascending order.
func sortedKeys(m map[string]schema.GitHookEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
