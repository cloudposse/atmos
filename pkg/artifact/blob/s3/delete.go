package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [blob.Store]. Conditional deletes are never retried by the SDK.
func (s *Store) Delete(ctx context.Context, key string, opts blob.DeleteOptions) error {
	defer perf.Track(nil, "blob.s3.Delete")()

	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.Capabilities().CheckDelete(storeName, key, opts); err != nil {
		return err
	}
	conditional := opts.IfMatch != ""
	if conditional {
		if err := s.ensureConditional(ctx, key, blob.IfMatch(opts.IfMatch)); err != nil {
			return err
		}
	}
	api, err := s.client(ctx)
	if err != nil {
		return err
	}
	in := &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(s.objectKey(key)), IfMatch: optionalString(opts.IfMatch)}
	if _, err := api.DeleteObject(ctx, in, retryOptions(conditional)...); err != nil {
		return classify(request{op: blob.OpDelete, key: key, mutating: true, conditional: conditional}, err)
	}
	return nil
}
