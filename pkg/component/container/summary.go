package container

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci"
	ctr "github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newContainerReporter builds the CI reporter that receives image summaries. It is a
// package-level variable so tests can inject a mock ci.Reporter; production code always
// uses ci.NewReporter, which owns provider detection and the ci.* gating.
var newContainerReporter = ci.NewReporter

//go:generate mockgen -typed -destination=mock_reporter_test.go -package=container github.com/cloudposse/atmos/pkg/ci Reporter

// imageCommentKeyPrefix namespaces the upsert key of the per-image pull request comment.
const imageCommentKeyPrefix = "container:image:"

// writeImageSummary renders the image summary and sends it to the CI reporter: always to the
// job summary and, when the summary reached a CI provider, to one upserted pull request comment
// per image (gated by ci.comments.enabled inside the reporter). It is best-effort: failures are
// logged and never fail the build or push. The ci.summary gate is checked up front only to skip
// the image inspection and rendering when nothing would be reported; the reporter still owns the
// per-destination gating.
func writeImageSummary(ctx context.Context, config *schema.AtmosConfiguration, info *ctr.ImageInfo, opts ctr.ImageSummaryOptions) {
	if !ci.SummaryEnabled(config) || info == nil {
		return
	}
	md, err := ctr.RenderImageSummary(config, info, opts)
	if err != nil {
		log.Warn("container component: failed to render CI image summary", "image", opts.Image, "error", err)
		return
	}
	if md == "" {
		return
	}

	reporter := newContainerReporter(config)
	receipt, err := reporter.Summary(md)
	if err != nil {
		log.Debug("container component: failed to write CI image summary", "image", opts.Image, "error", err)
		return
	}
	if receipt.Local {
		// The summary was already previewed locally; a comment would only repeat it.
		return
	}

	postImageComment(ctx, reporter, md, firstNonEmpty([]string{opts.Image, firstRepoTag(info)}))
}

// postImageComment upserts the pull request comment for an image. It does nothing without an image name,
// because the comment key would not identify the image.
func postImageComment(ctx context.Context, reporter ci.Reporter, md, image string) {
	if image == "" {
		return
	}
	if _, err := reporter.Comment(ctx, ci.CommentRequest{Body: md, Key: imageCommentKeyPrefix + image}); err != nil {
		log.Debug("container component: failed to post CI image comment", "image", image, "error", err)
	}
}

func inspectAndWriteImageSummary(ctx context.Context, runtime ctr.Runtime, config *schema.AtmosConfiguration, image, digest string) {
	if !ci.SummaryEnabled(config) || image == "" {
		return
	}
	info, err := runtime.ImageInspect(ctx, image)
	if err != nil {
		log.Debug("container component: failed to inspect image for CI summary", "image", image, "error", err)
		return
	}
	writeImageSummary(ctx, config, info, ctr.ImageSummaryOptions{Image: image, Digest: digest})
}

func firstRepoTag(info *ctr.ImageInfo) string {
	if len(info.RepoTags) == 0 {
		return ""
	}
	return info.RepoTags[0]
}

func firstNonEmpty(values []string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
