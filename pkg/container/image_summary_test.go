package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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
			"backtick":          "run `cmd`",
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
	assert.Contains(t, md, "| `backtick` | `run 'cmd'` |")
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

func TestRenderImageSummaryMarkdownEdgeFallbacks(t *testing.T) {
	assert.Empty(t, RenderImageSummaryMarkdown(nil, ImageSummaryOptions{}))
	assert.Empty(t, summaryBadges(&ImageInfo{}, nil))
	assert.Equal(t, "app@sha256:digest", summaryTag("app@sha256:digest", nil))
	assert.Equal(t, "n/a", summaryDigest(&ImageInfo{}, ""))
	assert.Equal(t, "sha256:inline", digestFromRepoDigest("sha256:inline"))
	assert.Empty(t, digestFromRepoDigest("not-a-digest"))
	assert.Equal(t, "999 B", humanizeDecimalBytes(999))
	assert.Empty(t, humanizeDecimalBytes(0))
	assert.Equal(t, "n/a", joinOrNA(nil))

	md := RenderImageSummaryMarkdown(&ImageInfo{
		ID:       "sha256:id",
		RepoTags: []string{"example.com/app"},
		Labels: map[string]string{
			labelOCISource: "not a url with *markdown*",
		},
		Env: []string{"EMPTY"},
	}, ImageSummaryOptions{})

	assert.Contains(t, md, "## 🐳 example.com/app")
	assert.Contains(t, md, "| Tag | `example.com/app` |")
	assert.Contains(t, md, "| Digest | `n/a` |")
	assert.Contains(t, md, "| Source | not a url with \\*markdown\\* |")
	assert.Contains(t, md, "| `EMPTY` | `n/a` |")
	assert.NotContains(t, md, "Layers (")
	assert.NotContains(t, md, "Raw JSON")
}

// goldenImageSummaryCases are rendered by the embedded default template and compared with
// the output captured from the pre-template renderer, which proves the template is byte-identical.
func goldenImageSummaryCases() map[string]struct {
	info *ImageInfo
	opts ImageSummaryOptions
} {
	type c = struct {
		info *ImageInfo
		opts ImageSummaryOptions
	}
	return map[string]c{
		"full": {&ImageInfo{
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
				"backtick":          "run `cmd`",
			},
			Env:            []string{"PATH=/bin", "APP_ENV=test"},
			Cmd:            []string{"./app"},
			Entrypoint:     []string{"/entrypoint.sh"},
			ExposedPorts:   []string{"8080/tcp"},
			StopSignal:     "SIGTERM",
			StorageDriver:  "overlay2",
			LayerDigests:   []string{"sha256:l1", "sha256:l2"},
			RawInspectJSON: "[\n  {\n    \"Id\": \"sha256:image-id\"\n  }\n]",
		}, ImageSummaryOptions{Image: "registry.example.com/app:sha-abc", Digest: "sha256:pushed"}},
		"repo_digest": {&ImageInfo{
			RepoTags:    []string{"localhost:5000/app:latest"},
			RepoDigests: []string{"localhost:5000/app@sha256:repo"},
		}, ImageSummaryOptions{}},
		"minimal": {&ImageInfo{
			ID:       "sha256:id",
			RepoTags: []string{"example.com/app"},
			Labels:   map[string]string{labelOCISource: "not a url with *markdown*"},
			Env:      []string{"EMPTY"},
		}, ImageSummaryOptions{}},
		"empty_info": {&ImageInfo{}, ImageSummaryOptions{Image: "a_b*c<d>"}},
		"multiline_cells": {&ImageInfo{
			Entrypoint: []string{"a|b", "c"},
			Labels:     map[string]string{"k": "line1\nline2\r\nline3"},
			Env:        []string{"A=b=c"},
			Size:       999,
			Os:         "linux",
		}, ImageSummaryOptions{Image: "x:1"}},
	}
}

