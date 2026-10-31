package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// abortTimeout bounds the cleanup of an abandoned multipart upload, which must outlive the
// canceled operation context.
const abortTimeout = 30 * time.Second

// multipart is one in-progress multipart upload.
type multipart struct {
	store    *Store
	plan     *putPlan
	uploadID string
}

// putMultipart streams the body as a multipart upload with bounded memory and concurrency. The
// upload is aborted on any definite failure, including cancellation. After an ambiguous
// completion it is left alone, because it may already have become the object.
func (s *Store) putMultipart(ctx context.Context, plan *putPlan, first, carry []byte, body io.Reader) (*blob.ObjectInfo, error) {
	in := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(s.cfg.bucket),
		Key:         aws.String(plan.objectKey),
		ContentType: optionalString(plan.opts.ContentType),
		Metadata:    plan.opts.Metadata,
		Tagging:     optionalString(plan.tags),
	}
	created, err := plan.api.CreateMultipartUpload(ctx, in, skipOptionalChecksums)
	if err != nil {
		return nil, classify(plan.request(false), err)
	}
	upload := &multipart{store: s, plan: plan, uploadID: aws.ToString(created.UploadId)}

	parts, total, err := upload.uploadParts(ctx, first, carry, body)
	if err == nil && plan.opts.Size >= 0 && total != plan.opts.Size {
		err = sizeMismatch(plan.key, total, plan.opts.Size)
	}
	if err != nil {
		return nil, upload.abort(ctx, asBlobError("put", plan.key, err))
	}
	return upload.complete(ctx, parts, total)
}

// partResult records an uploaded part.
type partResult struct {
	mu    sync.Mutex
	parts []types.CompletedPart
}

func (r *partResult) add(number int32, etag *string) {
	r.mu.Lock()
	r.parts = append(r.parts, types.CompletedPart{ETag: etag, PartNumber: aws.Int32(number)})
	r.mu.Unlock()
}

// uploadParts reads the body part by part and uploads them through a bounded pool. A free-list of
// buffers caps memory at one buffer per worker.
func (m *multipart) uploadParts(ctx context.Context, first, carry []byte, body io.Reader) ([]types.CompletedPart, int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, gctx := errgroup.WithContext(ctx)

	free := make(chan []byte, m.store.cfg.concurrency)
	for range m.store.cfg.concurrency {
		free <- nil
	}
	result := &partResult{}
	submit := func(number int32, data []byte) {
		group.Go(func() error {
			defer func() { free <- data }()
			etag, err := m.uploadPart(gctx, number, data)
			if err != nil {
				return err
			}
			result.add(number, etag)
			return nil
		})
	}

	feed := &partFeed{free: free, submit: submit}
	total, produceErr := m.produce(gctx, feed, first, carry, body)
	if produceErr != nil {
		cancel()
	}
	waitErr := group.Wait()
	if produceErr != nil {
		return nil, 0, produceErr
	}
	if waitErr != nil {
		return nil, 0, waitErr
	}
	sort.Slice(result.parts, func(i, j int) bool {
		return aws.ToInt32(result.parts[i].PartNumber) < aws.ToInt32(result.parts[j].PartNumber)
	})
	return result.parts, total, nil
}

// partFeed hands parts to the worker pool. Its free list holds one buffer slot per worker.
type partFeed struct {
	free   chan []byte
	submit func(number int32, data []byte)
}

