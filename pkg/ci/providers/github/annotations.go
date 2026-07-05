package github

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// workflowCommandsOut is where GitHub workflow commands are written. It
// defaults to os.Stdout because the runner parses workflow commands from the
// step log stream, and is overridable in tests. Package-level rather than a
// field so the lazily-constructed Provider needs no extra wiring.
//
//nolint:gochecknoglobals // test seam for stdout-bound workflow commands.
var workflowCommandsOut io.Writer = os.Stdout

// Annotate implements provider.Annotator by emitting one GitHub Actions
// workflow annotation command per finding. GitHub renders these inline on the
// pull request diff (and in the run's annotations list) — the "non-CodeQL" path
// that needs no GitHub Advanced Security.
func (p *Provider) Annotate(annotations []provider.Annotation) error {
	defer perf.Track(nil, "github.Provider.Annotate")()

	for i := range annotations {
		if _, err := fmt.Fprintln(workflowCommandsOut, formatAnnotation(&annotations[i])); err != nil {
			return err
		}
	}
	return nil
}

// formatAnnotation renders one annotation as a GitHub workflow command:
//
//	::error file=main.tf,line=6,title=CKV_AWS_21::Ensure versioning is enabled
//
// When StartLine is 0 (unknown), the line/endLine properties are omitted so the
// annotation anchors at the file level. The level falls back to "warning" for
// any unrecognized value.
func formatAnnotation(a *provider.Annotation) string {
	level := string(a.Level)
	switch a.Level {
	case provider.AnnotationError, provider.AnnotationWarning, provider.AnnotationNotice:
	default:
		level = string(provider.AnnotationWarning)
	}

	var props []string
	if a.Path != "" {
		props = append(props, "file="+escapeProperty(a.Path))
	}
	if a.StartLine > 0 {
		props = append(props, "line="+strconv.Itoa(a.StartLine))
		if a.EndLine >= a.StartLine {
			props = append(props, "endLine="+strconv.Itoa(a.EndLine))
		}
	}
	if a.Title != "" {
		props = append(props, "title="+escapeProperty(a.Title))
	}

	var b strings.Builder
	b.WriteString("::")
	b.WriteString(level)
	if len(props) > 0 {
		b.WriteString(" ")
		b.WriteString(strings.Join(props, ","))
	}
	b.WriteString("::")
	b.WriteString(escapeData(a.Message))
	return b.String()
}

// escapeData escapes a workflow-command message per GitHub's spec.
func escapeData(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}

// escapeProperty escapes a workflow-command property value. Property values
// additionally escape "," and ":" on top of the data escapes.
func escapeProperty(s string) string {
	s = escapeData(s)
	s = strings.ReplaceAll(s, ",", "%2C")
	s = strings.ReplaceAll(s, ":", "%3A")
	return s
}
