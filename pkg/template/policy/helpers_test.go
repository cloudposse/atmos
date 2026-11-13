package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmospolicy "github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/template/render"
)

func templatePolicy(t *testing.T, kind, rule string, roots []string) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	manifest := fmt.Sprintf("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: template}\nspec:\n  policies:\n    guard:\n      on: %s\n      action: deny\n      rule: %q\n", kind, rule)
	if len(roots) > 0 {
		manifest += "      allowed_roots:\n"
		for _, root := range roots {
			manifest += fmt.Sprintf("        - %q\n", root)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
}

// templatePolicyInvalid points ATMOS_POLICY_DIR at an empty directory so loading the bundle fails.
func templatePolicyInvalid(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_POLICY_DIR", t.TempDir())
}

// newTemplateData builds gomplate datasources from alias=URL definitions and optional header definitions.
func newTemplateData(t *testing.T, sources, headers []string) *render.Data {
	t.Helper()
	d, err := render.NewData(sources, headers)
	require.NoError(t, err)
	return d
}

// failPolicyEnabled replaces the capability lookup with one that fails when shouldFail reports true for the
// kind and the 1-based call number. Other calls report the capability as enabled. The production lookup is
// restored when the test ends.
func failPolicyEnabled(t *testing.T, shouldFail func(kind string, call int) bool) {
	t.Helper()

	var calls atomic.Int32
	guard.SetEnabled(func(kind string) (bool, error) {
		if shouldFail(kind, int(calls.Add(1))) {
			return false, fmt.Errorf("%w: capability lookup failed", errUtils.ErrPolicyEvaluation)
		}
		return true, nil
	})
	t.Cleanup(func() {
		guard.SetEnabled(func(kind string) (bool, error) { return admission.HasRules(atmospolicy.InputKind(kind)) })
	})
}
