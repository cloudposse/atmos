package imagesummary

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github/ghtest"
	"github.com/cloudposse/atmos/pkg/component/container/imagesummary/imagesummarytest"
	ctr "github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests set.
var (
	_ = schema.CIConfig{Enabled: true}
	_ = schema.CICommentsConfig{Enabled: boolPtr(true)}
	_ = schema.CISummaryConfig{Enabled: boolPtr(true)}
)

func boolPtr(v bool) *bool { return &v }

// harness is the GitHub Actions harness plus the buffer that receives anything the reporter renders
// locally, which is the log preview.
type harness struct {
	*imagesummarytest.GitHub
	server *ghtest.Server
	local  *bytes.Buffer
}

func newHarness(t *testing.T, serverOpts ...ghtest.Option) *harness {
	t.Helper()

	g := imagesummarytest.NewGitHub(t, serverOpts...)
	local := &bytes.Buffer{}
	prev := newReporter
	newReporter = func(cfg *schema.AtmosConfiguration) ci.Reporter { return ci.NewReporter(cfg).WithOutput(local) }
	t.Cleanup(func() { newReporter = prev })
	return &harness{GitHub: g, server: g.Server, local: local}
}

func (h *harness) summary(t *testing.T) string {
	t.Helper()
	return h.Summary(t)
}

func ciConfig(commentsEnabled bool) *schema.AtmosConfiguration {
	return imagesummarytest.Config(commentsEnabled)
}

// inspectStub answers ImageInspect from a map and fails for every other image. It embeds the
// interface so only the method under test is implemented.
type inspectStub struct {
	ctr.Runtime
	images map[string]*ctr.ImageInfo
}

func (s inspectStub) ImageInspect(_ context.Context, image string) (*ctr.ImageInfo, error) {
	if info, ok := s.images[image]; ok {
		return info, nil
	}
	return nil, assert.AnError
}

func info(tag string) *ctr.ImageInfo {
	return &ctr.ImageInfo{RepoTags: []string{tag}}
}

func TestWriteCommentsOffDoesNotPreviewOrComment(t *testing.T) {
	h := newHarness(t)
	cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}

	Write(context.Background(), cfg, info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})

	assert.Contains(t, h.summary(t), "## 🐳 img:1", "the job summary still receives the image")
	assert.Empty(t, h.local.String(), "nothing is previewed in the log when comments are off")
	assert.Empty(t, h.server.Comments())
	assert.Empty(t, h.server.Requests(), "no API request is made when comments are off")
}

func TestWriteCommentsExplicitlyOffDoesNotPreviewOrComment(t *testing.T) {
	h := newHarness(t)

	Write(context.Background(), ciConfig(false), info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})

	assert.Contains(t, h.summary(t), "## 🐳 img:1")
	assert.Empty(t, h.local.String())
	assert.Empty(t, h.server.Comments())
}

func TestWriteCommentsOnPostsOneCommentPerImage(t *testing.T) {
	h := newHarness(t)
	cfg := ciConfig(true)

	Write(context.Background(), cfg, info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})
	Write(context.Background(), cfg, info("img:2"), ctr.ImageSummaryOptions{Image: "img:2"})
	Write(context.Background(), cfg, info("img:3"), ctr.ImageSummaryOptions{Image: "img:3", Digest: "sha256:abc"})

	writes := h.server.Comments()
	require.Len(t, writes, 3, "one create followed by two in-place edits")
	assert.False(t, writes[0].Edited)
	assert.True(t, writes[1].Edited)
	assert.True(t, writes[2].Edited)
	for _, w := range writes {
		assert.Contains(t, w.Body, "<!-- atmos:ci:container:image:img -->", "the key is the repository without the tag")
	}
	current := h.server.CommentsFor("owner", "repo", 42)
	require.Len(t, current, 1, "SHA or version tags must not create a comment per push")
	assert.Contains(t, current[0].Body, "## 🐳 img:3")
	assert.NotContains(t, current[0].Body, "## 🐳 img:1")
	assert.Empty(t, h.local.String())
	assert.Contains(t, h.summary(t), "## 🐳 img:1", "the job summary keeps every push")
	assert.Contains(t, h.summary(t), "## 🐳 img:3")
}

