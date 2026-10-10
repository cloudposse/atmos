package container

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ci"
	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// useMockReporter injects a mock ci.Reporter through newContainerReporter and returns it
// together with the slices that recordings from expectSummaryAndComment append to.
func useMockReporter(t *testing.T, ctrl *gomock.Controller) (*MockReporter, *[]string, *[]ci.CommentRequest) {
	t.Helper()
	reporter := NewMockReporter(ctrl)
	prev := newContainerReporter
	newContainerReporter = func(*schema.AtmosConfiguration) ci.Reporter { return reporter }
	t.Cleanup(func() { newContainerReporter = prev })
	return reporter, &[]string{}, &[]ci.CommentRequest{}
}

// expectSummaryAndComment expects one Summary call followed by one Comment call and records
// their arguments. The Comment call returns commentErr.
func expectSummaryAndComment(reporter *MockReporter, summaries *[]string, comments *[]ci.CommentRequest, commentErr error) {
	gomock.InOrder(
		reporter.EXPECT().Summary(gomock.Any()).DoAndReturn(func(md string) (ci.Receipt, error) {
			*summaries = append(*summaries, md)
			return ci.Receipt{Provider: "github"}, nil
		}),
		reporter.EXPECT().Comment(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req ci.CommentRequest) (ci.Receipt, error) {
			*comments = append(*comments, req)
			return ci.Receipt{Provider: "github"}, commentErr
		}),
	)
}

func enabledCIConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
}

func TestWriteImageSummaryReportsSummaryAndComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter, summaries, comments := useMockReporter(t, ctrl)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	info := &ctr.ImageInfo{RepoTags: []string{"img:1"}}
	opts := ctr.ImageSummaryOptions{Image: "img:1"}
	writeImageSummary(context.Background(), enabledCIConfig(), info, opts)

	want := ctr.RenderImageSummaryMarkdown(info, opts)
	require.NotEmpty(t, want)
	require.Len(t, *summaries, 1)
	assert.Equal(t, want, (*summaries)[0])
	require.Len(t, *comments, 1)
	assert.Equal(t, "container:image:img:1", (*comments)[0].Key)
	assert.Equal(t, want, (*comments)[0].Body)
}

func TestWriteImageSummaryCommentKeyFallsBackToRepoTag(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter, summaries, comments := useMockReporter(t, ctrl)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	writeImageSummary(context.Background(), enabledCIConfig(), &ctr.ImageInfo{RepoTags: []string{"tagged:2"}}, ctr.ImageSummaryOptions{})

	require.Len(t, *comments, 1)
	assert.Equal(t, "container:image:tagged:2", (*comments)[0].Key)
}

func TestWriteImageSummaryDoesNotCommentWithoutImageName(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter, _, _ := useMockReporter(t, ctrl)
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "github"}, nil)

	writeImageSummary(context.Background(), enabledCIConfig(), &ctr.ImageInfo{ID: "sha256:x"}, ctr.ImageSummaryOptions{})
}

func TestWriteImageSummaryDoesNotCommentWhenRenderedLocally(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter, _, _ := useMockReporter(t, ctrl)
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "generic", Local: true}, nil)

	writeImageSummary(context.Background(), enabledCIConfig(), &ctr.ImageInfo{RepoTags: []string{"img:1"}}, ctr.ImageSummaryOptions{Image: "img:1"})
}

func TestWriteImageSummaryIgnoresReporterErrors(t *testing.T) {
	t.Run("summary error skips the comment and does not propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		reporter, _, _ := useMockReporter(t, ctrl)
		reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{}, assert.AnError)

		assert.NotPanics(t, func() {
			writeImageSummary(context.Background(), enabledCIConfig(), &ctr.ImageInfo{RepoTags: []string{"img:1"}}, ctr.ImageSummaryOptions{Image: "img:1"})
		})
	})

	t.Run("comment error does not propagate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		reporter, summaries, comments := useMockReporter(t, ctrl)
		expectSummaryAndComment(reporter, summaries, comments, assert.AnError)

		assert.NotPanics(t, func() {
			writeImageSummary(context.Background(), enabledCIConfig(), &ctr.ImageInfo{RepoTags: []string{"img:1"}}, ctr.ImageSummaryOptions{Image: "img:1"})
		})
		assert.Len(t, *comments, 1)
	})
}

func TestWriteImageSummaryUsesTemplateOverride(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("custom {{.Image}}"), 0o644))
	cfg := enabledCIConfig()
	cfg.CI.Templates.BasePath = baseDir

	ctrl := gomock.NewController(t)
	reporter, summaries, comments := useMockReporter(t, ctrl)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	writeImageSummary(context.Background(), cfg, &ctr.ImageInfo{RepoTags: []string{"img:1"}}, ctr.ImageSummaryOptions{Image: "img:1"})

	require.Len(t, *summaries, 1)
	assert.Equal(t, "custom img:1", (*summaries)[0])
	assert.Equal(t, "custom img:1", (*comments)[0].Body)
}

func TestWriteImageSummarySkipsReportingWhenTemplateFails(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("{{.Missing"), 0o644))
	cfg := enabledCIConfig()
	cfg.CI.Templates.BasePath = baseDir

	ctrl := gomock.NewController(t)
	// The mock has no expectations, so any reporter call fails the test.
	useMockReporter(t, ctrl)

	writeImageSummary(context.Background(), cfg, &ctr.ImageInfo{RepoTags: []string{"img:1"}}, ctr.ImageSummaryOptions{Image: "img:1"})
}
