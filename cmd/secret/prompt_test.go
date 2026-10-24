package secret

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// overrideRunForm swaps the runForm seam for the duration of a test and restores it via Cleanup.
func overrideRunForm(t *testing.T, fn func(*huh.Form) error) {
	t.Helper()

	orig := runForm
	runForm = fn
	t.Cleanup(func() { runForm = orig })
}

// accessibleReader returns a runForm that drives the real form in accessible mode reading scripted
// input from r. This executes the production prompt body (titles, validators) without a live TTY.
func accessibleReader(r io.Reader) func(*huh.Form) error {
	return func(f *huh.Form) error {
		return f.WithAccessible(true).WithInput(r).WithOutput(io.Discard).Run()
	}
}

func TestConfirmAction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "yes", input: "y\n", want: true},
		{name: "no", input: "n\n", want: false},
		{name: "empty defaults to no", input: "\n", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideRunForm(t, accessibleReader(strings.NewReader(tt.input)))

			got, err := confirmAction("Delete this secret?")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfirmAction_FormError(t *testing.T) {
	boom := errors.New("form boom")
	overrideRunForm(t, func(*huh.Form) error { return boom })

	_, err := confirmAction("Delete?")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "confirmation prompt failed")
}

func TestPromptForSecretValue_FormError(t *testing.T) {
	boom := errors.New("form boom")
	overrideRunForm(t, func(*huh.Form) error { return boom })

	_, err := promptForSecretValue()
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "secret prompt failed")
}

// TestPromptForSecretValue_PTY exercises the real masked-input body. The huh accessible password
// path requires a TTY file descriptor (term.ReadPassword), so a pty provides one. The empty first
// line trips the non-empty validator (ErrMissingInput), then a real value is accepted — covering
// both the validate closure's error branch and the success path.
func TestPromptForSecretValue_PTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty is not supported on Windows")
	}

	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})

	overrideRunForm(t, func(f *huh.Form) error {
		// Output must also go to the tty so term.ReadPassword's prompt writes succeed.
		return f.WithAccessible(true).WithInput(tty).WithOutput(io.Discard).Run()
	})

	// Carriage return is what a terminal sends on Enter; the empty line exercises the validator.
	writeErr := make(chan error, 1)
	go func() {
		_, e := io.WriteString(ptmx, "\rs3cret-value\r")
		writeErr <- e
	}()

	got, err := promptForSecretValue()
	require.NoError(t, err)
	assert.Equal(t, "s3cret-value", got)
	require.NoError(t, <-writeErr)
}

// Compile-time guard: the secret validator references ErrMissingInput; a rename must fail the build.
var _ = errUtils.ErrMissingInput

// Ensure os is referenced even if the pty test is skipped at runtime on unusual platforms.
var _ = os.Stdin
