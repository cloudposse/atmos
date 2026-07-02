package container

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	labelOCIBase        = "org.opencontainers.image."
	labelOCIDescription = labelOCIBase + "description"
	labelOCILicenses    = labelOCIBase + "licenses"
	labelOCIRevision    = labelOCIBase + "revision"
	labelOCISource      = labelOCIBase + "source"
	labelOCITitle       = labelOCIBase + "title"
	labelOCIVersion     = labelOCIBase + "version"
)

// ImageSummaryOptions controls the rich CI Markdown summary for a built or
// pushed container image.
type ImageSummaryOptions struct {
	Image  string
	Digest string
}

// RenderImageSummaryMarkdown renders a GitHub-flavored Markdown job summary for
// a container image. The format intentionally mirrors the summary produced by
// Cloud Posse's Docker build action so native Atmos container builds have the
// same CI polish without an extra GitHub Action.
func RenderImageSummaryMarkdown(info *ImageInfo, opts ImageSummaryOptions) string {
	defer perf.Track(nil, "container.RenderImageSummaryMarkdown")()

	if info == nil {
		return ""
	}
	image := firstNonEmpty(opts.Image, firstString(info.RepoTags))
	labels := info.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n## 🐳 %s", markdownText(image))
	for _, badge := range summaryBadges(info, labels) {
		fmt.Fprintf(&b, "  &nbsp; `%s`", markdownCodeText(badge))
	}
	b.WriteString("\n\n")
	if description := labels[labelOCIDescription]; description != "" {
		b.WriteString(markdownText(description))
		b.WriteString("\n\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(markdownTable([][2]string{
		{"Field", "Value"},
		{"Tag", codeOrNA(summaryTag(image, labels))},
		{"Digest", codeOrNA(summaryDigest(info, opts.Digest))},
		{"Image ID", codeOrNA(info.ID)},
		{"Revision", codeOrNA(labels[labelOCIRevision])},
		{"Source", linkOrText(labels[labelOCISource])},
	}))
	b.WriteString("\n")

	b.WriteString(detailsSection("⚙️ Runtime", markdownTable([][2]string{
		{"Field", "Value"},
		{"Entrypoint", codeOrNA(strings.Join(info.Entrypoint, " "))},
		{"Command", codeOrNA(strings.Join(info.Cmd, " "))},
		{"Stop signal", codeOrNA(firstNonEmpty(info.StopSignal, "n/a"))},
		{"Storage driver", codeOrNA(firstNonEmpty(info.StorageDriver, "n/a"))},
		{"Exposed ports", codeOrNA(joinOrNA(info.ExposedPorts))},
	})))
	b.WriteString("\n")

	if len(info.Env) > 0 {
		b.WriteString(detailsSection("🌱 Environment variables", envTable(info.Env)))
		b.WriteString("\n")
	}
	if len(labels) > 0 {
		b.WriteString(detailsSection("🔖 Labels", labelsTable(labels)))
		b.WriteString("\n")
	}
	if len(info.LayerDigests) > 0 {
		b.WriteString(detailsSection(fmt.Sprintf("📦 Layers (%d)", len(info.LayerDigests)), layersTable(info.LayerDigests)))
		b.WriteString("\n")
	}
	if info.RawInspectJSON != "" {
		b.WriteString(detailsSection("📄 Raw JSON", "```json\n"+info.RawInspectJSON+"\n```"))
		b.WriteString("\n")
	}
	return b.String()
}

func summaryBadges(info *ImageInfo, labels map[string]string) []string {
	badges := []string{}
	if size := humanizeDecimalBytes(info.Size); size != "" {
		badges = append(badges, size)
	}
	if license := labels[labelOCILicenses]; license != "" {
		badges = append(badges, license)
	}
	if info.Architecture != "" {
		badges = append(badges, info.Architecture)
	}
	if info.Os != "" {
		badges = append(badges, info.Os)
	}
	return badges
}

func summaryTag(image string, labels map[string]string) string {
	if version := labels[labelOCIVersion]; version != "" {
		return version
	}
	if tag := imageTag(image); tag != "" {
		return tag
	}
	return image
}

func summaryDigest(info *ImageInfo, pushedDigest string) string {
	if pushedDigest != "" {
		return pushedDigest
	}
	if digest := digestFromRepoDigest(firstString(info.RepoDigests)); digest != "" {
		return digest
	}
	return "n/a"
}

func markdownTable(rows [][2]string) string {
	var b strings.Builder
	for i, row := range rows {
		fmt.Fprintf(&b, "| %s | %s |\n", markdownCell(row[0]), markdownCell(row[1]))
		if i == 0 {
			b.WriteString("|---|---|\n")
		}
	}
	return b.String()
}

func envTable(env []string) string {
	rows := [][2]string{{"Variable", "Value"}}
	for _, item := range sortedStrings(env) {
		key, value, _ := strings.Cut(item, "=")
		rows = append(rows, [2]string{codeOrNA(key), codeOrNA(value)})
	}
	return markdownTable(rows)
}

func labelsTable(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := [][2]string{{"Label", "Value"}}
	for _, key := range keys {
		rows = append(rows, [2]string{codeOrNA(key), codeOrNA(labels[key])})
	}
	return markdownTable(rows)
}

func layersTable(layers []string) string {
	rows := [][2]string{{"#", "Digest"}}
	for i, layer := range layers {
		rows = append(rows, [2]string{fmt.Sprintf("%d", i+1), codeOrNA(layer)})
	}
	return markdownTable(rows)
}

func detailsSection(summary, body string) string {
	return fmt.Sprintf("<details>\n<summary>%s</summary>\n\n%s\n</details>\n", markdownText(summary), strings.TrimRight(body, "\n"))
}

func humanizeDecimalBytes(b int64) string {
	if b <= 0 {
		return ""
	}
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	value := float64(b)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f EB", value/unit)
}

func imageTag(image string) string {
	if image == "" || strings.Contains(image, "@") {
		return ""
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon <= lastSlash {
		return ""
	}
	return image[lastColon+1:]
}

func digestFromRepoDigest(value string) string {
	if _, digest, ok := strings.Cut(value, "@"); ok {
		return digest
	}
	if strings.HasPrefix(value, "sha256:") {
		return value
	}
	return ""
}

func joinOrNA(values []string) string {
	if len(values) == 0 {
		return "n/a"
	}
	return strings.Join(values, ", ")
}

func codeOrNA(value string) string {
	if value == "" {
		value = "n/a"
	}
	return "`" + markdownCodeText(value) + "`"
}

func linkOrText(value string) string {
	if value == "" {
		return "`n/a`"
	}
	if u, err := url.Parse(value); err == nil && u.Scheme != "" && u.Host != "" {
		return value
	}
	return markdownText(value)
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "<br>")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func markdownText(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"[", "\\[",
		"]", "\\]",
		"<", "&lt;",
		">", "&gt;",
		"|", "\\|",
	)
	return replacer.Replace(value)
}

func markdownCodeText(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
