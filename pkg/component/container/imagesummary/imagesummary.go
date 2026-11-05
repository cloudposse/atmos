// Package imagesummary reports container image summaries to CI. It is the single implementation
// shared by the container component (atmos container build/push) and the container workflow step.
//
// Every inspected image is appended to the job summary. When ci.comments.enabled is on and the
// summary reached a CI provider, one pull request comment per image repository is upserted and
// updated in place across builds and pushes. Nothing is previewed in the log when comments are off.
package imagesummary

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci"
	ctr "github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CommentKeyPrefix namespaces the upsert key of the per-image pull request comment.
const CommentKeyPrefix = "container:image:"

// commentSeparator divides the summaries of several refs of one image inside a single comment.
const commentSeparator = "\n\n---\n\n"

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -typed -destination=mock_reporter_test.go -package=imagesummary github.com/cloudposse/atmos/pkg/ci Reporter

// newReporter builds the CI reporter that receives image summaries. It is a package-level variable
// so tests can inject a reporter; production code always uses ci.NewReporter, which owns provider
// detection and the ci.* gating.
var newReporter = ci.NewReporter

// Session collects the image summaries of one build or push run. Each image is written to the job
// summary as soon as it is added; the pull request comments are posted once, by Flush, so that every
// ref pushed in the run is listed in the single comment of its image.
type Session struct {
	config   *schema.AtmosConfiguration
	reporter ci.Reporter
	// order holds the comment keys in first-seen order.
	order []string
	// pending maps a comment key to the image repository and the rendered summaries queued for it.
	pending map[string]*pendingComment
}

type pendingComment struct {
	repository string
	bodies     []string
}

// NewSession returns a Session that reports through the CI reporter built for config.
func NewSession(config *schema.AtmosConfiguration) *Session {
	defer perf.Track(config, "imagesummary.NewSession")()

	return &Session{config: config, pending: map[string]*pendingComment{}}
}

// Write reports one image: job summary now, pull request comment immediately after. It is the
// one-shot form of NewSession, Add, and Flush.
func Write(ctx context.Context, config *schema.AtmosConfiguration, info *ctr.ImageInfo, opts ctr.ImageSummaryOptions) {
	defer perf.Track(config, "imagesummary.Write")()

	s := NewSession(config)
	s.Add(info, opts)
	s.Flush(ctx)
}

// Inspect inspects image through runtime and reports it like Write. Inspect failures are logged
// and never fail the build or push.
func Inspect(ctx context.Context, runtime ctr.Runtime, config *schema.AtmosConfiguration, image, digest string) {
	defer perf.Track(config, "imagesummary.Inspect")()

	s := NewSession(config)
	s.AddInspected(ctx, runtime, image, digest)
	s.Flush(ctx)
}

// AddInspected inspects image through runtime and adds the result to the session.
func (s *Session) AddInspected(ctx context.Context, runtime ctr.Runtime, image, digest string) {
	defer perf.Track(s.config, "imagesummary.Session.AddInspected")()

	if !ci.SummaryEnabled(s.config) || image == "" {
		return
	}
	info, err := runtime.ImageInspect(ctx, image)
	if err != nil {
		log.Debug("container: failed to inspect image for CI summary", "image", image, "error", err)
		return
	}
	s.Add(info, ctr.ImageSummaryOptions{Image: image, Digest: digest})
}

// Add renders the image summary and appends it to the job summary. When the summary reached a CI
// provider and ci.comments.enabled is on, the summary is also queued for the image's comment.
// It is best-effort: failures are logged and never fail the build or push. The ci.summary gate
// is checked up front only to skip the rendering when nothing would be reported; the reporter
// still owns the per-destination gating.
func (s *Session) Add(info *ctr.ImageInfo, opts ctr.ImageSummaryOptions) {
	defer perf.Track(s.config, "imagesummary.Session.Add")()

	if !ci.SummaryEnabled(s.config) || info == nil {
		return
	}
	md, err := ctr.RenderImageSummary(s.config, info, opts)
	if err != nil {
		log.Warn("container: failed to render CI image summary", "image", opts.Image, "error", err)
		return
	}
	if md == "" {
		return
	}

	if s.reporter == nil {
		s.reporter = newReporter(s.config)
	}
	receipt, err := s.reporter.Summary(md)
	if err != nil {
		log.Debug("container: failed to write CI image summary", "image", opts.Image, "error", err)
		return
	}
	// A local summary was already previewed, and a comment is only attempted when it is enabled.
	// Calling Comment while it is off would make the reporter preview the whole body in the log.
	if receipt.Local || !ci.CommentsEnabled(s.config) {
		return
	}
	s.queue(firstNonEmpty(opts.Image, firstString(info.RepoTags)), md)
}

// queue holds md for the comment of image. An image without a name is skipped because the
// comment key would not identify it.
func (s *Session) queue(image, md string) {
	key := CommentKey(image)
	if key == "" {
		return
	}
	entry, ok := s.pending[key]
	if !ok {
		entry = &pendingComment{repository: ImageRepository(image)}
		s.pending[key] = entry
		s.order = append(s.order, key)
	}
	entry.bodies = append(entry.bodies, md)
}

// Flush posts one pull request comment per image repository queued by Add, then clears the queue.
// The comment lists every ref added for the image and is truncated to fit the provider's limit;
// the job summary keeps the full content. Failures are logged at Warn and never returned.
func (s *Session) Flush(ctx context.Context) {
	defer perf.Track(s.config, "imagesummary.Session.Flush")()

	for _, key := range s.order {
		entry := s.pending[key]
		body := TruncateComment(strings.Join(entry.bodies, commentSeparator))
		if _, err := s.reporter.Comment(ctx, ci.CommentRequest{Body: body, Key: key}); err != nil {
			log.Warn("container: failed to post CI image comment", "image", entry.repository, "error", err)
		}
	}
	s.order = nil
	s.pending = map[string]*pendingComment{}
}

// CommentKey returns the upsert key of the pull request comment for an image reference. The key
// names the image repository without its tag or digest so every build and push of an image updates
// one comment. It is empty when ref is empty.
func CommentKey(ref string) string {
	defer perf.Track(nil, "imagesummary.CommentKey")()

	repository := ImageRepository(ref)
	if repository == "" {
		return ""
	}
	return CommentKeyPrefix + repository
}

// ImageRepository strips the tag and digest from an image reference, keeping the registry (with
// any port) and repository path: "ghcr.io/org/app:1.2@sha256:abc" becomes "ghcr.io/org/app".
func ImageRepository(ref string) string {
	defer perf.Track(nil, "imagesummary.ImageRepository")()

	if before, _, found := strings.Cut(ref, "@"); found {
		ref = before
	}
	// A colon is a tag separator only after the last slash; before it, it is a registry port.
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref = ref[:colon]
	}
	return ref
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
