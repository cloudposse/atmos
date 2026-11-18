// Package signaturetest keeps the historical import path for the isolated Sigstore test authority.
// The implementation lives in pkg/signature/signaturetest.
package signaturetest

import (
	"testing"

	shared "github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

// Authority owns isolated CA, CT and Rekor keys and exposes only public trust material.
type Authority = shared.Authority

// New creates a fresh trust domain for one test.
//
//nolint:lintroller // Test helper delegating to the tracked shared implementation.
func New(t *testing.T) *Authority {
	t.Helper()
	return shared.New(t)
}
