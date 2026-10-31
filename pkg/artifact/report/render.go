package report

import (
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxBytes is the largest summary the renderer produces. GitHub rejects a step summary
	// over 1 MiB, so the default leaves headroom below it for content other steps already wrote.
	DefaultMaxBytes = 1024*1024 - 64*1024

	// The compactDiagnostics constant is the number of diagnostics a size-reduced report keeps per operation.
	compactDiagnostics = 3

	// The reportHeading constant is the heading of the summary.
	reportHeading = "Artifacts"

	// The closeAllowance constant is the number of unclosed collapsible sections a hard truncation closes.
	closeAllowance = 4

	// The codeFence constant is the Markdown fence a hard truncation must leave balanced.
	codeFence = "```"

	detailsOpen  = "<details>"
	detailsClose = "</details>"
)

// truncationNote is appended after a hard truncation. It is visible in the rendered summary.
const truncationNote = "\n\n> [!WARNING]\n> This summary was cut to fit the size limit. The workflow log has the full results.\n"

// templateSet loads the frame and per-action templates once, in the loader's order: an explicit
// ci.templates.artifacts.<name> file, then <base_path>/artifacts/<name>.md, then the embedded
// default. A configured file that cannot be read is an error, never a silent fallback.
type templateSet struct {
	loader   *templates.Loader
	defaults fs.FS
	frame    string
	sections map[string]string
}

// newTemplateSet loads the frame template.
func newTemplateSet(config *schema.AtmosConfiguration) (*templateSet, error) {
	set := &templateSet{loader: templates.NewLoader(config), defaults: defaultTemplates{}, sections: map[string]string{}}
	frame, err := set.loader.Load(componentType, frameCommand, set.defaults)
	if err != nil {
		return nil, renderError(err)
	}
	set.frame = frame
	return set, nil
}

// section returns the template of one action's section, loading it on first use.
func (s *templateSet) section(action string) (string, error) {
	if action == "" || !isKnownAction(action) {
		action = "inspect"
	}
	if content, ok := s.sections[action]; ok {
		return content, nil
	}
	content, err := s.loader.Load(componentType, action, s.defaults)
	if err != nil {
		return "", renderError(err)
	}
	s.sections[action] = content
	return content, nil
}

// renderAll renders the report, degrading it in visible steps until it fits maxBytes: first the
// bulky details are dropped, then trailing operation sections, and only then is the text cut.
func renderAll(set *templateSet, report *Report, operations []*Operation, maxBytes int) (string, error) {
	shown, compact := len(operations), false
	for {
		markdown, err := renderOnce(set, report, operations, shown, compact)
		if err != nil {
			return "", err
		}
		if len(markdown) <= maxBytes {
			return markdown, nil
		}
		switch {
		case !compact:
			compact = true
		case shown > 0:
			shown /= 2
		default:
			return truncateMarkdown(markdown, maxBytes), nil
		}
	}
}

// renderOnce renders the frame with the first shown sections, optionally with compacted details.
func renderOnce(set *templateSet, report *Report, operations []*Operation, shown int, compact bool) (string, error) {
	view := *report
	view.Sections = make([]string, 0, shown)
	for _, operation := range operations[:shown] {
		current := operation
		if compact {
			current = compacted(operation)
		}
		section, err := renderSection(set, current)
		if err != nil {
			return "", err
		}
		view.Sections = append(view.Sections, section)
	}
	view.Omitted = len(operations) - shown
	view.Notice = degradationNotice(view.Omitted, len(operations), compact)

	frame, err := set.loader.RenderStrict(set.frame, &view)
	if err != nil {
		return "", renderError(err)
	}
	return atmosio.MaskString(frame), nil
}

// renderSection renders one operation's section with the template of its action.
func renderSection(set *templateSet, operation *Operation) (string, error) {
	content, err := set.section(operation.Action)
	if err != nil {
		return "", err
	}
	rendered, err := set.loader.RenderStrict(content, operation)
	if err != nil {
		return "", renderError(err)
	}
	return strings.TrimRight(rendered, "\n"), nil
}

// compacted returns a copy of an operation without the bulky details. The counts that remain say
// what was left out.
func compacted(operation *Operation) *Operation {
	copied := *operation
	copied.Inventory = nil
	copied.Labels = nil
	copied.Description = ""
	if copied.Collection != nil {
		collection := *copied.Collection
		collection.Artifacts = nil
		collection.More = 0
		copied.Collection = &collection
	}
	if len(copied.Diagnostics) > compactDiagnostics {
		copied.DiagnosticsMore += len(copied.Diagnostics) - compactDiagnostics
		copied.Diagnostics = copied.Diagnostics[:compactDiagnostics]
	}
	return &copied
}

// degradationNotice words what a size-reduced report left out. It is empty for a complete report.
func degradationNotice(omitted, total int, compact bool) string {
	switch {
	case omitted > 0:
		return fmt.Sprintf("%d of %d operation sections were left out to keep this summary within the size limit. The table above lists every operation.", omitted, total)
	case compact:
		return "Inventory, metadata, and long diagnostic lists were left out to keep this summary within the size limit."
	default:
		return ""
	}
}

// truncateMarkdown cuts the text at a rune boundary below maxBytes, closes the code fence and
// collapsible sections the cut left open, and appends a visible note.
func truncateMarkdown(markdown string, maxBytes int) string {
	reserved := len(truncationNote) + len(codeFence) + 1 + closeAllowance*(len(detailsClose)+1)
	budget := max(maxBytes-reserved, 0)
	if budget < len(markdown) {
		cut := budget
		for cut > 0 && !utf8.RuneStart(markdown[cut]) {
			cut--
		}
		markdown = markdown[:cut]
	}
	if strings.Count(markdown, codeFence)%2 == 1 {
		markdown += "\n" + codeFence
	}
	unclosed := strings.Count(markdown, detailsOpen) - strings.Count(markdown, detailsClose)
	for index := 0; index < unclosed && index < closeAllowance; index++ {
		markdown += "\n" + detailsClose
	}
	return markdown + truncationNote
}

// renderError wraps a template loading or rendering failure with the report render sentinel.
func renderError(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrArtifactReportRender, err)
}
