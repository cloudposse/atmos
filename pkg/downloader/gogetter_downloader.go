package downloader

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

type goGetterClient struct {
	client *getter.Client
}

// Get executes the download.
func (c *goGetterClient) Get() error {
	return c.client.Get()
}

type goGetterClientFactory struct{}

// NewClient creates a new `go-getter` client.
func (f *goGetterClientFactory) NewClient(ctx context.Context, src, dest string, mode ClientMode) (DownloadClient, error) {
	clientMode := getter.ClientModeAny

	switch mode {
	case ClientModeAny:
		clientMode = getter.ClientModeAny
	case ClientModeDir:
		clientMode = getter.ClientModeDir
	case ClientModeFile:
		clientMode = getter.ClientModeFile
	}

	client := &getter.Client{
		Ctx:  ctx,
		Src:  src,
		Dst:  dest,
		Mode: clientMode,
	}

	return &goGetterClient{client: client}, nil
}

// registerCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func registerCustomDetectors(atmosConfig *schema.AtmosConfiguration) {
	getter.Detectors = append(
		[]getter.Detector{
			NewCustomGitHubDetector(atmosConfig),
		},
		getter.Detectors...,
	)
}

func NewGoGetterDownloader(atmosConfig *schema.AtmosConfiguration) FileDownloader {
	registerCustomDetectors(atmosConfig)
	return NewFileDownloader(&goGetterClientFactory{})
}
