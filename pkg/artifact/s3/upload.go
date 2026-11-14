package s3

import (
	"context"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// uploadAll writes the payload objects with bounded concurrency, then the documents and metadata.
// The reservation already made the location exclusively this reference's, so the writes are
// unconditional and idempotent: a retry simply writes the same bytes again.
func (r *Repository) uploadAll(ctx context.Context, job *publishJob, items []objectPlan) error {
	var payload, rest []*objectPlan
	for index := range items {
		if items[index].payload {
			payload = append(payload, &items[index])
		} else {
			rest = append(rest, &items[index])
		}
	}
	var started atomic.Bool
	err := r.forEach(ctx, len(payload), func(ctx context.Context, index int) error {
		if err := r.uploadObject(ctx, payload[index].place(job.location), payload[index]); err != nil {
			return err
		}
		if started.CompareAndSwap(false, true) {
			return r.settings.fault(faultDuringPayload)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return r.forEach(ctx, len(rest), func(ctx context.Context, index int) error {
		return r.uploadObject(ctx, rest[index].place(job.location), rest[index])
	})
}

// uploadObject streams one object, verifying size and digest while it goes. An ambiguous outcome is
// repeated: the write is unconditional and the bytes are the same.
func (r *Repository) uploadObject(ctx context.Context, key string, item *objectPlan) error {
	var err error
	for range putAttempts {
		err = r.uploadOnce(ctx, key, item)
		if !blob.IsAmbiguous(err) {
			break
		}
	}
	return r.mapErr(opWrite, key, err)
}

func (r *Repository) uploadOnce(ctx context.Context, key string, item *objectPlan) error {
	source, err := item.open(ctx)
	if err != nil {
		return err
	}
	defer source.Close()
	_, err = r.store.Put(ctx, key, newVerifyingReader(source, item.digest, item.size, nil), r.putOptions(item.size, item.contentType, blob.Condition{}))
	return err
}
