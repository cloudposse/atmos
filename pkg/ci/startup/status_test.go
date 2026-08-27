package startup

import (
	"bytes"
	"context"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// testStreams is a minimal iolib.Streams implementation for capturing UI output.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// initTestUI wires the UI formatter to a captured stderr buffer.
func initTestUI(t *testing.T) *bytes.Buffer {
	t.Helper()

	stderr := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)

	return stderr
}

// fakeProvider is a minimal ci provider.Provider whose Detect() result is
// controlled by the test, used to force ci.IsCI() true/false without
// depending on the host environment's real CI variables.
type fakeProvider struct {
	detected bool
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Detect() bool { return f.detected }

func (f *fakeProvider) Context() (*provider.Context, error) { return &provider.Context{}, nil }

func (f *fakeProvider) GetStatus(_ context.Context, _ provider.StatusOptions) (*provider.Status, error) {
	return &provider.Status{}, nil
}

func (f *fakeProvider) CreateCheckRun(_ context.Context, _ *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	return &provider.CheckRun{}, nil
}

func (f *fakeProvider) UpdateCheckRun(_ context.Context, _ *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	return &provider.CheckRun{}, nil
}

func (f *fakeProvider) PostComment(_ context.Context, _ *provider.PostCommentOptions) (*provider.Comment, error) {
	return &provider.Comment{}, nil
}

func (f *fakeProvider) OutputWriter() provider.OutputWriter { return nil }

func (f *fakeProvider) ResolveBase() (*provider.BaseResolution, error) { return nil, nil }

func TestPrintStartupStatus_NoOpOutsideCI(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(&fakeProvider{detected: false})

	stderr := initTestUI(t)

	PrintStartupStatus(&schema.AtmosConfiguration{})

	assert.Empty(t, stderr.String())
}

func TestPrintStartupStatus_PrintsWhenInCI(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	defer restore()
	ci.Register(&fakeProvider{detected: true})

	stderr := initTestUI(t)

	PrintStartupStatus(&schema.AtmosConfiguration{})

	assert.Contains(t, stderr.String(), "Atmos version")
}

func TestPrintStatusLines(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		legacyRepo  string
		wantLines   []string
		wantAbsent  []string
	}{
		{
			name: "ci enabled, pro enabled",
			atmosConfig: &schema.AtmosConfiguration{
				CI: schema.CIConfig{Enabled: true},
				Settings: schema.AtmosSettings{
					Pro: schema.ProSettings{WorkspaceID: "ws-123"},
				},
			},
			wantLines: []string{
				"Atmos version",
				"Atmos CI is enabled; learn more at https://atmos.tools/ci",
				"Atmos Pro is enabled; learn more at https://atmos.tools/pro",
			},
			wantAbsent: []string{"Atmos CI is disabled", "Atmos Pro is disabled", "legacy action"},
		},
		{
			name: "ci enabled, pro disabled",
			atmosConfig: &schema.AtmosConfiguration{
				CI: schema.CIConfig{Enabled: true},
			},
			wantLines: []string{
				"Atmos CI is enabled; learn more at https://atmos.tools/ci",
				"Atmos Pro is disabled; learn more at https://atmos.tools/pro",
			},
		},
		{
			name:        "ci disabled, pro disabled",
			atmosConfig: &schema.AtmosConfiguration{},
			wantLines: []string{
				"Atmos CI is disabled; learn more at https://atmos.tools/ci",
				"Atmos Pro is disabled; learn more at https://atmos.tools/pro",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTION_REPOSITORY", "")
			stderr := initTestUI(t)

			printStatusLines(tt.atmosConfig)

			output := stderr.String()
			for _, want := range tt.wantLines {
				assert.Contains(t, output, want)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, output, absent)
			}
		})
	}
}

func TestPrintStatusLines_LegacyActionWarning(t *testing.T) {
	t.Setenv("GITHUB_ACTION_REPOSITORY", "cloudposse/github-action-atmos-terraform-plan")
	stderr := initTestUI(t)

	printStatusLines(&schema.AtmosConfiguration{})

	assert.Contains(t, stderr.String(), "Detected legacy action cloudposse/github-action-atmos-terraform-plan")
	assert.Contains(t, stderr.String(), "https://atmos.tools/ci")
}

func TestPrintStatusLines_NoLegacyActionWarningWhenUnset(t *testing.T) {
	t.Setenv("GITHUB_ACTION_REPOSITORY", "")
	stderr := initTestUI(t)

	printStatusLines(&schema.AtmosConfiguration{})

	assert.NotContains(t, stderr.String(), "legacy action")
}
