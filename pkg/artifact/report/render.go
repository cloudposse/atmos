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

	// The lineBreak constant is the newline the renderer trims and appends around blocks.
	lineBreak = "\n"

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
	var removed []string
	if compact {
		removed = compactedDetails(operations[:shown])
	}
	view.Notice = degradationNotice(view.Omitted, len(operations), removed)

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
	return strings.TrimRight(rendered, lineBreak), nil
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
// The removed list names the details compaction actually dropped, so the notice never claims a
// removal that did not happen.
func degradationNotice(omitted, total int, removed []string) string {
	var notices []string
	if omitted > 0 {
		notices = append(notices, fmt.Sprintf("%d of %d operation sections were left out to keep this summary within the size limit. The table above lists every operation.", omitted, total))
	}
	if len(removed) > 0 {
		notices = append(notices, "Details were left out to keep this summary within the size limit: "+strings.Join(removed, ", ")+".")
	}
	return strings.Join(notices, " ")
}

// compactedDetails names the details that compacted() removes from the given operations, in a
// stable order. It is empty when compaction changes nothing, for example when a long summary alone
// made the report too large.
func compactedDetails(operations []*Operation) []string {
	var found compactionFlags
	for _, operation := range operations {
		found.merge(flagsOf(operation))
	}
	var removed []string
	for _, detail := range []struct {
		dropped bool
		name    string
	}{
		{found.inventory, "inventory"},
		{found.metadata, "metadata"},
		{found.references, "artifact references"},
		{found.diagnostics, "long diagnostic lists"},
	} {
		if detail.dropped {
			removed = append(removed, detail.name)
		}
	}
	return removed
}

// compactionFlags records which kinds of detail compaction would drop.
type compactionFlags struct {
	inventory, metadata, references, diagnostics bool
}

// merge adds the flags of another operation.
func (f *compactionFlags) merge(other compactionFlags) {
	f.inventory = f.inventory || other.inventory
	f.metadata = f.metadata || other.metadata
	f.references = f.references || other.references
	f.diagnostics = f.diagnostics || other.diagnostics
}

// flagsOf reports which details of one operation compaction would drop.
func flagsOf(operation *Operation) compactionFlags {
	collection := operation.Collection
	return compactionFlags{
		inventory:   operation.Inventory != nil,
		metadata:    len(operation.Labels) > 0 || operation.Description != "",
		references:  collection != nil && (len(collection.Artifacts) > 0 || collection.More > 0),
		diagnostics: len(operation.Diagnostics) > compactDiagnostics,
	}
}

// truncateMarkdown cuts the text at a rune boundary below maxBytes, closes the code fence and
// collapsible sections the cut left open, and appends a visible note.
//
// A limit smaller than the note leaves no room for report text, so the result is the start of the
// note cut to the limit: the output never exceeds a positive limit.
func truncateMarkdown(markdown string, maxBytes int) string {
	if maxBytes < len(truncationNote) {
		return cutAtRune(strings.TrimLeft(truncationNote, lineBreak), maxBytes)
	}
	reserved := len(truncationNote) + len(codeFence) + 1 + closeAllowance*(len(detailsClose)+1)
	budget := max(maxBytes-reserved, 0)
	markdown = cutAtRune(markdown, budget)
	if strings.Count(markdown, codeFence)%2 == 1 {
		markdown += lineBreak + codeFence
	}
	unclosed := strings.Count(markdown, detailsOpen) - strings.Count(markdown, detailsClose)
	for index := 0; index < unclosed && index < closeAllowance; index++ {
		markdown += lineBreak + detailsClose
	}
	return markdown + truncationNote
}

// cutAtRune returns the longest prefix of text that ends at a rune boundary and is at most limit bytes.
func cutAtRune(text string, limit int) string {
	if limit >= len(text) {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// renderError wraps a template loading or rendering failure with the report render sentinel.
func renderError(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrArtifactReportRender, err)
}
