package github

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
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

func TestProvider_Annotate_WritesOneLinePerFinding(t *testing.T) {
	var buf bytes.Buffer
	prev := annotationsOut
	annotationsOut = &buf
	defer func() { annotationsOut = prev }()

	p := NewProvider()
	err := p.Annotate([]provider.Annotation{
		{Path: "a.tf", StartLine: 1, Level: provider.AnnotationError, Title: "R1", Message: "first"},
		{Path: "b.tf", StartLine: 2, Level: provider.AnnotationWarning, Title: "R2", Message: "second"},
	})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "::error file=a.tf,line=1,title=R1::first", lines[0])
	assert.Equal(t, "::warning file=b.tf,line=2,title=R2::second", lines[1])
}

// failWriter fails every write, standing in for a broken log stream.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// A write failure on the annotation stream surfaces as an error rather than
// being silently dropped.
func TestProvider_Annotate_WriteErrorPropagates(t *testing.T) {
	prev := annotationsOut
	annotationsOut = failWriter{}
	defer func() { annotationsOut = prev }()

	p := NewProvider()
	err := p.Annotate([]provider.Annotation{
		{Path: "a.tf", StartLine: 1, Level: provider.AnnotationError, Title: "R1", Message: "first"},
	})
	require.Error(t, err)
}
