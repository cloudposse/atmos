package oci

import (
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/cloudposse/atmos/pkg/perf"
)

// IsAlreadyExists reports whether a push found the identical content already stored, which
// content addressing makes a success for the writer.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func IsAlreadyExists(err error) bool {
	return errors.Is(err, errdef.ErrAlreadyExists)
}

// IsNotFound distinguishes a missing manifest from auth and transport failures.
// It recognizes errors from both the legacy and the ORAS-backed transport.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func IsNotFound(err error) bool {
	var response *transport.Error
	if errors.As(err, &response) {
		return response.StatusCode == http.StatusNotFound
	}
	var remote *errcode.ErrorResponse
	if errors.As(err, &remote) {
		return remote.StatusCode == http.StatusNotFound
	}
	return errors.Is(err, errdef.ErrNotFound)
}

// IsCatalogUnavailable identifies responses indicating that a registry does not expose a catalog.
func IsCatalogUnavailable(err error) bool {
	defer perf.Track(nil, "oci.IsCatalogUnavailable")()
	if IsNotFound(err) {
		return true
	}
	code := statusOf(err)
	return code == http.StatusMethodNotAllowed || code == http.StatusNotImplemented
}