func TestSessionListsEveryPushedRefInOneCommentPerImage(t *testing.T) {
	h := newHarness(t)
	s := NewSession(ciConfig(true))

	s.Add(info("ghcr.io/org/app:1.2.3"), ctr.ImageSummaryOptions{Image: "ghcr.io/org/app:1.2.3"})
	s.Add(info("ghcr.io/org/app:latest"), ctr.ImageSummaryOptions{Image: "ghcr.io/org/app:latest"})
	s.Add(info("ghcr.io/org/worker:1.2.3"), ctr.ImageSummaryOptions{Image: "ghcr.io/org/worker:1.2.3"})
	assert.Empty(t, h.server.Comments(), "comments are posted by Flush")
	s.Flush(context.Background())

	current := h.server.CommentsFor("owner", "repo", 42)
	require.Len(t, current, 2, "one comment per image, not per ref")
	var app, worker string
	for _, c := range current {
		switch {
		case strings.Contains(c.Body, "<!-- atmos:ci:container:image:ghcr.io/org/app -->"):
			app = c.Body
		case strings.Contains(c.Body, "<!-- atmos:ci:container:image:ghcr.io/org/worker -->"):
			worker = c.Body
		}
	}
	require.NotEmpty(t, app)
	assert.Contains(t, app, "## 🐳 ghcr.io/org/app:1.2.3")
	assert.Contains(t, app, "## 🐳 ghcr.io/org/app:latest")
	require.NotEmpty(t, worker)
	assert.Contains(t, worker, "## 🐳 ghcr.io/org/worker:1.2.3")
	assert.NotContains(t, worker, "app:latest")

	// A second flush has nothing left to post.
	before := len(h.server.Comments())
	s.Flush(context.Background())
	assert.Len(t, h.server.Comments(), before)
}

func TestWriteKeyFallsBackToRepoTagAndSkipsUnnamedImages(t *testing.T) {
	h := newHarness(t)
	cfg := ciConfig(true)

	Write(context.Background(), cfg, info("tagged:2"), ctr.ImageSummaryOptions{})
	require.Len(t, h.server.Comments(), 1)
	assert.Contains(t, h.server.Comments()[0].Body, "<!-- atmos:ci:container:image:tagged -->")

	Write(context.Background(), cfg, &ctr.ImageInfo{ID: "sha256:x"}, ctr.ImageSummaryOptions{})
	assert.Len(t, h.server.Comments(), 1, "an image without a name has no comment key")
}

func TestWriteTruncatesCommentButNotJobSummary(t *testing.T) {
	h := newHarness(t)
	big := &ctr.ImageInfo{RepoTags: []string{"img:1"}, RawInspectJSON: strings.Repeat("x", 80000) + "END-OF-RAW-JSON"}

	Write(context.Background(), ciConfig(true), big, ctr.ImageSummaryOptions{Image: "img:1"})

	current := h.server.CommentsFor("owner", "repo", 42)
	require.Len(t, current, 1)
	assert.LessOrEqual(t, len([]rune(current[0].Body)), MaxCommentChars+100, "the marker is the only addition to the truncated body")
	assert.Contains(t, current[0].Body, "Comment truncated; see the job summary for the full report.")
	assert.NotContains(t, current[0].Body, "END-OF-RAW-JSON")
	assert.Contains(t, h.summary(t), "END-OF-RAW-JSON", "the job summary keeps the full report")
}

func TestWriteCommentFailureDoesNotFailAndWarnsWithImage(t *testing.T) {
	h := newHarness(t, ghtest.WithFailure("POST", "/repos/owner/repo/issues/42/comments", 422, "Validation Failed"))
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	assert.NotPanics(t, func() {
		Write(context.Background(), ciConfig(true), info("ghcr.io/org/app:1"), ctr.ImageSummaryOptions{Image: "ghcr.io/org/app:1"})
	})

	assert.Contains(t, h.summary(t), "## 🐳 ghcr.io/org/app:1", "the job summary is written before the comment is attempted")
	assert.Empty(t, h.server.Comments())
	assert.Contains(t, logs.String(), "WARN")
	assert.Contains(t, logs.String(), "ghcr.io/org/app")
}

func TestWriteSkipsEverythingWhenSummaryDisabledOrNoInfo(t *testing.T) {
	h := newHarness(t)
	cfg := ciConfig(true)
	cfg.CI.Summary.Enabled = boolPtr(false)

	Write(context.Background(), cfg, info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})
	Write(context.Background(), ciConfig(true), nil, ctr.ImageSummaryOptions{Image: "img:1"})
	Write(context.Background(), nil, info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})
	Write(context.Background(), &schema.AtmosConfiguration{}, info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})

	assert.Empty(t, h.summary(t))
	assert.Empty(t, h.server.Requests())
	assert.Empty(t, h.local.String())
}

