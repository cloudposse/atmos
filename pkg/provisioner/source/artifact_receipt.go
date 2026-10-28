package source

import (
	"context"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// WithRuntimePreservation keeps Terraform runtime artifacts while refreshing source-owned files.
// Callers supply the component configuration so custom backend paths take precedence over source ownership.
func WithRuntimePreservation(componentConfig map[string]any) VendorSourceOption {
	defer perf.Track(nil, "source.WithRuntimePreservation")()
	return func(opts *vendorSourceOptions) { opts.preserveRuntime = true; opts.componentConfig = componentConfig }
}

// WithArtifactReceipt receives provenance captured from the fetched source before runtime files are merged.
func WithArtifactReceipt(receipt *downloader.ResolvedArtifact) VendorSourceOption {
	defer perf.Track(nil, "source.WithArtifactReceipt")()
	return func(opts *vendorSourceOptions) { opts.artifact = receipt }
}

// captureArtifact uses downloader-owned fetch metadata; the destination is never treated as a Git checkout.
func (opts vendorSourceOptions) captureArtifact(ctx context.Context, config *schema.AtmosConfiguration, uri, staged string, metadata downloader.FetchMetadata) error {
	receipt := opts.artifact
	if receipt == nil {
		return nil
	}
	if metadata.GitCommit != "" {
		*receipt = downloader.ResolvedArtifact{Declared: downloader.RedactSource(uri), Resolved: downloader.RedactSource(uri), Kind: "git", Identity: metadata.GitCommit}
		return nil
	}
	artifact, err := downloader.ResolveArtifact(ctx, config, uri, staged)
	if err != nil {
		return err
	}
	artifact.ETag, artifact.Modified = metadata.ETag, metadata.LastModified
	*receipt = artifact
	return nil
}
