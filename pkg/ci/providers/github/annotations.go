package github

import (
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Annotate implements provider.Annotator by emitting one GitHub Actions
// workflow annotation command per finding. GitHub renders these inline on the
// pull request diff (and in the run's annotations list) — the "non-CodeQL" path
// that needs no GitHub Advanced Security.
//
// Workflow commands are written to stderr (the UI channel) via pkg/ui, NOT to
// stdout (the data channel). The GitHub Actions runner parses workflow commands
// from the step's combined log stream, which captures both stdout and stderr,
// so annotations still render when written to stderr. Writing them to stdout
// would corrupt the structured data (JSON/YAML) that downstream consumers read
// from stdout — for example, legacy actions like
// cloudposse/github-action-atmos-get-settings capture the stdout of
// `atmos describe ...` and parse it as JSON, which fails on a stray
// `::warning ...` line. See issue #3309 and
// docs/fixes/2026-10-07-ci-legacy-action-stdout-pollution.md.
//
// Unlike log-group markers — whose `::group::`/`::endgroup::` must bracket
// stdout content and therefore cannot move to stderr (the runner does not
// guarantee ordering across the two streams) — annotations are standalone and
// render correctly from stderr. Writes go through pkg/ui, which routes to
// stderr with secret masking and logs (does not return) any write failure,
// matching the fire-and-forget contract of every other UI-channel write in
// Atmos; a dropped diagnostic annotation is never worth failing the command.
func (p *Provider) Annotate(annotations []provider.Annotation) error {
	defer perf.Track(nil, "github.Provider.Annotate")()

	for i := range annotations {
		ui.Writeln(formatAnnotation(&annotations[i]))
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
