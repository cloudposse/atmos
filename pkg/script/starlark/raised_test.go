package starlark

import (
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRaisedFailureMessage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, message string }{
		{"check", `fail("Formatting changed; review fixes and restage")`, "Formatting changed; review fixes and restage"},
		{"arguments", `fail("expected", 42, sep = ": ")`, "expected: 42"},
		{"builder", `errors.build("Deployment blocked").with_hint("Fix credentials").fail()`, "Deployment blocked"},
		{"task", "def check():\n    fail(\"Formatting changed\")\nsteps.parallel(functions = [check])", `task "check[0]": Formatting changed`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			require.EqualError(t, err, tc.message)
			require.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "test.star")
			if tc.name == "builder" {
				require.Contains(t, cockroach.GetAllHints(err), "Fix credentials")
			}
		})
	}
}

func TestRaisedFailureDoesNotHideInterpreterErrors(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`fail("bad", sep = 42)`,
		`fail("bad", unexpected = True)`,
		`{}["missing"]`,
		"def check():\n    fail(\"check rejected\")\ndef broken():\n    return 1 // 0\nsteps.parallel(functions = [check, broken])",
	} {
		_, err := runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlark)
		require.Contains(t, err.Error(), prefixText)
	}
}
