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
	size, err := io.Copy(file, &fallbackReader{ctx: ctx, reader: io.MultiReader(bytes.NewReader(first), bytes.NewReader(carry), body)})
	if err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpPut, plan.key, fmt.Errorf("spool upload: %w", err))
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

type fallbackReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *fallbackReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "s3.fallbackReader.Read")()

	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
