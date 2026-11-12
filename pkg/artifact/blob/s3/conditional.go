package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// probePrefix holds the short-lived objects the conditional-write probe creates.
const probePrefix = ".atmos-blob-probe/"

const probeOperation = "probe"

const probeCleanupTimeout = 5 * time.Second

// ensureConditional makes sure a conditional operation is safe to send. In probe mode it first
// proves, once per store, that the endpoint enforces If-None-Match and If-Match; an endpoint that silently
// ignores the header would otherwise turn create-if-absent into an unconditional overwrite.
func (s *Store) ensureConditional(ctx context.Context, key string, cond blob.Condition) error {
	if !cond.IsConditional() || s.cfg.conditional != ConditionalProbe {
		return nil
	}
	if err := s.ProbeConditionalWrites(ctx); err != nil {
		return unsupportedConditionalFrom(key, err)
	}
	return nil
}

// unsupportedConditionalFrom reports a failed probe as the operation's error.
func unsupportedConditionalFrom(key string, probeErr error) error {
	if blob.IsConditionalUnsupported(probeErr) {
		return blob.NewError(blob.KindUnsupported, blob.OpPut, key, probeErr)
	}
	return probeErr
}

// ProbeConditionalWrites verifies that the endpoint enforces If-None-Match by creating a throwaway
// object twice: the second create must be rejected. A definite answer is cached for the life of the
// store; a transient failure is returned without being cached. It returns an error matching
// ErrBlobConditionalUnsupported when the endpoint ignores or rejects the condition.
func (s *Store) ProbeConditionalWrites(ctx context.Context) error {
	defer perf.Track(nil, "blob.s3.ProbeConditionalWrites")()

	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if s.probed {
		return s.probeError
	}
	api, err := s.client(ctx)
	if err != nil {
		return err
	}
	err = s.runProbe(ctx, api)
	if err == nil || blob.IsConditionalUnsupported(err) {
		s.probed, s.probeError = true, err
	}
	return err
}

// runProbe performs the double create and cleans up the probe object.
func (s *Store) runProbe(ctx context.Context, api API) (resultErr error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return blob.NewError(blob.KindOther, probeOperation, "", err)
	}
	key := probePrefix + hex.EncodeToString(raw[:])
	objectKey := s.objectKey(key)
	create := func() error {
		_, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s.cfg.bucket),
			Key:         aws.String(objectKey),
			Body:        bytes.NewReader([]byte("atmos conditional write probe")),
			IfNoneMatch: aws.String("*"),
		}, noRetry)
		return classify(request{op: probeOperation, key: key, mutating: true, conditional: true}, err)
	}
	if err := create(); err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeCleanupTimeout)
		defer cancel()
		_, err := api.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(objectKey)}, noRetry)
		if err != nil {
			resultErr = blob.NewError(blob.KindOther, probeOperation, key, fmt.Errorf("cleanup probe object: %w", err))
		}
	}()
	err := create()
	if blob.IsPreconditionFailed(err) {
		return s.probeIfMatch(ctx, api, key, objectKey)
	}
	if err == nil {
		return unsupportedConditional(probeOperation, key, "the endpoint accepted a second create-if-absent for the same key, so it ignores If-None-Match")
	}
	return err
}

// probeIfMatch verifies that stale validators cannot overwrite or delete the probe object.
func (s *Store) probeIfMatch(ctx context.Context, api API, key, objectKey string) error {
	stale := aws.String(`"atmos-probe-nonmatching-etag"`)
	_, err := api.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(objectKey), Body: bytes.NewReader([]byte("unexpected")), IfMatch: stale}, noRetry)
	if err = classify(request{op: probeOperation, key: key, mutating: true, conditional: true}, err); !blob.IsPreconditionFailed(err) {
		if err == nil {
			return unsupportedConditional(probeOperation, key, "the endpoint ignored If-Match on put")
		}
		return err
	}
	_, err = api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(objectKey), IfMatch: stale}, noRetry)
	if err = classify(request{op: probeOperation, key: key, mutating: true, conditional: true}, err); !blob.IsPreconditionFailed(err) {
		if err == nil {
			return unsupportedConditional(probeOperation, key, "the endpoint ignored If-Match on delete")
		}
		return err
	}
	return nil
}
