package cache

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// trustProbeTimeout bounds the pre-flight TLS probe of the proxy.
const trustProbeTimeout = 5 * time.Second

// VerifyTrust checks, before terraform/tofu runs, whether the OS trust store trusts
// the proxy's certificate, so the user gets an actionable message instead of a raw
// x509 error from the subprocess. It only probes on platforms where trust is
// installed in the OS store (macOS/Windows); on Linux/BSD trust comes from the
// SSL_CERT_FILE bundle Atmos sets on the subprocess, so it is a no-op.
func (s *Setup) VerifyTrust(ctx context.Context) error {
	defer perf.Track(nil, "tfcache.Setup.VerifyTrust")()

	if s == nil || s.proxyURL == "" {
		return nil
	}
	if required, _ := TrustInstructions(); !required {
		return nil
	}

	// Probe with the default client so this mirrors the platform verifier the
	// terraform/tofu subprocess uses on macOS/Windows.
	client := &http.Client{Timeout: trustProbeTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.proxyURL, nil)
	if err != nil {
		return nil
	}
	//nolint:gosec // s.proxyURL is our own ephemeral loopback proxy, not user input.
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		return nil
	}
	if !isCertTrustError(err) {
		// A non-certificate error (e.g. transient) — let terraform surface it.
		return nil
	}

	return errUtils.Build(errUtils.ErrCacheCertUntrusted).
		WithExplanationf("terraform/tofu cannot verify the registry cache proxy certificate at %s because it is not in the OS trust store.", s.proxyURL).
		WithHint("Run `atmos terraform cache trust` to trust the cache certificate (a one-time step on macOS/Windows), then re-run your command.").
		Err()
}

// isCertTrustError reports whether err is a TLS certificate verification failure.
func isCertTrustError(err error) bool {
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	var verifyErr *tls.CertificateVerificationError
	return errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &invalidErr) ||
		errors.As(err, &verifyErr)
}
