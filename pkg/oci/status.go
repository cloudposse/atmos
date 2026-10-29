package oci

import (
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// IsNotFound distinguishes a missing manifest from auth and transport failures.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func IsNotFound(err error) bool {
	var response *transport.Error
	return errors.As(err, &response) && response.StatusCode == http.StatusNotFound
}
