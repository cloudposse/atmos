package oci

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	errUtils "github.com/cloudposse/atmos/errors"
)

// wrapRegistry marks a failure as an OCI registry error while keeping the cause chain.
// HTTP failures also carry a legacy *transport.Error with the real status code so the
// existing status classifiers (IsNotFound, discovery availability, pull diagnostics)
// keep working for callers that still reason about go-containerregistry errors.
func wrapRegistry(err error) error {
	if err == nil {
		return nil
	}
	if legacy := legacyStatusError(err); legacy != nil && !errors.As(err, new(*transport.Error)) {
		return errors.Join(errUtils.ErrOCIRegistry, err, legacy)
	}
	return errors.Join(errUtils.ErrOCIRegistry, err)
}

func wrapIntegrity(err error) error {
	return errors.Join(errUtils.ErrOCIIntegrity, err)
}

// legacyStatusError synthesizes the status carrier from an ORAS error.
func legacyStatusError(err error) *transport.Error {
	var response *errcode.ErrorResponse
	if errors.As(err, &response) {
		return &transport.Error{StatusCode: response.StatusCode}
	}
	if errors.Is(err, errdef.ErrNotFound) {
		return &transport.Error{StatusCode: http.StatusNotFound}
	}
	return nil
}

// statusOf returns the HTTP status behind an error, or zero when none was received.
func statusOf(err error) int {
	var response *errcode.ErrorResponse
	if errors.As(err, &response) {
		return response.StatusCode
	}
	var legacy *transport.Error
	if errors.As(err, &legacy) {
		return legacy.StatusCode
	}
	if errors.Is(err, errdef.ErrNotFound) {
		return http.StatusNotFound
	}
	return 0
}

// definiteRejection reports whether the destination clearly refused a write. Authentication,
// authorization, validation and conflict responses are final answers; everything else
// (timeouts, resets, 5xx) leaves the write outcome unknown.
func definiteRejection(err error) bool {
	status := statusOf(err)
	return status >= http.StatusBadRequest && status < http.StatusInternalServerError &&
		status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
}

// ambiguousWrite reports whether a failed mutation may nevertheless have taken effect.
func ambiguousWrite(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	return !definiteRejection(err)
}
