package s3

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Number parsing parameters for Content-Range totals.
const (
	decimalBase = 10
	bitSize64   = 64
)

// Get implements [blob.Store].
func (s *Store) Get(ctx context.Context, key string, opts blob.GetOptions) (*blob.Object, error) {
	defer perf.Track(nil, "blob.s3.Get")()

	if err := s.validateKey(key); err != nil {
		return nil, err
	}
	if opts.Offset < 0 {
		return nil, blob.NewError(blob.KindInvalid, blob.OpGet, key, fmt.Errorf("%w: negative offset", errUtils.ErrBlobInvalid))
	}
	api, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	in := &s3.GetObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(s.objectKey(key)), Range: rangeHeader(opts)}
	out, err := api.GetObject(ctx, in)
	if err != nil {
		return nil, classify(request{op: blob.OpGet, key: key}, err)
	}
	info := blob.ObjectInfo{
		Key:          key,
		Size:         objectSize(out),
		ETag:         aws.ToString(out.ETag),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: aws.ToTime(out.LastModified),
		Metadata:     out.Metadata,
	}
	return &blob.Object{ObjectInfo: info, Body: out.Body}, nil
}

// rangeHeader renders a partial read as an HTTP Range header.
func rangeHeader(opts blob.GetOptions) *string {
	if !opts.IsRange() {
		return nil
	}
	if opts.Length > 0 {
		return aws.String(fmt.Sprintf("bytes=%d-%d", opts.Offset, opts.Offset+opts.Length-1))
	}
	return aws.String(fmt.Sprintf("bytes=%d-", opts.Offset))
}

// objectSize returns the full object size, using Content-Range for partial responses.
func objectSize(out *s3.GetObjectOutput) int64 {
	if out.ContentRange != nil {
		if _, total, ok := strings.Cut(aws.ToString(out.ContentRange), "/"); ok {
			if size, err := strconv.ParseInt(total, decimalBase, bitSize64); err == nil {
				return size
			}
		}
	}
	return aws.ToInt64(out.ContentLength)
}

// Stat implements [blob.Store].
func (s *Store) Stat(ctx context.Context, key string, opts blob.StatOptions) (*blob.ObjectInfo, error) {
	defer perf.Track(nil, "blob.s3.Stat")()

	if err := s.validateKey(key); err != nil {
		return nil, err
	}
	api, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	out, err := api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(s.objectKey(key))})
	if err != nil {
		return nil, classify(request{op: blob.OpStat, key: key}, err)
	}
	info := &blob.ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ETag:         aws.ToString(out.ETag),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: aws.ToTime(out.LastModified),
		Metadata:     out.Metadata,
	}
	if opts.IncludeTags {
		if info.Tags, err = s.tags(ctx, api, key); err != nil {
			return nil, err
		}
	}
	return info, nil
}

// tags loads the object tags.
func (s *Store) tags(ctx context.Context, api API, key string) (map[string]string, error) {
	out, err := api.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(s.objectKey(key))})
	if err != nil {
		return nil, classify(request{op: "tags", key: key}, err)
	}
	tags := make(map[string]string, len(out.TagSet))
	for _, tag := range out.TagSet {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return tags, nil
}
