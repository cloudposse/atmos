package container

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/component/container/imagesummary/imagesummarytest"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true}
	_ = schema.CICommentsConfig{Enabled: boolPtr(true)}
	_ = schema.CISummaryConfig{Enabled: boolPtr(true)}
)

func boolPtr(v bool) *bool { return &v }

func TestExecuteBuild_WritesJobSummaryWithoutCommentWhenCommentsOff(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	section := map[string]any{
		"build": map[string]any{"context": "app", "dockerfile": "Dockerfile", "tags": []any{"img:1"}},
	}
	withStubsConfig(t, &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, section, nil, rt)
	gomock.InOrder(
		rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil),
		rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(&ctr.ImageInfo{
			ID:          "sha256:img",
			RepoTags:    []string{"img:1"},
			RepoDigests: []string{"img@sha256:digest"},
		}, nil),
	)

	require.NoError(t, ExecuteBuild(context.Background(), infoFor("api")))

	assert.Contains(t, h.Summary(t), "## 🐳 img:1")
	assert.Contains(t, h.Summary(t), "| Digest | `sha256:digest` |")
	assert.Empty(t, h.Server.Requests(), "comments are off by default, so GitHub is never called")
}

func TestExecuteBuild_PostsOneCommentPerImageWhenCommentsOn(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	section := map[string]any{
		"build": map[string]any{"context": "app", "dockerfile": "Dockerfile", "tags": []any{"img:sha-1"}},
	}
	withStubsConfig(t, imagesummarytest.Config(true), section, nil, rt)
	rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil)
	rt.EXPECT().ImageInspect(gomock.Any(), "img:sha-1").Return(&ctr.ImageInfo{RepoTags: []string{"img:sha-1"}}, nil)

	require.NoError(t, ExecuteBuild(context.Background(), infoFor("api")))

	comments := h.Comments()
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0].Body, "<!-- atmos:ci:container:image:img -->")
	assert.Contains(t, comments[0].Body, "## 🐳 img:sha-1")
}

func TestExecuteBuild_SkipsCISummaryWhenDisabled(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	cfg := imagesummarytest.Config(true)
	cfg.CI.Summary.Enabled = boolPtr(false)
	section := map[string]any{
		"build": map[string]any{"context": "app", "dockerfile": "Dockerfile", "tags": []any{"img:1"}},
	}
	withStubsConfig(t, cfg, section, nil, rt)
	// The runtime mock has no ImageInspect expectation: a disabled summary must not even inspect the image.
	rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil)

	require.NoError(t, ExecuteBuild(context.Background(), infoFor("api")))

	assert.Empty(t, h.Summary(t))
	assert.Empty(t, h.Server.Requests())
}

func TestExecutePush_ListsEveryPushedRefInTheImageComment(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	withStubsConfig(t, imagesummarytest.Config(true),
		buildSection("app:v1", "reg1.example.com/app:v1", "reg1.example.com/app:latest", "reg2.example.com/app:v1"), nil, rt)
	for _, ref := range []string{"reg1.example.com/app:v1", "reg1.example.com/app:latest", "reg2.example.com/app:v1"} {
		gomock.InOrder(
			rt.EXPECT().Push(gomock.Any(), ref).Return(&ctr.PushResult{Image: ref, Digest: "sha256:111"}, nil),
			rt.EXPECT().ImageInspect(gomock.Any(), ref).Return(&ctr.ImageInfo{RepoTags: []string{ref}}, nil),
		)
	}

	require.NoError(t, ExecutePush(context.Background(), infoFor("app")))

	for _, ref := range []string{"reg1.example.com/app:v1", "reg1.example.com/app:latest", "reg2.example.com/app:v1"} {
		assert.Contains(t, h.Summary(t), "## 🐳 "+ref, "the job summary lists every pushed ref")
	}
	comments := h.Comments()
	require.Len(t, comments, 2, "one comment per image repository, not per pushed ref")
	var reg1, reg2 string
	for _, c := range comments {
		switch {
		case strings.Contains(c.Body, "<!-- atmos:ci:container:image:reg1.example.com/app -->"):
			reg1 = c.Body
		case strings.Contains(c.Body, "<!-- atmos:ci:container:image:reg2.example.com/app -->"):
			reg2 = c.Body
		}
	}
	require.NotEmpty(t, reg1)
	assert.Contains(t, reg1, "## 🐳 reg1.example.com/app:v1")
	assert.Contains(t, reg1, "## 🐳 reg1.example.com/app:latest")
	require.NotEmpty(t, reg2)
	assert.Contains(t, reg2, "## 🐳 reg2.example.com/app:v1")
}

func TestExecutePush_PostsTheCommentOfPushedRefsWhenALaterPushFails(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	withStubsConfig(t, imagesummarytest.Config(true), buildSection("app:v1", "reg1.example.com/app:v1", "reg2.example.com/app:v1"), nil, rt)
	gomock.InOrder(
		rt.EXPECT().Push(gomock.Any(), "reg1.example.com/app:v1").Return(&ctr.PushResult{Image: "reg1.example.com/app:v1"}, nil),
		rt.EXPECT().ImageInspect(gomock.Any(), "reg1.example.com/app:v1").Return(&ctr.ImageInfo{RepoTags: []string{"reg1.example.com/app:v1"}}, nil),
		rt.EXPECT().Push(gomock.Any(), "reg2.example.com/app:v1").Return(nil, assert.AnError),
	)

	require.Error(t, ExecutePush(context.Background(), infoFor("app")))

	comments := h.Comments()
	require.Len(t, comments, 1)
	assert.Contains(t, comments[0].Body, "## 🐳 reg1.example.com/app:v1")
}

func TestInspectFailureSkipsReporting(t *testing.T) {
	h := imagesummarytest.NewGitHub(t)
	ctrl := gomock.NewController(t)
	rt := NewMockRuntime(ctrl)
	section := map[string]any{
		"build": map[string]any{"context": "app", "dockerfile": "Dockerfile", "tags": []any{"img:1"}},
	}
	withStubsConfig(t, imagesummarytest.Config(true), section, nil, rt)
	rt.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil)
	rt.EXPECT().ImageInspect(gomock.Any(), "img:1").Return(nil, assert.AnError)

	require.NoError(t, ExecuteBuild(context.Background(), infoFor("api")))

	assert.Empty(t, h.Summary(t))
	assert.Empty(t, h.Server.Requests())
}
