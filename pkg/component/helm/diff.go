package helm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	hd "github.com/databus23/helm-diff/v3/diff"
	"github.com/databus23/helm-diff/v3/manifest"
	"github.com/mgutz/ansi"
	"github.com/muesli/termenv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// defaultDiffContextLines is the number of unchanged context lines shown around
// each change in the unified diff.
const defaultDiffContextLines = 3

func init() {
	// helm-diff colorizes its output via mgutz/ansi without any TTY detection.
	// Disable colors so the produced diff is plain text suitable for secret
	// masking, piping, and rendering inside a GitHub `diff` fenced block in CI
	// job summaries. Atmos owns presentation through its own UI layer.
	ansi.DisableColors(true)
}

// unifiedDiff computes a unified diff between two rendered manifests using the
// helm-diff library. Both inputs are raw manifest strings, so this is fully
// offline — no cluster or Helm release object is required. Secret values are
// redacted (ShowSecrets is false) so sensitive data never reaches stdout or a CI
// job summary. It returns the diff text, whether any changes were detected, and
// an error.
func unifiedDiff(oldManifest, newManifest, namespace string, contextLines int) (diff string, changed bool, err error) {
	defer perf.Track(nil, "helm.unifiedDiff")()

	// helm-diff's report generation panics on internal formatting errors. Recover
	// and convert to a normal error so a malformed manifest cannot crash Atmos.
	defer func() {
		if r := recover(); r != nil {
			diff = ""
			changed = false
			err = fmt.Errorf("%w: %v", errUtils.ErrHelmDiffFailed, r)
		}
	}()

	if contextLines <= 0 {
		contextLines = defaultDiffContextLines
	}

	// normalizeManifests=true canonicalizes object ordering/whitespace so the diff
	// reflects real changes rather than formatting noise.
	oldIndex := manifest.Parse([]byte(oldManifest), namespace, true)
	newIndex := manifest.Parse([]byte(newManifest), namespace, true)

	var buf bytes.Buffer
	options := &hd.Options{
		OutputFormat:  "diff",
		OutputContext: contextLines,
		ShowSecrets:   false,
	}
	changed = hd.Manifests(oldIndex, newIndex, options, &buf)

	return buf.String(), changed, nil
}

func colorizeUnifiedDiff(diffText string) string {
	if diffText == "" || ui.GetColorProfile() == termenv.Ascii {
		return diffText
	}

	added := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	removed := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	hunk := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var b strings.Builder
	for _, line := range strings.SplitAfter(diffText, "\n") {
		if line == "" {
			continue
		}

		hasNewline := strings.HasSuffix(line, "\n")
		text := strings.TrimSuffix(line, "\n")
		text = colorizeUnifiedDiffLine(text, &added, &removed, &hunk, &meta)

		b.WriteString(text)
		if hasNewline {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func colorizeUnifiedDiffLine(text string, added, removed, hunk, meta *lipgloss.Style) string {
	switch {
	case strings.HasPrefix(text, "+") && !strings.HasPrefix(text, "+++"):
		return added.Render(text)
	case strings.HasPrefix(text, "-") && !strings.HasPrefix(text, "---"):
		return removed.Render(text)
	case strings.HasPrefix(text, "@@"):
		return hunk.Render(text)
	case isUnifiedDiffMetadataLine(text):
		return meta.Render(text)
	default:
		return text
	}
}

func isUnifiedDiffMetadataLine(text string) bool {
	return strings.HasPrefix(text, "diff ") ||
		strings.HasPrefix(text, "index ") ||
		strings.HasPrefix(text, "---") ||
		strings.HasPrefix(text, "+++")
}
