package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderImageSummaryMarkdown(t *testing.T) {
	info := &ImageInfo{
		ID:           "sha256:image-id",
		RepoTags:     []string{"registry.example.com/app:sha-abc"},
		RepoDigests:  []string{"registry.example.com/app@sha256:repo"},
		Size:         27093765,
		Architecture: "amd64",
		Os:           "linux",
		Labels: map[string]string{
			labelOCIDescription: "Deploy app",
			labelOCILicenses:    "Apache-2.0",
			labelOCIRevision:    "abc",
			labelOCISource:      "https://github.com/example/app",
			labelOCIVersion:     "sha-abc",
			"z":                 "last",
			"a":                 "first|pipe",
		},
		Env:           []string{"PATH=/bin", "APP_ENV=test"},
		Cmd:           []string{"./app"},
		Entrypoint:    []string{"/entrypoint.sh"},
		ExposedPorts:  []string{"8080/tcp"},
		StopSignal:    "SIGTERM",
		StorageDriver: "overlay2",
		LayerDigests:  []string{"sha256:l1", "sha256:l2"},
		RawInspectJSON: `[
  {
    "Id": "sha256:image-id"
  }
]`,
	}

	md := RenderImageSummaryMarkdown(info, ImageSummaryOptions{
		Image:  "registry.example.com/app:sha-abc",
		Digest: "sha256:pushed",
	})

	assert.Contains(t, md, "## 🐳 registry.example.com/app:sha-abc")
	assert.Contains(t, md, "`27.1 MB`")
	assert.Contains(t, md, "`Apache-2.0`")
	assert.Contains(t, md, "| Tag | `sha-abc` |")
	assert.Contains(t, md, "| Digest | `sha256:pushed` |")
	assert.Contains(t, md, "| Image ID | `sha256:image-id` |")
	assert.Contains(t, md, "| Source | https://github.com/example/app |")
	assert.Contains(t, md, "<summary>⚙️ Runtime</summary>")
	assert.Contains(t, md, "| Entrypoint | `/entrypoint.sh` |")
	assert.Contains(t, md, "<summary>🌱 Environment variables</summary>")
	assert.Contains(t, md, "| `APP_ENV` | `test` |")
	assert.Contains(t, md, "<summary>🔖 Labels</summary>")
	assert.Contains(t, md, "| `a` | `first\\|pipe` |")
	assert.Contains(t, md, "<summary>📦 Layers (2)</summary>")
	assert.Contains(t, md, "| 2 | `sha256:l2` |")
	assert.Contains(t, md, "<summary>📄 Raw JSON</summary>")
	assert.Contains(t, md, "```json")
	assert.True(t, strings.HasPrefix(md, "\n"), "summary appends as a separated chapter")
}

func TestRenderImageSummaryMarkdownFallsBackToRepoDigest(t *testing.T) {
	md := RenderImageSummaryMarkdown(&ImageInfo{
		RepoTags:    []string{"localhost:5000/app:latest"},
		RepoDigests: []string{"localhost:5000/app@sha256:repo"},
	}, ImageSummaryOptions{})

	assert.Contains(t, md, "| Tag | `latest` |")
	assert.Contains(t, md, "| Digest | `sha256:repo` |")
}
