package s3upload

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// UploadReader compares an object before uploading a repeatable stream.
func UploadReader(ctx context.Context, client Client, reader io.ReadSeeker, size int64, opts Options) (bool, error) {
	defer perf.Track(nil, "s3upload.UploadReader")()
	if size < 0 || size > maxObjectSize {
		return false, errUtils.ErrS3UploadSource
	}
	bucket, key, err := ParseDestination(opts.Destination)
	if err != nil {
		return false, err
	}
	input, err := prepareReader(ctx, reader, bucket, object{filename: opts.Source, key: key, size: size}, opts)
	if err != nil {
		return false, err
	}
	remote, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: input.Bucket, Key: input.Key})
	if err != nil && !isMissing(err) {
		return false, err
	}
	if err == nil && objectMatches(remote, input) {
		return false, nil
	}
	_, err = client.PutObject(ctx, input)
	return err == nil, err
}
