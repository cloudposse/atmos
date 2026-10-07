package github

import (
	"bytes"
	"errors"
	stdio "io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestFormatAnnotation(t *testing.T) {
	tests := []struct {
		name string
		in   provider.Annotation
		want string
	}{
		{
			name: "error with file/line/title",
			in:   provider.Annotation{Path: "main.tf", StartLine: 6, Level: provider.AnnotationError, Title: "CKV_AWS_21", Message: "Ensure versioning is enabled"},
			want: "::error file=main.tf,line=6,title=CKV_AWS_21::Ensure versioning is enabled",
		},
		{
			name: "warning level",
			in:   provider.Annotation{Path: "a.tf", StartLine: 1, Level: provider.AnnotationWarning, Title: "R1", Message: "msg"},
			want: "::warning file=a.tf,line=1,title=R1::msg",
		},
		{
			name: "line 0 omits line property (file-level annotation)",
			in:   provider.Annotation{Path: "a.tf", StartLine: 0, Level: provider.AnnotationWarning, Message: "no line"},
			want: "::warning file=a.tf::no line",
		},
		{
			name: "endLine included when >= startLine",
			in:   provider.Annotation{Path: "a.tf", StartLine: 3, EndLine: 5, Level: provider.AnnotationNotice, Message: "range"},
			want: "::notice file=a.tf,line=3,endLine=5::range",
		},
		{
			name: "unknown level falls back to warning",
			in:   provider.Annotation{Path: "a.tf", StartLine: 1, Level: provider.AnnotationLevel("bogus"), Message: "x"},
			want: "::warning file=a.tf,line=1::x",
		},
		{
			name: "message escaping (% and newline)",
			in:   provider.Annotation{Path: "a.tf", StartLine: 1, Level: provider.AnnotationError, Message: "50% off\nsecond line"},
			want: "::error file=a.tf,line=1::50%25 off%0Asecond line",
		},
		{
			name: "property escaping (comma and colon in title/path)",
			in:   provider.Annotation{Path: "a:b.tf", StartLine: 1, Level: provider.AnnotationError, Title: "R,1", Message: "m"},
			want: "::error file=a%3Ab.tf,line=1,title=R%2C1::m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatAnnotation(&tt.in))
		})
	}
}

// testStreams is a minimal Streams implementation for capturing data-channel
// output in tests, mirroring pkg/data/data_test.go's testStreams helper.
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

// errWriter fails every write, standing in for a broken output stream. Shared
// with other tests in this package (e.g. log_group_test.go).
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// initCapture wires the data channel (stdout) and the UI formatter (stderr) to
// separate captured buffers sharing a single io.Context, so a test can assert
// exactly which channel each annotation line lands on. NO_COLOR forces plain
// text so the workflow-command lines are matched byte-for-byte regardless of
// the host's own color-support detection (CI runners are detected as
// color-capable even when writing to a buffer).
func initCapture(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")

	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	return stdout, stderr
}

// Annotations must be written to stderr (the UI channel), one workflow command
// per finding, so the GitHub runner still renders them while stdout (the data
// channel) stays pristine for downstream JSON/YAML consumers. Regression guard
// for #3309.
func TestProvider_Annotate_WritesOneLinePerFindingToStderr(t *testing.T) {
	stdout, stderr := initCapture(t)

	p := NewProvider()
	err := p.Annotate([]provider.Annotation{
		{Path: "a.tf", StartLine: 1, Level: provider.AnnotationError, Title: "R1", Message: "first"},
		{Path: "b.tf", StartLine: 2, Level: provider.AnnotationWarning, Title: "R2", Message: "second"},
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "::error file=a.tf,line=1,title=R1::first", lines[0])
	assert.Equal(t, "::warning file=b.tf,line=2,title=R2::second", lines[1])

	// The data channel (stdout) must carry none of the annotation output.
	assert.Empty(t, stdout.String(), "annotations must not pollute stdout (the data channel)")
}

// A nil/empty annotation slice writes nothing and does not error.
func TestProvider_Annotate_NoAnnotations(t *testing.T) {
	stdout, stderr := initCapture(t)

	p := NewProvider()
	require.NoError(t, p.Annotate(nil))

	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}
