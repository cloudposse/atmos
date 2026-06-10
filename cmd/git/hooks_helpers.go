package git

import (
	"path/filepath"
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

// validateHookShimName rejects hook names that could escape .git/hooks.
func validateHookShimName(hookName string) error {
	if hookName == "" || hookName == "." || hookName == ".." || filepath.IsAbs(hookName) || strings.ContainsAny(hookName, `/\`) {
		return invalidHookShimNameErr(hookName)
	}

	for _, r := range hookName {
		if !isHookShimNameChar(r) {
			return invalidHookShimNameErr(hookName)
		}
	}

	return nil
}

func isHookShimNameChar(r rune) bool {
	return r == '.' || r == '_' || r == '-' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

func invalidHookShimNameErr(hookName string) error {
	return errUtils.Build(errUtils.ErrInvalidConfig).
		WithHintf("Hook name %q must be a simple filename using only letters, digits, '.', '_' or '-'.", hookName).
		WithHint("Hook names must not be absolute paths, '.', '..', or contain path separators.").
		WithExitCode(2).
		Err()
}
