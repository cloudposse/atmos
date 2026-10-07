package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/schema"
)

// useMockReporter injects a mock ci.Reporter through newContainerReporter and returns it
// together with the slices that recordings from expectSummaryAndComment append to.
func useMockReporter(t *testing.T) (*MockReporter, *[]string, *[]ci.CommentRequest) {
	t.Helper()
	reporter := NewMockReporter(gomock.NewController(t))
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

func TestWriteContainerImageSummaryReportsSummaryAndComment(t *testing.T) {
	reporter, summaries, comments := useMockReporter(t)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	info := &container.ImageInfo{RepoTags: []string{"img:1"}}
	opts := container.ImageSummaryOptions{Image: "img:1"}
	writeContainerImageSummary(context.Background(), enabledCIConfig(), info, opts)

	want := container.RenderImageSummaryMarkdown(info, opts)
	require.NotEmpty(t, want)
	require.Len(t, *summaries, 1)
	assert.Equal(t, want, (*summaries)[0])
	require.Len(t, *comments, 1)
	assert.Equal(t, "container:image:img:1", (*comments)[0].Key)
	assert.Equal(t, want, (*comments)[0].Body)
}

func TestWriteContainerImageSummaryCommentKeyFallsBackToRepoTag(t *testing.T) {
	reporter, summaries, comments := useMockReporter(t)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	writeContainerImageSummary(context.Background(), enabledCIConfig(), &container.ImageInfo{RepoTags: []string{"tagged:2"}}, container.ImageSummaryOptions{})

	require.Len(t, *comments, 1)
	assert.Equal(t, "container:image:tagged:2", (*comments)[0].Key)
}

func TestWriteContainerImageSummaryDoesNotCommentWithoutImageName(t *testing.T) {
	reporter, _, _ := useMockReporter(t)
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "github"}, nil)

	writeContainerImageSummary(context.Background(), enabledCIConfig(), &container.ImageInfo{ID: "sha256:x"}, container.ImageSummaryOptions{})
}

func TestWriteContainerImageSummaryDoesNotCommentWhenRenderedLocally(t *testing.T) {
	reporter, _, _ := useMockReporter(t)
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "generic", Local: true}, nil)

	writeContainerImageSummary(context.Background(), enabledCIConfig(), &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})
}

func TestWriteContainerImageSummaryIgnoresReporterErrors(t *testing.T) {
	t.Run("summary error skips the comment and does not propagate", func(t *testing.T) {
		reporter, _, _ := useMockReporter(t)
		reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{}, assert.AnError)

		assert.NotPanics(t, func() {
			writeContainerImageSummary(context.Background(), enabledCIConfig(), &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})
		})
	})

	t.Run("comment error does not propagate", func(t *testing.T) {
		reporter, summaries, comments := useMockReporter(t)
		expectSummaryAndComment(reporter, summaries, comments, assert.AnError)

		assert.NotPanics(t, func() {
			writeContainerImageSummary(context.Background(), enabledCIConfig(), &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})
		})
		assert.Len(t, *comments, 1)
	})
}

func TestWriteContainerImageSummaryUsesTemplateOverride(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("custom {{.Image}}"), 0o644))
	cfg := enabledCIConfig()
	cfg.CI.Templates.BasePath = baseDir

	reporter, summaries, comments := useMockReporter(t)
	expectSummaryAndComment(reporter, summaries, comments, nil)

	writeContainerImageSummary(context.Background(), cfg, &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})

	require.Len(t, *summaries, 1)
	assert.Equal(t, "custom img:1", (*summaries)[0])
	assert.Equal(t, "custom img:1", (*comments)[0].Body)
}

func TestWriteContainerImageSummarySkipsReportingWhenTemplateFails(t *testing.T) {
	baseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(baseDir, "container"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "container", "image.md"), []byte("{{.Missing"), 0o644))
	cfg := enabledCIConfig()
	cfg.CI.Templates.BasePath = baseDir

	// The mock has no expectations, so any reporter call fails the test.
	useMockReporter(t)

	writeContainerImageSummary(context.Background(), cfg, &container.ImageInfo{RepoTags: []string{"img:1"}}, container.ImageSummaryOptions{Image: "img:1"})
}
