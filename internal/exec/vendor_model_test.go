package exec

import (
	"bytes"
	"errors"
	stdio "io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestVendorFailureError(t *testing.T) {
	// Regression test: the vendor error must contain a descriptive explanation
	// listing the failed component names, not just a bare integer count.
	t.Run("single failed component", func(t *testing.T) {
		err := vendorFailureError(1, 3, []string{"my-vpc"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.NotContains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "Failed to vendor 1 of 3 components")
	})

	t.Run("multiple failed components", func(t *testing.T) {
		err := vendorFailureError(3, 5, []string{"my-vpc", "my-rds", "my-s3"})

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVendorComponents)

		config := errUtils.DefaultFormatterConfig()
		formatted := errUtils.Format(err, config)

		assert.NotContains(t, formatted, "## Explanation")
		assert.Contains(t, formatted, "my-vpc")
		assert.Contains(t, formatted, "my-rds")
		assert.Contains(t, formatted, "my-s3")
		assert.Contains(t, formatted, "Failed to vendor 3 of 5 components")
	})
}

func TestHandleInstalledPkgMsg_TracksFailedNames(t *testing.T) {
	// Verify that handleInstalledPkgMsg appends failed package names.
	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc"},
			{name: "rds"},
		},
		index: 0,
		isTTY: false,
	}

	// Simulate a failed install message.
	msg := &installedPkgMsg{
		err:  errors.New("download failed"),
		name: "vpc",
	}
	m.handleInstalledPkgMsg(msg)

	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)

	// Simulate a second package succeeding.
	m.index = 1
	msg2 := &installedPkgMsg{name: "rds"}
	m.handleInstalledPkgMsg(msg2)

	// Failed count should not change.
	assert.Equal(t, 1, m.failedPkg)
	assert.Equal(t, []string{"vpc"}, m.failedPkgNames)
}

func TestHandleInstalledPkgMsg_NonTTYStatusOutput(t *testing.T) {
	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	t.Run("non-final success logs package status", func(t *testing.T) {
		stderr.Reset()
		m := &modelVendor{
			packages: []pkgVendor{
				{name: "vpc", version: "1.0.0"},
				{name: "rds", version: "2.0.0"},
			},
			isTTY: false,
		}

		_, cmd := m.handleInstalledPkgMsg(&installedPkgMsg{name: "vpc"})

		assert.NotNil(t, cmd)
		assert.Equal(t, 1, m.index)
		assert.Contains(t, atmosansi.Strip(stderr.String()), "vpc (1.0.0)")
	})

	t.Run("final failure logs failed package and summary", func(t *testing.T) {
		stderr.Reset()
		m := &modelVendor{
			packages: []pkgVendor{
				{name: "vpc", version: "1.0.0"},
			},
			isTTY: false,
		}

		_, cmd := m.handleInstalledPkgMsg(&installedPkgMsg{
			name: "vpc",
			err:  errors.New("download failed"),
		})

		assert.NotNil(t, cmd)
		assert.True(t, m.done)
		output := atmosansi.Strip(stderr.String())
		assert.Contains(t, output, "Failed to vendor vpc")
		assert.Contains(t, output, "vpc (1.0.0)")
		assert.Contains(t, output, "Vendored components (success: 0, failed: 1)")
	})
}

func TestLogNonTTYFinalStatus_DryRunSuccessSummary(t *testing.T) {
	stderr, cleanup := setupVendorModelTestUI(t)
	defer cleanup()

	m := &modelVendor{
		packages: []pkgVendor{
			{name: "vpc", version: "1.0.0"},
		},
		dryRun: true,
		isTTY:  false,
	}

	m.logNonNTYFinalStatus(m.packages[0], false)

	output := atmosansi.Strip(stderr.String())
	assert.Contains(t, output, "vpc (1.0.0)")
	assert.Contains(t, output, "Done! Dry run completed. No components vendored")
	assert.Contains(t, output, "Vendored components (success: 1)")
}

type vendorModelTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *vendorModelTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *vendorModelTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *vendorModelTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *vendorModelTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *vendorModelTestStreams) RawError() stdio.Writer  { return ts.stderr }

func setupVendorModelTestUI(t *testing.T) (stderr *bytes.Buffer, cleanup func()) {
	t.Helper()

	stdout := &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &vendorModelTestStreams{
		stdin:  strings.NewReader(""),
		stdout: stdout,
		stderr: stderr,
	}

	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)

	ui.InitFormatter(ioCtx)

	return stderr, ui.Reset
}
