package marketplace

import (
	"context"
)

// DownloaderInterface defines the interface for downloading agents.
type DownloaderInterface interface {
	Download(ctx context.Context, source *SourceInfo) (string, error)
}
