package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxListKeys is the most entries S3 returns in one page.
const maxListKeys = 1000

// List implements [blob.Store]. The cursor is the S3 continuation token.
func (s *Store) List(ctx context.Context, opts blob.ListOptions) (*blob.ListPage, error) {
	defer perf.Track(nil, "blob.s3.List")()

	api, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	in := &s3.ListObjectsV2Input{
		Bucket:            aws.String(s.cfg.bucket),
		Prefix:            optionalString(s.listPrefix(opts.Prefix)),
		Delimiter:         optionalString(opts.Delimiter),
		ContinuationToken: optionalString(opts.Cursor),
	}
	if opts.Limit > 0 {
		in.MaxKeys = aws.Int32(int32(min(opts.Limit, maxListKeys)))
	}
	out, err := api.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, classify(request{op: blob.OpList, key: opts.Prefix}, err)
	}
	page := &blob.ListPage{}
	for _, object := range out.Contents {
		page.Objects = append(page.Objects, blob.ObjectInfo{
			Key:          s.storeKey(aws.ToString(object.Key)),
			Size:         aws.ToInt64(object.Size),
			ETag:         aws.ToString(object.ETag),
			LastModified: aws.ToTime(object.LastModified),
		})
	}
	for _, prefix := range out.CommonPrefixes {
		page.CommonPrefixes = append(page.CommonPrefixes, s.storeKey(aws.ToString(prefix.Prefix)))
	}
	if aws.ToBool(out.IsTruncated) {
		page.NextCursor = aws.ToString(out.NextContinuationToken)
	}
	return page, nil
}

// listPrefix maps a store prefix to the bucket prefix.
func (s *Store) listPrefix(prefix string) string {
	if s.cfg.prefix == "" {
		return prefix
	}
	return s.cfg.prefix + "/" + prefix
}