func TestRenderImageSummaryMarkdownMatchesGolden(t *testing.T) {
	cases := goldenImageSummaryCases()
	require.NotEmpty(t, cases)
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "image_summary", name+".golden.md"))
			require.NoError(t, err)
			require.NotEmpty(t, wantBytes)
			// Golden files are normalized to end with a newline by pre-commit; the rendered
			// markdown is compared without its trailing newline so that normalization is immaterial.
			want := strings.TrimRight(string(wantBytes), "\n")

			assert.Equal(t, want, strings.TrimRight(RenderImageSummaryMarkdown(tc.info, tc.opts), "\n"))

			got, err := RenderImageSummary(&schema.AtmosConfiguration{}, tc.info, tc.opts)
			require.NoError(t, err)
			assert.Equal(t, want, strings.TrimRight(got, "\n"), "a config without overrides uses the embedded default")
		})
	}
}

func TestRenderImageSummaryNilInfo(t *testing.T) {
	got, err := RenderImageSummary(nil, nil, ImageSummaryOptions{})
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Nil(t, BuildImageSummaryData(nil, ImageSummaryOptions{}))
}

func TestBuildImageSummaryData(t *testing.T) {
	data := BuildImageSummaryData(&ImageInfo{
		RepoTags:     []string{"app:1"},
		Size:         1500,
		Architecture: "arm64",
		Env:          []string{"B=2", "A=1"},
		Labels:       map[string]string{"z": "last", "a": "first"},
		LayerDigests: []string{"sha256:l1", "sha256:l2"},
	}, ImageSummaryOptions{Digest: "sha256:pushed"})
	require.NotNil(t, data)

	assert.Equal(t, "app:1", data.Image)
	assert.Equal(t, []string{"1.5 KB", "arm64"}, data.Badges)
	assert.Equal(t, "`1`", data.Tag)
	assert.Equal(t, "`sha256:pushed`", data.Digest)
	assert.Equal(t, "`n/a`", data.Revision)
	assert.Equal(t, []ImageSummaryRow{{Name: "`A`", Value: "`1`"}, {Name: "`B`", Value: "`2`"}}, data.Env)
	assert.Equal(t, []ImageSummaryRow{{Name: "`a`", Value: "`first`"}, {Name: "`z`", Value: "`last`"}}, data.Labels)
	require.Len(t, data.Layers, 2)
	assert.Equal(t, ImageSummaryLayer{Index: 1, Digest: "`sha256:l1`"}, data.Layers[0])
	assert.Equal(t, ImageSummaryLayer{Index: 2, Digest: "`sha256:l2`"}, data.Layers[1])
}

func TestRenderImageSummaryTemplateOverride(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(baseDir, "container", "image.md"),
		[]byte("custom {{.Image}} {{.Tag}} layers={{len .Layers}}"),
		0o644,
	))
	info := &ImageInfo{RepoTags: []string{"app:1"}, LayerDigests: []string{"sha256:l1"}}

	t.Run("base_path convention overrides the default", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Templates: schema.CITemplatesConfig{BasePath: baseDir}}}
		got, err := RenderImageSummary(cfg, info, ImageSummaryOptions{})
		require.NoError(t, err)
		assert.Equal(t, "custom app:1 `1` layers=1", got)
	})

	t.Run("ci.templates.container.image overrides the default", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(baseDir, "named.md"), []byte("named {{.Image}}"), 0o644))
		cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Templates: schema.CITemplatesConfig{
			BasePath:  baseDir,
			Container: map[string]string{"image": "named.md"},
		}}}
		got, err := RenderImageSummary(cfg, info, ImageSummaryOptions{})
		require.NoError(t, err)
		assert.Equal(t, "named app:1", got)
	})

	t.Run("no override falls back to the default", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Templates: schema.CITemplatesConfig{BasePath: t.TempDir()}}}
		got, err := RenderImageSummary(cfg, info, ImageSummaryOptions{})
		require.NoError(t, err)
		assert.Contains(t, got, "## 🐳 app:1")
	})

	t.Run("a broken override template returns the sentinel error", func(t *testing.T) {
		brokenDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(brokenDir, "container"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(brokenDir, "container", "image.md"), []byte("{{.Missing"), 0o644))
		cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Templates: schema.CITemplatesConfig{BasePath: brokenDir}}}
		_, err := RenderImageSummary(cfg, info, ImageSummaryOptions{})
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
}
