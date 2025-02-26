package validator

import (
	"github.com/cloudposse/atmos/pkg/downloader"
)

// URLFetcher fetches content from a URL.
type URLFetcher struct {
	URL            string
	fileDownloader downloader.FileDownloader
}

func (u *URLFetcher) Fetch() ([]byte, error) {
	return u.fileDownloader.FetchData(u.URL)
}
