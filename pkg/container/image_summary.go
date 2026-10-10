package container

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/templates"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	labelOCIBase        = "org.opencontainers.image."
	labelOCIDescription = labelOCIBase + "description"
	labelOCILicenses    = labelOCIBase + "licenses"
	labelOCIRevision    = labelOCIBase + "revision"
	labelOCISource      = labelOCIBase + "source"
	labelOCITitle       = labelOCIBase + "title"
	labelOCIVersion     = labelOCIBase + "version"
	markdownBacktick    = "`"
	markdownLineBreak   = "\n"

	// The component type and command select the ci.templates.container.image override.
	imageSummaryComponentType = "container"
	imageSummaryCommand       = "image"
)

// ImageSummaryOptions controls the rich CI Markdown summary for a built or
// pushed container image.
type ImageSummaryOptions struct {
	Image  string
	Digest string
}

// ImageSummaryRow is one name/value row of a table in the image summary. Both fields are
// Markdown-ready: already escaped and, where applicable, wrapped in code spans.
type ImageSummaryRow struct {
	Name  string
	Value string
}

// ImageSummaryLayer is one layer row of the image summary.
type ImageSummaryLayer struct {
	// Index is the 1-based layer position.
	Index int
	// Digest is the Markdown-ready layer digest.
	Digest string
}

// ImageSummaryData is the data passed to the container image summary template
// (ci.templates.container.image). Every string field is Markdown-ready: escaped for
// its position and, for table cells, wrapped in a code span or link. A missing optional
// value renders as `n/a`.
type ImageSummaryData struct {
	// Image is the escaped image reference used as the heading.
	Image string
	// Badges are the size, license, architecture, and OS values, each safe inside a code span.
	Badges []string
	// Description is the escaped OCI description label, empty when unset.
	Description string
	// Tag, Digest, ID, Revision, and Source are table cells.
	Tag      string
	Digest   string
	ID       string
	Revision string
	Source   string
	// Entrypoint, Command, StopSignal, StorageDriver, and ExposedPorts are table cells.
	Entrypoint    string
	Command       string
	StopSignal    string
	StorageDriver string
	ExposedPorts  string
	// Env holds the environment variables sorted by name.
	Env []ImageSummaryRow
	// Labels holds the image labels sorted by name.
	Labels []ImageSummaryRow
	// Layers holds the layer digests in image order.
	Layers []ImageSummaryLayer
	// RawJSON is the unescaped inspect output, empty when unavailable.
	RawJSON string
}

// BuildImageSummaryData converts inspected image metadata into template data.
func BuildImageSummaryData(info *ImageInfo, opts ImageSummaryOptions) *ImageSummaryData {
	defer perf.Track(nil, "container.BuildImageSummaryData")()

	if info == nil {
		return nil
	}
	image := firstNonEmpty(opts.Image, firstString(info.RepoTags))
	labels := info.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	badges := summaryBadges(info, labels)
	for i, badge := range badges {
		badges[i] = markdownCodeText(badge)
	}
	data := &ImageSummaryData{
		Image:         markdownText(image),
		Badges:        badges,
		Description:   markdownText(labels[labelOCIDescription]),
		Tag:           markdownCell(codeOrNA(summaryTag(image, labels))),
		Digest:        markdownCell(codeOrNA(summaryDigest(info, opts.Digest))),
		ID:            markdownCell(codeOrNA(info.ID)),
		Revision:      markdownCell(codeOrNA(labels[labelOCIRevision])),
		Source:        markdownCell(linkOrText(labels[labelOCISource])),
		Entrypoint:    markdownCell(codeOrNA(strings.Join(info.Entrypoint, " "))),
		Command:       markdownCell(codeOrNA(strings.Join(info.Cmd, " "))),
		StopSignal:    markdownCell(codeOrNA(firstNonEmpty(info.StopSignal, "n/a"))),
		StorageDriver: markdownCell(codeOrNA(firstNonEmpty(info.StorageDriver, "n/a"))),
		ExposedPorts:  markdownCell(codeOrNA(joinOrNA(info.ExposedPorts))),
		Env:           envRows(info.Env),
		Labels:        labelRows(labels),
		RawJSON:       info.RawInspectJSON,
	}
	for i, layer := range info.LayerDigests {
		data.Layers = append(data.Layers, ImageSummaryLayer{Index: i + 1, Digest: markdownCell(codeOrNA(layer))})
	}
	return data
}

// RenderImageSummary renders the Markdown summary for a container image through the CI
// template loader. The embedded default is used unless ci.templates.container.image or a
// file under ci.templates.base_path overrides it. A nil atmosConfig always uses the default.
func RenderImageSummary(atmosConfig *schema.AtmosConfiguration, info *ImageInfo, opts ImageSummaryOptions) (string, error) {
	defer perf.Track(atmosConfig, "container.RenderImageSummary")()

	data := BuildImageSummaryData(info, opts)
	if data == nil {
		return "", nil
	}
	return templates.NewLoader(atmosConfig).LoadAndRender(imageSummaryComponentType, imageSummaryCommand, templates.ContainerDefaults(), data)
}

// RenderImageSummaryMarkdown renders a GitHub-flavored Markdown job summary for
// a container image using the embedded default template. The format intentionally
// mirrors the summary produced by Cloud Posse's Docker build action so native
// Atmos container builds have the same CI polish without an extra GitHub Action.
// Use RenderImageSummary to honor ci.templates overrides.
func RenderImageSummaryMarkdown(info *ImageInfo, opts ImageSummaryOptions) string {
	defer perf.Track(nil, "container.RenderImageSummaryMarkdown")()

	md, err := RenderImageSummary(nil, info, opts)
	if err != nil {
		// The embedded default is validated by tests, so this only guards against a broken build.
		log.Debug("Failed to render the default container image summary", "error", err)
		return ""
	}
	return md
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

func envRows(env []string) []ImageSummaryRow {
	rows := make([]ImageSummaryRow, 0, len(env))
	for _, item := range sortedStrings(env) {
		key, value, _ := strings.Cut(item, "=")
		rows = append(rows, ImageSummaryRow{Name: markdownCell(codeOrNA(key)), Value: markdownCell(codeOrNA(value))})
	}
	return rows
}

func labelRows(labels map[string]string) []ImageSummaryRow {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]ImageSummaryRow, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, ImageSummaryRow{Name: markdownCell(codeOrNA(key)), Value: markdownCell(codeOrNA(labels[key]))})
	}
	return rows
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
	return markdownBacktick + markdownCodeText(value) + markdownBacktick
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
	value = strings.ReplaceAll(value, markdownLineBreak, "<br>")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func markdownText(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		markdownBacktick, "\\`",
		"[", "\\[",
		"]", "\\]",
		"<", "&lt;",
		">", "&gt;",
		"|", "\\|",
	)
	return replacer.Replace(value)
}

func markdownCodeText(value string) string {
	return strings.ReplaceAll(value, markdownBacktick, "'")
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
