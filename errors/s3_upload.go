package errors

import "errors"

var (
	// ErrS3Upload indicates a failed S3 artifact upload.
	ErrS3Upload = errors.New("S3 upload failed")
	// ErrS3UploadDestination indicates an invalid S3 destination URI.
	ErrS3UploadDestination = errors.New("invalid S3 upload destination")
	// ErrS3UploadSource indicates an unsupported local source.
	ErrS3UploadSource = errors.New("invalid S3 upload source")
)
