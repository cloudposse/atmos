package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// putPlan is a validated write ready to send.
type putPlan struct {
	api       API
	key       string
	objectKey string
	opts      blob.PutOptions
	tags      string
	partSize  int64
}

func (p *putPlan) request(mutating bool) request {
	return request{op: blob.OpPut, key: p.key, mutating: mutating, conditional: p.opts.Condition.IsConditional()}
}

// Put implements [blob.Store]. A body that fits one part is sent with a single PutObject; anything
// larger, or of unknown length, streams as a bounded multipart upload.
func (s *Store) Put(ctx context.Context, key string, body io.Reader, opts blob.PutOptions) (*blob.ObjectInfo, error) {
	defer perf.Track(nil, "blob.s3.Put")()

	plan, err := s.planPut(ctx, key, opts)
	if err != nil {
		return nil, err
	}
	first, carry, err := readFirstPart(body, plan.partSize, opts.Size)
	if err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpPut, key, fmt.Errorf("read body: %w", err))
	}
	if len(carry) == 0 {
		return s.putSingle(ctx, plan, first)
	}
	return s.putMultipart(ctx, plan, first, carry, body)
}

// planPut validates the request and prepares the client, tags, and part size.
func (s *Store) planPut(ctx context.Context, key string, opts blob.PutOptions) (*putPlan, error) {
	if err := s.validateKey(key); err != nil {
		return nil, err
	}
	tags := blob.MergeTags(s.cfg.tags, opts.Tags)
	if err := blob.ValidateTags(tags); err != nil {
		return nil, err
	}
	if opts.Condition.Kind == blob.ConditionIfMatch && opts.Condition.ETag == "" {
		return nil, blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: if-match requires an ETag", errUtils.ErrBlobInvalid))
	}
	if err := s.Capabilities().CheckPut(storeName, key, opts.Condition); err != nil {
		return nil, err
	}
	if err := s.ensureConditional(ctx, key, opts.Condition); err != nil {
		return nil, err
	}
	api, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	return &putPlan{
		api:       api,
		key:       key,
		objectKey: s.objectKey(key),
		opts:      opts,
		tags:      encodeTags(tags),
		partSize:  effectivePartSize(opts.Size, s.cfg.partSize),
	}, nil
}

// effectivePartSize grows the part size so a known-size body fits within the S3 part limit.
func effectivePartSize(size, partSize int64) int64 {
	if size <= 0 {
		return partSize
	}
	needed := (size + MaxParts - 1) / MaxParts
	return max(partSize, needed)
}

// readFirstPart reads up to one part. When the body continues past the part, carry holds the one
// byte already consumed from the next part; an empty carry means the body ended.
func readFirstPart(body io.Reader, partSize, size int64) (first, carry []byte, err error) {
	var buf bytes.Buffer
	if size >= 0 && size < partSize {
		buf.Grow(int(size))
	}
	if _, err := io.Copy(&buf, io.LimitReader(body, partSize)); err != nil {
		return nil, nil, err
	}
	if int64(buf.Len()) < partSize {
		return buf.Bytes(), nil, nil
	}
	probe := make([]byte, 1)
	n, err := io.ReadFull(body, probe)
	switch {
	case n == 1:
		return buf.Bytes(), probe, nil
	case errors.Is(err, io.EOF):
		return buf.Bytes(), nil, nil
	default:
		return nil, nil, err
	}
}

// putSingle sends a body that fits one part with one PutObject.
func (s *Store) putSingle(ctx context.Context, plan *putPlan, data []byte) (*blob.ObjectInfo, error) {
	size := int64(len(data))
	if plan.opts.Size >= 0 && size != plan.opts.Size {
		return nil, sizeMismatch(plan.key, size, plan.opts.Size)
	}
	cond := plan.opts.Condition
	in := &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.bucket),
		Key:           aws.String(plan.objectKey),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(size),
		ContentType:   optionalString(plan.opts.ContentType),
		Metadata:      plan.opts.Metadata,
		Tagging:       optionalString(plan.tags),
	}
	in.IfNoneMatch, in.IfMatch = conditionHeaders(cond)
	out, err := plan.api.PutObject(ctx, in, retryOptions(cond.IsConditional())...)
	if err != nil {
		return nil, classify(plan.request(true), err)
	}
	return &blob.ObjectInfo{Key: plan.key, Size: size, ETag: aws.ToString(out.ETag)}, nil
}

// conditionHeaders maps a condition to the If-None-Match and If-Match request headers.
func conditionHeaders(cond blob.Condition) (ifNoneMatch, ifMatch *string) {
	switch cond.Kind {
	case blob.ConditionIfAbsent:
		return aws.String("*"), nil
	case blob.ConditionIfMatch:
		return nil, aws.String(cond.ETag)
	default:
		return nil, nil
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}

// encodeTags formats tags as the URL query string S3 expects, with stable ordering.
func encodeTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := url.Values{}
	for _, key := range keys {
		values.Set(key, tags[key])
	}
	return values.Encode()
}

func sizeMismatch(key string, got, declared int64) error {
	return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: body is %d bytes, declared %d", errUtils.ErrBlobInvalid, got, declared))
}
