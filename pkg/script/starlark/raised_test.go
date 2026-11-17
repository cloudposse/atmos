package starlark

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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
			require.NotContains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "Traceback")
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

// Changing the global logger requires a non-parallel test.
func TestRaisedFailureTracebackIsDebugOnly(t *testing.T) {
	var output bytes.Buffer
	previous := log.Default()
	log.SetDefault(newTestLogger(&output, log.DebugLevel))
	t.Cleanup(func() { log.SetDefault(previous) })
	_, err := runSource(t, `fail("Already in a development shell")`)
	require.EqualError(t, err, "Already in a development shell")
	require.Empty(t, cockroach.GetAllDetails(err))
	require.Contains(t, output.String(), "Starlark failure traceback")
	require.Contains(t, output.String(), "test.star")
	var eval *starlark.EvalError
	require.True(t, errors.As(err, &eval), "retain the interpreter error for programmatic diagnostics")
	require.Contains(t, eval.Backtrace(), "Already in a development shell")
}
