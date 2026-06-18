package step

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderImageInspect(t *testing.T) {
	info := &container.ImageInfo{
		ID:           "fef51e975bdcb872c3ff2d3b4e4d0ff0c0f522096a0f52f54f9dc4d306de0e32",
		RepoTags:     []string{"atmos-container-step:local"},
		RepoDigests:  []string{"localhost/atmos-container-step@sha256:b7e390e5767ed0aabbccddeeff"},
		Size:         8_912_896, // 8.5 MiB
		Created:      "2026-06-18T23:06:08Z",
		Architecture: "arm64",
		Os:           "linux",
		Layers:       2,
		Labels:       map[string]string{"org.opencontainers.image.title": "Atmos Example", "extra": "x"},
	}

	out := renderImageInspect("atmos-container-step:local", info)

	assert.Contains(t, out, "## Image `atmos-container-step:local`")
	assert.Contains(t, out, "| ID | fef51e975bdc |")
	assert.Contains(t, out, "| Digest | sha256:b7e390e5767e |")
	assert.Contains(t, out, "| Tags | atmos-container-step:local |")
	assert.Contains(t, out, "| Created | 2026-06-18 23:06:08 UTC |")
	assert.Contains(t, out, "| Size | 8.5 MiB |")
	assert.Contains(t, out, "| Platform | linux/arm64 |")
	assert.Contains(t, out, "| Layers | 2 |")
	assert.Contains(t, out, "| Labels | 2 |")
	assert.Contains(t, out, "| Title | Atmos Example |")
	// Curated, not a JSON/map dump.
	assert.NotContains(t, out, "map[")
}

func TestRenderImageInspectOmitsEmptyFields(t *testing.T) {
	// A minimal image (e.g. when inspect returns sparse data) must not render
	// empty rows like "| Size |  |".
	out := renderImageInspect("scratch:latest", &container.ImageInfo{ID: "abc123abc123def"})
	assert.NotContains(t, out, "| Size |")
	assert.NotContains(t, out, "| Platform |")
	assert.NotContains(t, out, "| Created |")
	assert.Contains(t, out, "| ID | abc123abc123 |")
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
	// Flat `image:`/`runtime:` shorthand is adopted.
	got := effectiveInspectStep(&schema.WorkflowStep{Image: "alpine:latest", Runtime: "podman"})
	assert.Equal(t, "alpine:latest", got.Image)
	assert.Equal(t, "podman", got.Runtime)

	// An explicit `inspect:` block wins over the flat shorthand.
	got = effectiveInspectStep(&schema.WorkflowStep{
		Image:   "flat",
		Inspect: &schema.ContainerInspectStep{Image: "explicit"},
	})
	assert.Equal(t, "explicit", got.Image)
}
