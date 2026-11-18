package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// putFallback preserves the already-consumed prefix, spooling only after multipart is rejected.
func (s *Store) putFallback(ctx context.Context, plan *putPlan, first, carry []byte, body io.Reader) (*blob.ObjectInfo, error) {
	file, err := os.CreateTemp("", "atmos-blob-fallback-*")
	if err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpPut, plan.key, fmt.Errorf("create upload spool: %w", err))
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	// One byte past the limit is enough to prove the body is too large without spooling all of it.
	source := io.LimitReader(&ctxReader{ctx: ctx, reader: io.MultiReader(bytes.NewReader(first), bytes.NewReader(carry), body)}, s.singlePutLimit()+1)
	size, err := io.Copy(file, source)
	if err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpPut, plan.key, fmt.Errorf("spool upload: %w", err))
	}
	if size > s.singlePutLimit() {
		return nil, tooLargeForSinglePut(plan.key, size, s.singlePutLimit())
	}
	if plan.opts.Size >= 0 && size != plan.opts.Size {
		return nil, sizeMismatch(plan.key, size, plan.opts.Size)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpPut, plan.key, err)
	}
	in := &s3.PutObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(plan.objectKey), Body: file, ContentLength: aws.Int64(size), ContentType: optionalString(plan.opts.ContentType), Metadata: plan.opts.Metadata, Tagging: optionalString(plan.tags)}
	in.IfNoneMatch, in.IfMatch = conditionHeaders(plan.opts.Condition)
	out, err := plan.api.PutObject(ctx, in, retryOptions(plan.opts.Condition.IsConditional())...)
	if err != nil {
		return nil, classify(plan.request(true), err)
	}
	return &blob.ObjectInfo{Key: plan.key, Size: size, ETag: aws.ToString(out.ETag)}, nil
}

// ctxReader fails reads once its context is done, so a canceled operation stops consuming its body.
type ctxReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "s3.ctxReader.Read")()

	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
