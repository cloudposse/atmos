package downloader

import (
	"context"
	"sync"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

// detectorsMutex guards modifications to getter.Detectors.
var detectorsMutex sync.Mutex

type goGetterClient struct {
	client *getter.Client
}

// Get executes the download.
func (c *goGetterClient) Get() error {
	return c.client.Get()
}

type goGetterClientFactory struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewClient creates a new `go-getter` client.
func (f *goGetterClientFactory) NewClient(ctx context.Context, src, dest string, mode ClientMode) (DownloadClient, error) {
	clientMode := getter.ClientModeAny
	registerCustomDetectors(f.atmosConfig, src)
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
		Getters: map[string]getter.Getter{
			// Overriding 'git'
			"git":   &CustomGitGetter{},
			"file":  &getter.FileGetter{},
			"hg":    &getter.HgGetter{},
			"http":  &getter.HttpGetter{},
			"https": &getter.HttpGetter{},
			// "s3": &getter.S3Getter{}, // add as needed
			// "gcs": &getter.GCSGetter{},
		},
	}

	return &goGetterClient{client: client}, nil
}

// registerCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func registerCustomDetectors(atmosConfig *schema.AtmosConfiguration, src string) {
	detectorsMutex.Lock()
	defer detectorsMutex.Unlock()

	getter.Detectors = append(
		[]getter.Detector{
			&CustomGitDetector{atmosConfig: atmosConfig, source: src},
		},
		getter.Detectors...,
	)
}

func NewGoGetterDownloader(atmosConfig *schema.AtmosConfiguration) FileDownloader {
	return NewFileDownloader(&goGetterClientFactory{
		atmosConfig: atmosConfig,
	})
}