func TestWriteDoesNotCommentWhenSummaryWasRenderedLocally(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter := NewMockReporter(ctrl)
	prev := newReporter
	newReporter = func(*schema.AtmosConfiguration) ci.Reporter { return reporter }
	t.Cleanup(func() { newReporter = prev })
	// The mock expects no Comment call, so any comment fails the test.
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "generic", Local: true}, nil)

	Write(context.Background(), ciConfig(true), info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})
}

func TestWriteDoesNotCommentWhenSummaryFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter := NewMockReporter(ctrl)
	prev := newReporter
	newReporter = func(*schema.AtmosConfiguration) ci.Reporter { return reporter }
	t.Cleanup(func() { newReporter = prev })
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{}, assert.AnError)

	assert.NotPanics(t, func() {
		Write(context.Background(), ciConfig(true), info("img:1"), ctr.ImageSummaryOptions{Image: "img:1"})
	})
}

func TestWriteCommentsOnCallsReporterOncePerImageWithStrippedKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	reporter := NewMockReporter(ctrl)
	prev := newReporter
	newReporter = func(*schema.AtmosConfiguration) ci.Reporter { return reporter }
	t.Cleanup(func() { newReporter = prev })
	var requests []ci.CommentRequest
	reporter.EXPECT().Summary(gomock.Any()).Return(ci.Receipt{Provider: "github"}, nil).Times(2)
	reporter.EXPECT().Comment(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req ci.CommentRequest) (ci.Receipt, error) {
		requests = append(requests, req)
		return ci.Receipt{Provider: "github"}, nil
	}).Times(1)

	s := NewSession(ciConfig(true))
	s.Add(info("reg.io/team/app:a"), ctr.ImageSummaryOptions{Image: "reg.io/team/app:a"})
	s.Add(info("reg.io/team/app:b"), ctr.ImageSummaryOptions{Image: "reg.io/team/app:b"})
	s.Flush(context.Background())

	require.Len(t, requests, 1)
	assert.Equal(t, "container:image:reg.io/team/app", requests[0].Key)
}

func TestInspectReportsInspectedImageAndIgnoresInspectFailures(t *testing.T) {
	h := newHarness(t)
	runtime := inspectStub{images: map[string]*ctr.ImageInfo{"ok:1": info("ok:1")}}

	Inspect(context.Background(), runtime, ciConfig(true), "missing:1", "")
	Inspect(context.Background(), runtime, ciConfig(true), "", "")
	assert.Empty(t, h.server.Comments())

	Inspect(context.Background(), runtime, ciConfig(true), "ok:1", "sha256:pushed")
	require.Len(t, h.server.Comments(), 1)
	assert.Contains(t, h.server.Comments()[0].Body, "sha256:pushed")
	assert.Contains(t, h.server.Comments()[0].Body, "<!-- atmos:ci:container:image:ok -->")
}

func TestImageRepositoryAndCommentKey(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		repository string
		key        string
	}{
		{name: "repo with tag", ref: "repo:tag", repository: "repo", key: "container:image:repo"},
		{name: "repo without tag", ref: "repo", repository: "repo", key: "container:image:repo"},
		{name: "namespaced registry repo with tag", ref: "registry/ns/repo:tag", repository: "registry/ns/repo", key: "container:image:registry/ns/repo"},
		{name: "registry with port and tag", ref: "localhost:5000/team/app:1.2", repository: "localhost:5000/team/app", key: "container:image:localhost:5000/team/app"},
		{name: "registry with port and no tag", ref: "localhost:5000/app", repository: "localhost:5000/app", key: "container:image:localhost:5000/app"},
		{name: "digest only", ref: "repo@sha256:0123abcd", repository: "repo", key: "container:image:repo"},
		{name: "tag and digest", ref: "ghcr.io/org/app:1.2@sha256:0123abcd", repository: "ghcr.io/org/app", key: "container:image:ghcr.io/org/app"},
		{name: "sha tag", ref: "ghcr.io/org/app:sha-0123456", repository: "ghcr.io/org/app", key: "container:image:ghcr.io/org/app"},
		{name: "empty", ref: "", repository: "", key: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.repository, ImageRepository(tt.ref))
			assert.Equal(t, tt.key, CommentKey(tt.ref))
		})
	}
}

func TestMultiTagPushesShareOneKey(t *testing.T) {
	refs := []string{"ghcr.io/org/app:1.2.3", "ghcr.io/org/app:latest", "ghcr.io/org/app:sha-abc", "ghcr.io/org/app@sha256:ff"}
	want := CommentKey(refs[0])
	for _, ref := range refs {
		assert.Equal(t, want, CommentKey(ref), ref)
	}
}
