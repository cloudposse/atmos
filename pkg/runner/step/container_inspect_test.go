package step

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ansiRE matches ANSI escape sequences so tests can strip them and assert on
// plain content without depending on the active terminal color profile.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderImageInspect(t *testing.T) {
	info := &container.ImageInfo{
		ID:           "fef51e975bdcb872c3ff2d3b4e4d0ff0c0f522096a0f52f54f9dc4d306de0e32",
		RepoTags:     []string{"atmos-container-step:local"},
		RepoDigests:  []string{"localhost/atmos-container-step@sha256:b7e390e5767ed0aabbccddeeff"},
		Size:         8_912_896, // 8.5 MiB.
		Created:      "2026-06-18T23:06:08Z",
		Architecture: "arm64",
		Os:           "linux",
		Layers:       2,
		Labels:       map[string]string{"org.opencontainers.image.title": "Atmos Example", "extra": "x"},
	}

	// renderImageInspect renders only the borderless key/value body; the title is a
	// separate Markdown heading rendered by the caller.
	raw := renderImageInspect(info)
	// Strip ANSI codes so assertions are independent of the active color profile.
	out := ansiRE.ReplaceAllString(raw, "")

	// No Markdown table pipes and no heading inside the body.
	assert.NotContains(t, out, "| ID |")
	assert.NotContains(t, out, "## Image")

	// Each data row: key present + value present (borderless, two-column).
	assert.Contains(t, out, "fef51e975bdc")
	assert.Contains(t, out, "sha256:b7e390e5767e")
	assert.Contains(t, out, "atmos-container-step:local")
	assert.Contains(t, out, "2026-06-18 23:06:08 UTC")
	assert.Contains(t, out, "8.5 MiB")
	assert.Contains(t, out, "linux/arm64")
	assert.Contains(t, out, "Layers")
	assert.Contains(t, out, "Labels")
	assert.Contains(t, out, "Title")
	assert.Contains(t, out, "Atmos Example")
	// Curated output — no raw Go map dump.
	assert.NotContains(t, out, "map[")
}

func TestRenderImageInspectOmitsEmptyFields(t *testing.T) {
	// A minimal image (e.g. when inspect returns sparse data) must not render
	// empty rows for Size, Platform, or Created.
	raw := renderImageInspect(&container.ImageInfo{ID: "abc123abc123def"})
	out := ansiRE.ReplaceAllString(raw, "")

	assert.NotContains(t, out, "Size")
	assert.NotContains(t, out, "Platform")
	assert.NotContains(t, out, "Created")
	// The ID row must appear with the truncated digest.
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "abc123abc123")
}

func TestShortDigest(t *testing.T) {
	assert.Equal(t, "sha256:b7e390e5767e", shortDigest("repo@sha256:b7e390e5767ed0aabbcc"))
	assert.Equal(t, "fef51e975bdc", shortDigest("fef51e975bdcb872c3ff2d3b4e4d0ff0"))
	assert.Equal(t, "", shortDigest(""))
	assert.Equal(t, "alpine:latest", shortDigest("alpine:latest"))
}

func TestHumanizeBytes(t *testing.T) {
	assert.Equal(t, "", humanizeBytes(0))
	assert.Equal(t, "512 B", humanizeBytes(512))
	assert.Equal(t, "1.0 KiB", humanizeBytes(1024))
	assert.Equal(t, "8.5 MiB", humanizeBytes(8_912_896))
}

func TestPlatformString(t *testing.T) {
	assert.Equal(t, "linux/arm64", platformString("linux", "arm64"))
	assert.Equal(t, "linux", platformString("linux", ""))
	assert.Equal(t, "arm64", platformString("", "arm64"))
	assert.Equal(t, "", platformString("", ""))
}

func TestFormatInspectTime(t *testing.T) {
	assert.Equal(t, "2026-06-18 23:06:08 UTC", formatInspectTime("2026-06-18T23:06:08Z"))
	assert.Equal(t, "raw-unparseable", formatInspectTime("raw-unparseable"))
	assert.Empty(t, formatInspectTime(""))
}

func TestEffectiveInspectStep(t *testing.T) {
	// Image comes from the `inspect:` block; provider falls through from the step level.
	got := effectiveInspectStep(&schema.WorkflowStep{Inspect: &schema.ContainerInspectStep{Image: "alpine:latest"}, Provider: "podman"})
	assert.Equal(t, "alpine:latest", got.Image)
	assert.Equal(t, "podman", got.Provider)

	// The `inspect:` block's image is used.
	got = effectiveInspectStep(&schema.WorkflowStep{
		Inspect: &schema.ContainerInspectStep{Image: "explicit"},
	})
	assert.Equal(t, "explicit", got.Image)
}
