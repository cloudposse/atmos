package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newContainerReporter builds the CI reporter that receives image summaries. It is a
// package-level variable so tests can inject a mock ci.Reporter; production code always
// uses ci.NewReporter, which owns provider detection and the ci.* gating.
var newContainerReporter = ci.NewReporter

//go:generate mockgen -typed -destination=mock_reporter_test.go -package=step github.com/cloudposse/atmos/pkg/ci Reporter

// imageCommentKeyPrefix namespaces the upsert key of the per-image pull request comment.
const imageCommentKeyPrefix = "container:image:"

// writeContainerImageSummary renders the image summary and sends it to the CI reporter: always
// to the job summary and, when the summary reached a CI provider, to one upserted pull request
// comment per image (gated by ci.comments.enabled inside the reporter). It is best-effort:
// failures are logged and never fail the build or push. The ci.summary gate is checked up front
// only to skip rendering when nothing would be reported; the reporter still owns the
// per-destination gating.
func writeContainerImageSummary(ctx context.Context, config *schema.AtmosConfiguration, info *container.ImageInfo, opts container.ImageSummaryOptions) {
	if !ci.SummaryEnabled(config) || info == nil {
		return
	}
	md, err := container.RenderImageSummary(config, info, opts)
	if err != nil {
		log.Warn("container step: failed to render CI image summary", "image", opts.Image, "error", err)
		return
	}
	if md == "" {
		return
	}

	reporter := newContainerReporter(config)
	receipt, err := reporter.Summary(md)
	if err != nil {
		log.Debug("container step: failed to write CI image summary", "image", opts.Image, "error", err)
		return
	}
	if receipt.Local {
		// The summary was already previewed locally; a comment would only repeat it.
		return
	}

	image := opts.Image
	if image == "" && len(info.RepoTags) > 0 {
		image = info.RepoTags[0]
	}
	postImageComment(ctx, reporter, md, image)
}

// postImageComment upserts the pull request comment for an image. It does nothing without an image name,
// because the comment key would not identify the image.
func postImageComment(ctx context.Context, reporter ci.Reporter, md, image string) {
	if image == "" {
		return
	}
	if _, err := reporter.Comment(ctx, ci.CommentRequest{Body: md, Key: imageCommentKeyPrefix + image}); err != nil {
		log.Debug("container step: failed to post CI image comment", "image", image, "error", err)
	}
}

func writePushedImageSummaries(ctx context.Context, runtime container.Runtime, config *schema.AtmosConfiguration, pushes []*container.PushResult) {
	if !ci.SummaryEnabled(config) {
		return
	}
	for _, pushed := range pushes {
		if pushed == nil || pushed.Image == "" {
			continue
		}
		info, err := runtime.ImageInspect(ctx, pushed.Image)
		if err != nil {
			log.Debug("container step: failed to inspect pushed image for CI summary", "image", pushed.Image, "error", err)
			continue
		}
		writeContainerImageSummary(ctx, config, info, container.ImageSummaryOptions{
			Image:  pushed.Image,
			Digest: pushed.Digest,
		})
	}
}
