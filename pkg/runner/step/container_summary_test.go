package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component/container/imagesummary/imagesummarytest"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true}
	_ = schema.CISummaryConfig{Enabled: boolPtrForSummary(true)}
	_ = schema.CITemplatesConfig{BasePath: "templates"}
)

func boolPtrForSummary(v bool) *bool { return &v }

func TestWriteContainerImageSummaryCommentsOffWritesJobSummaryOnly(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)

	writeContainerImageSummary(context.Background(), imagesummarytest.Config(false), &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})

	assert.Contains(t, h.Summary(t), "## 🐳 img:1")
	assert.Empty(t, h.Server.Requests(), "comments are off, so GitHub is never called")
}

func TestWriteContainerImageSummaryUpdatesOneCommentPerImage(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	cfg := imagesummarytest.Config(true)
	ctx := context.Background()

	writeContainerImageSummary(ctx, cfg, &container.ImageInfo{RepoTags: []string{"img:sha-1"}}, container.ImageSummaryOptions{Image: "img:sha-1"})
	writeContainerImageSummary(ctx, cfg, &container.ImageInfo{RepoTags: []string{"img:sha-2"}}, container.ImageSummaryOptions{Image: "img:sha-2"})

	comments := h.Comments()
	require.Len(t, comments, 1, "a new tag updates the image comment instead of adding one")
	assert.Contains(t, comments[0].Body, "<!-- atmos:ci:container:image:img -->")
	assert.Contains(t, comments[0].Body, "## 🐳 img:sha-2")
	assert.NotContains(t, comments[0].Body, "img:sha-1")
}

func TestWriteContainerImageSummarySkipsWhenCIUnavailable(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)

	ctx := context.Background()
	writeContainerImageSummary(ctx, nil, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(ctx, &schema.AtmosConfiguration{}, &container.ImageInfo{RepoTags: []string{"app:local"}}, container.ImageSummaryOptions{Image: "app:local"})
	writeContainerImageSummary(ctx, imagesummarytest.Config(true), nil, container.ImageSummaryOptions{Image: "app:local"})

	assert.Empty(t, h.Summary(t))
	assert.Empty(t, h.Server.Requests())
}

func TestWriteContainerImageSummaryUsesTemplateOverride(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("custom {{.Image}}"), 0o644))
	cfg := imagesummarytest.Config(true)
	cfg.CI.Templates.BasePath = baseDir

	writeContainerImageSummary(context.Background(), cfg, &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})

	assert.Contains(t, h.Summary(t), "custom img:1")
	comments := h.Comments()
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0].Body, "custom img:1")
}

func TestWriteContainerImageSummarySkipsReportingWhenTemplateFails(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("{{.Missing"), 0o644))
	cfg := imagesummarytest.Config(true)
	cfg.CI.Templates.BasePath = baseDir

	writeContainerImageSummary(context.Background(), cfg, &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})

	assert.Empty(t, h.Summary(t))
	assert.Empty(t, h.Server.Requests())
}

func TestContainerHandlerExecuteBuildWritesCISummaryWhenEnabled(t *testing.T) {
	installStepFakeDocker(t)
	h := imagesummarytest.NewGitHub(t)
	handler := &ContainerHandler{}
	vars := NewVariables()
	vars.SetAtmosConfig(imagesummarytest.Config(true))

	_, err := handler.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	assert.Contains(t, h.Summary(t), "## 🐳 app:local")
	assert.Contains(t, h.Summary(t), "| Digest | `sha256:built` |")
	comments := h.Comments()
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0].Body, "<!-- atmos:ci:container:image:app -->")
	assert.Contains(t, comments[0].Body, "| Digest | `sha256:built` |")
}

func TestContainerHandlerExecuteBuildSkipsCISummaryWhenDisabled(t *testing.T) {
	installStepFakeDocker(t)
	h := imagesummarytest.NewGitHub(t)
	handler := &ContainerHandler{}
	cfg := imagesummarytest.Config(true)
	cfg.CI.Summary.Enabled = boolPtrForSummary(false)
	vars := NewVariables()
	vars.SetAtmosConfig(cfg)

	_, err := handler.executeBuild(context.Background(), &schema.WorkflowStep{
		Name: "build",
		Build: &schema.ContainerBuildStep{
			Provider: string(container.TypeDocker),
			Context:  ".",
			Tags:     []string{"app:local"},
		},
	}, vars)

	require.NoError(t, err)
	assert.Empty(t, h.Summary(t))
	assert.Empty(t, h.Server.Requests())
}

func TestWritePushedImageSummariesSkipsInvalidAndInspectFailures(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	runtime := &pushRuntime{
		imageInfos: map[string]*container.ImageInfo{
			"registry.example.com/app:ok": {
				ID:       "sha256:img",
				RepoTags: []string{"registry.example.com/app:ok"},
			},
		},
		inspectErrs: map[string]error{
			"registry.example.com/app:missing": assert.AnError,
		},
	}

	writePushedImageSummaries(context.Background(), runtime, imagesummarytest.Config(true), []*container.PushResult{
		nil,
		{},
		{Image: "registry.example.com/app:missing", Digest: "sha256:missing"},
		{Image: "registry.example.com/app:ok", Digest: "sha256:ok"},
	})

	assert.Contains(t, h.Summary(t), "## 🐳 registry.example.com/app:ok")
	assert.Contains(t, h.Summary(t), "| Digest | `sha256:ok` |")
	comments := h.Comments()
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0].Body, "<!-- atmos:ci:container:image:registry.example.com/app -->")
}

func TestWritePushedImageSummariesListsEveryRefInOneCommentPerImage(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	refs := []string{"registry.example.com/app:1.0", "registry.example.com/app:latest", "other.example.com/app:1.0"}
	runtime := &pushRuntime{imageInfos: map[string]*container.ImageInfo{}}
	var pushes []*container.PushResult
	for _, ref := range refs {
		runtime.imageInfos[ref] = &container.ImageInfo{RepoTags: []string{ref}}
		pushes = append(pushes, &container.PushResult{Image: ref, Digest: "sha256:ok"})
	}

	writePushedImageSummaries(context.Background(), runtime, imagesummarytest.Config(true), pushes)

	comments := h.Comments()
	require.Len(t, comments, 2, "one comment per image repository, not per pushed ref")
	var multi string
	for _, c := range comments {
		if strings.Contains(c.Body, "<!-- atmos:ci:container:image:registry.example.com/app -->") {
			multi = c.Body
		}
	}
	require.NotEmpty(t, multi)
	assert.Contains(t, multi, "## 🐳 registry.example.com/app:1.0")
	assert.Contains(t, multi, "## 🐳 registry.example.com/app:latest")
	assert.NotContains(t, multi, "other.example.com")
}

func TestWritePushedImageSummariesCommentsOffDoesNotCallGitHub(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	runtime := &pushRuntime{imageInfos: map[string]*container.ImageInfo{"r/app:1": {RepoTags: []string{"r/app:1"}}}}

	writePushedImageSummaries(context.Background(), runtime, imagesummarytest.Config(false), []*container.PushResult{{Image: "r/app:1"}})

	assert.Contains(t, h.Summary(t), "## 🐳 r/app:1")
	assert.Empty(t, h.Server.Requests())
}
