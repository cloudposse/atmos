package secret

import (
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func overrideInteractive(t *testing.T, interactive bool) {
	t.Helper()
	orig := interactiveFn
	interactiveFn = func() bool { return interactive }
	t.Cleanup(func() { interactiveFn = orig })
}

// TestConfirmActionInteractive_NonInteractive proves that without a terminal the confirmation fails
// with a clear error and a `--force` hint instead of the opaque "could not open a new TTY".
func TestConfirmActionInteractive_NonInteractive(t *testing.T) {
	overrideInteractive(t, false)
	overrideRunForm(t, func(*huh.Form) error {
		t.Error("no form must be run without an interactive terminal")
		return nil
	})

	confirmed, err := confirmActionInteractive("Secret `API_KEY` is already set. Update (rotate) it?")
	require.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	assert.False(t, confirmed)
	assert.Contains(t, cockroach.GetAllHints(err), "Pass `--force` to proceed without confirmation")
	assert.Contains(t, cockroach.GetAllDetails(err)[0], "API_KEY")
}

// TestConfirmActionInteractive_Interactive proves the prompt still runs when a terminal exists.
func TestConfirmActionInteractive_Interactive(t *testing.T) {
	overrideInteractive(t, true)
	overrideRunForm(t, accessibleReader(strings.NewReader("y\n")))

	confirmed, err := confirmActionInteractive("Delete?")
	require.NoError(t, err)
	assert.True(t, confirmed)
}

// TestRunSecretSet_NonInteractiveExistingSecretRequiresForce drives the real default confirmation seam
// through `secret set` for an existing secret without a terminal.
func TestRunSecretSet_NonInteractiveExistingSecretRequiresForce(t *testing.T) {
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{{Declaration: secrets.Declaration{Name: "API_KEY"}, Initialized: true}}
	installService(t, svc, nil)
	overrideInteractive(t, false)

	err := runSecretSubcommand(t, "set", "API_KEY=v2", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	assert.Empty(t, svc.setCalls)

	// With --force the confirmation is skipped and the write proceeds.
	require.NoError(t, runSecretSubcommand(t, "set", "API_KEY=v2", "--force", "--stack", "dev", "--component", "api"))
	require.Len(t, svc.setCalls, 1)
}
