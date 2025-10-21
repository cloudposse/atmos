package downloader

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

// FileDownloader handles downloading files and directories from various sources
// without exposing the underlying implementation.
type FileDownloader interface {
	// Fetch fetches content from a given source and saves it to the destination
	Fetch(src, dest string, mode ClientMode, timeout time.Duration) error

	// FetchAndAutoParse downloads a remote file, detects its format, and parses it
	FetchAndAutoParse(src string) (any, error)

	// FetchAndParseByExtension downloads a remote file and parses it based on its extension
	FetchAndParseByExtension(src string) (any, error)

	// FetchAndParseRaw downloads a remote file and always returns it as a raw string
	FetchAndParseRaw(src string) (any, error)

	// FetchData fetches content from a given source and returns it as a byte slice
	FetchData(src string) ([]byte, error)
}

// ClientFactory abstracts the creation of a downloader client for better testability.
type ClientFactory interface {
	NewClient(ctx context.Context, src, dest string, mode ClientMode) (DownloadClient, error)
}

// DownloadClient defines an interface for download operations.
type DownloadClient interface {
	Get() error
}

// ClientMode represents different modes for downloading (file, dir, etc.)
type ClientMode int

const (
	ClientModeInvalid ClientMode = iota

	// ClientModeAny downloads anything it can. In this mode, dst must
	// be a directory. If src is a file, it is saved into the directory
	// with the basename of the URL. If src is a directory or archive,
	// it is unpacked directly into dst.
	ClientModeAny

	// Be a file path (doesn't have to exist). The src must point to a single
	// file. It is saved as dst.
	ClientModeFile

	// A directory path (doesn't have to exist). The src must point to an
	// archive or directory (such as in s3).
	ClientModeDir
)
