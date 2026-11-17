package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// ensureConditional makes sure a conditional operation is safe to send through api. In probe mode
// it first proves, once per endpoint, that the endpoint enforces If-None-Match and If-Match; an
// endpoint that silently ignores the header would otherwise turn create-if-absent into an
// unconditional overwrite. The verdict covers the endpoint of api, the client the operation is
// about to use, so a named identity that now resolves to another endpoint is probed again.
func (s *Store) ensureConditional(ctx context.Context, api API, key string, cond blob.Condition) error {
	if !cond.IsConditional() || s.cfg.conditional != ConditionalProbe {
		return nil
	}
	if err := s.probeConditional(ctx, api); err != nil {
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
// object twice: the second create must be rejected. It then checks that stale If-Match validators
// are rejected on PutObject and DeleteObject. The guarantee covers those single-object requests
// only; conditional enforcement on CompleteMultipartUpload is not probed. A definite answer is
// cached for the endpoint it probed; a different endpoint, which a named identity can resolve to
// at any time, is probed again. A transient failure is returned without being cached. It
// returns an error matching ErrBlobConditionalUnsupported when the endpoint ignores or rejects the
// condition.
func (s *Store) ProbeConditionalWrites(ctx context.Context) error {
	defer perf.Track(nil, "blob.s3.ProbeConditionalWrites")()

	api, err := s.client(ctx)
	if err != nil {
		return err
	}
	return s.probeConditional(ctx, api)
}

// probeConditional probes the endpoint of api unless its verdict is already cached. One probe runs
// at a time; a waiting operation gives up when its context ends instead of blocking on the probe
// in flight.
func (s *Store) probeConditional(ctx context.Context, api API) error {
	endpoint := endpointOf(api)
	if verdict, cached := s.cachedProbe(endpoint); cached {
		return verdict
	}
	select {
	case s.probeSlot <- struct{}{}:
		defer func() { <-s.probeSlot }()
	case <-ctx.Done():
		return blob.NewError(blob.KindOther, probeOperation, "", ctx.Err())
	}
	// Another operation may have finished the probe while this one waited for the slot.
	if verdict, cached := s.cachedProbe(endpoint); cached {
		return verdict
	}
	verdict, cleanupErr := s.runProbe(ctx, api)
	if cleanupErr != nil {
		// A leftover probe object is a transient failure: neither verdict is cached, and both
		// errors stay visible to callers.
		return errors.Join(verdict, cleanupErr)
	}
	if verdict == nil || blob.IsConditionalUnsupported(verdict) {
		s.probeMu.Lock()
		s.probed, s.probedEndpoint, s.probeError = true, endpoint, verdict
		s.probeMu.Unlock()
	}
	return verdict
}

// cachedProbe returns the cached probe verdict, when a definite one exists for endpoint.
func (s *Store) cachedProbe(endpoint string) (verdict error, cached bool) {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()
	if !s.probed || s.probedEndpoint != endpoint {
		return nil, false
	}
	return s.probeError, true
}

// endpointOf names the endpoint a client targets: its base endpoint override, or its region for
// the standard AWS endpoint. A client that does not expose its options, such as a test double or
// an injected wrapper, has one fixed endpoint for the life of the store.
func endpointOf(api API) string {
	reporter, ok := api.(interface{ Options() s3.Options })
	if !ok {
		return ""
	}
	options := reporter.Options()
	if options.BaseEndpoint != nil && *options.BaseEndpoint != "" {
		return *options.BaseEndpoint
	}
	return "region:" + options.Region
}

// runProbe performs the double create and removes the probe object. The verdict is the probe
// result; cleanupErr reports a failure to delete the probe object.
func (s *Store) runProbe(ctx context.Context, api API) (verdict, cleanupErr error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return blob.NewError(blob.KindOther, probeOperation, "", err), nil
	}
	key := probePrefix + hex.EncodeToString(raw[:])
	objectKey := s.objectKey(key)
	// Cleanup is scheduled before the first create: a PUT that returns an ambiguous error may
	// still have written the probe object.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeCleanupTimeout)
		defer cancel()
		_, err := api.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.bucket), Key: aws.String(objectKey)}, noRetry)
		if err != nil {
			cleanupErr = blob.NewError(blob.KindOther, probeOperation, key, fmt.Errorf("cleanup probe object: %w", err))
		}
	}()
	return s.probeCreate(ctx, api, key, objectKey), nil
}

// probeCreate creates the probe object twice and checks the second create is rejected.
func (s *Store) probeCreate(ctx context.Context, api API, key, objectKey string) error {
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