// produce feeds parts to the workers, waiting for a free buffer before reading each next part so
// the reader never runs ahead of the uploaders.
func (m *multipart) produce(ctx context.Context, feed *partFeed, first, carry []byte, body io.Reader) (int64, error) {
	partSize := m.plan.partSize
	free, submit := feed.free, feed.submit
	if err := acquire(ctx, free); err != nil {
		return 0, err
	}
	number := int32(1)
	data := first
	total := int64(0)
	for {
		submit(number, data)
		total += int64(len(data))
		if int64(len(data)) < partSize {
			return total, nil
		}
		buf, err := acquireBuffer(ctx, free, partSize)
		if err != nil {
			return 0, err
		}
		n, readErr := readPart(body, buf, carry)
		carry = nil
		if readErr != nil {
			return 0, blob.NewError(blob.KindOther, blob.OpPut, m.plan.key, fmt.Errorf("read body: %w", readErr))
		}
		if n == 0 {
			free <- buf
			return total, nil
		}
		number++
		if number > MaxParts {
			return 0, blob.NewError(blob.KindInvalid, blob.OpPut, m.plan.key,
				fmt.Errorf("%w: upload exceeds %d parts; use a larger part size", errUtils.ErrBlobInvalid, MaxParts))
		}
		data = buf[:n]
	}
}

// acquire waits for a worker slot and discards its buffer.
func acquire(ctx context.Context, free chan []byte) error {
	select {
	case <-free:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// acquireBuffer waits for a worker slot and returns its buffer, sized to a part.
func acquireBuffer(ctx context.Context, free chan []byte, partSize int64) ([]byte, error) {
	select {
	case buf := <-free:
		if int64(cap(buf)) < partSize {
			buf = make([]byte, partSize)
		}
		return buf[:partSize], nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readPart fills buf from body, starting with a carried byte when one was already consumed. It
// returns the bytes in buf; zero means the body ended exactly on the previous boundary.
func readPart(body io.Reader, buf, carry []byte) (int, error) {
	offset := copy(buf, carry)
	n, err := io.ReadFull(body, buf[offset:])
	switch {
	case err == nil:
		return offset + n, nil
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return offset + n, nil
	default:
		return 0, err
	}
}

// uploadPart sends one part and returns its ETag.
func (m *multipart) uploadPart(ctx context.Context, number int32, data []byte) (*string, error) {
	out, err := m.plan.api.UploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(m.store.cfg.bucket),
		Key:           aws.String(m.plan.objectKey),
		UploadId:      aws.String(m.uploadID),
		PartNumber:    aws.Int32(number),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	}, skipOptionalChecksums)
	if err != nil {
		return nil, classify(m.plan.request(false), err)
	}
	return out.ETag, nil
}

// complete assembles the parts into the object under the write condition.
func (m *multipart) complete(ctx context.Context, parts []types.CompletedPart, total int64) (*blob.ObjectInfo, error) {
	cond := m.plan.opts.Condition
	in := &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(m.store.cfg.bucket),
		Key:             aws.String(m.plan.objectKey),
		UploadId:        aws.String(m.uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	}
	in.IfNoneMatch, in.IfMatch = conditionHeaders(cond)
	out, err := m.plan.api.CompleteMultipartUpload(ctx, in, append(retryOptions(cond.IsConditional()), skipOptionalChecksums)...)
	if err != nil {
		classified := classify(m.plan.request(true), err)
		if blob.IsAmbiguous(classified) {
			return nil, classified
		}
		return nil, m.abort(ctx, classified)
	}
	return &blob.ObjectInfo{Key: m.plan.key, Size: total, ETag: aws.ToString(out.ETag)}, nil
}

// abort removes the upload's stored parts. It uses a context detached from cancellation so a
// canceled operation still cleans up, and reports a failed cleanup alongside the original error.
func (m *multipart) abort(ctx context.Context, cause error) error {
	abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), abortTimeout)
	defer cancel()
	_, err := m.plan.api.AbortMultipartUpload(abortCtx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(m.store.cfg.bucket),
		Key:      aws.String(m.plan.objectKey),
		UploadId: aws.String(m.uploadID),
	})
	if err != nil {
		return errors.Join(cause, blob.NewError(blob.KindOther, "abort", m.plan.key, err))
	}
	return cause
}

// asBlobError classifies err as a plain failure unless it already is a blob error.
func asBlobError(op, key string, err error) error {
	var classified *blob.Error
	if errors.As(err, &classified) {
		return err
	}
	return blob.NewError(blob.KindOther, op, key, err)
}
