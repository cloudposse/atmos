package step

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component/container/imagesummary"
	"github.com/cloudposse/atmos/pkg/container"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeContainerImageSummary reports one built image to CI. The job summary and the pull request
// comment are written by pkg/component/container/imagesummary, which the container component
// (atmos container build/push) shares, so both paths key, gate, and truncate identically.
func writeContainerImageSummary(ctx context.Context, config *schema.AtmosConfiguration, info *container.ImageInfo, opts container.ImageSummaryOptions) {
	imagesummary.Write(ctx, config, info, opts)
}

// writePushedImageSummaries reports every pushed image to CI. Each image is appended to the job
// summary; the pull request comment is posted once per image repository and lists every ref pushed
// in this step.
func writePushedImageSummaries(ctx context.Context, runtime container.Runtime, config *schema.AtmosConfiguration, pushes []*container.PushResult) {
	if !ci.SummaryEnabled(config) {
		return
	}
	summaries := imagesummary.NewSession(config)
	defer summaries.Flush(ctx)
	for _, pushed := range pushes {
		if pushed == nil || pushed.Image == "" {
			continue
		}
		info, err := runtime.ImageInspect(ctx, pushed.Image)
		if err != nil {
			log.Debug("container step: failed to inspect pushed image for CI summary", "image", pushed.Image, "error", err)
			continue
		}
		summaries.Add(info, container.ImageSummaryOptions{Image: pushed.Image, Digest: pushed.Digest})
	}
}
