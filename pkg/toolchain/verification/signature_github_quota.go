package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

const publicAttestationPageSize = 30

var errPublicAttestationPredicateUnavailable = errors.New("public attestation predicate unavailable")

func isGitHubAttestationInstallationQuota(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "http 403") &&
		strings.Contains(message, "api rate limit exceeded for installation") &&
		strings.Contains(message, "https://api.github.com/repos/")
}

// verifyPublicGitHubAttestation retrieves public evidence without spending the
// exhausted installation token's quota. The bundle remains untrusted until gh
// verifies it against the artifact and every original identity/predicate flag.
func verifyPublicGitHubAttestation(ctx context.Context, req *Request, args []string, downloader Downloader) error {
	digest, err := digestFile(req.AssetPath, "sha256")
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/attestations/sha256:%s?per_page=%d",
		url.PathEscape(req.Tool.RepoOwner), url.PathEscape(req.Tool.RepoName), digest, publicAttestationPageSize)
	if downloader == nil {
		// Do not inherit either authenticated sidecar downloader.
		downloader = anonymousAttestationDownloader{}
	}
	response, err := downloader.Download(ctx, endpoint)
	if err != nil {
		return err
	}
	bundle, err := publicAttestationBundles(response)
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "atmos-attestation-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "bundles.jsonl")
	const bundlePermissions = 0o600
	if err := os.WriteFile(path, bundle, bundlePermissions); err != nil {
		return err
	}
	// Keep the original repo, signer workflow and predicate flags unchanged.
	verificationArgs := append(append([]string(nil), args...), "--bundle", path)
	err = runGitHubAttestationWithRetry(ctx, req, verificationArgs)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no attestations found with predicate type:") {
		return fmt.Errorf("%w: %w", errPublicAttestationPredicateUnavailable, err)
	}
	return err
}

func publicAttestationBundles(response []byte) ([]byte, error) {
	var payload struct {
		Attestations *[]struct {
			Bundle json.RawMessage `json:"bundle"`
		} `json:"attestations"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		return nil, fmt.Errorf("%w: invalid attestation response: %w", ErrSignatureFailed, err)
	}
	if payload.Attestations == nil {
		return nil, fmt.Errorf("%w: public response must contain an attestations array", ErrSignatureFailed)
	}
	attestations := *payload.Attestations
	// A full page might omit a matching predicate on a later page. Without a
	// complete response we cannot apply the optional-evidence availability policy.
	if len(attestations) >= publicAttestationPageSize {
		return nil, fmt.Errorf("%w: public attestation response may be truncated", ErrSignatureFailed)
	}
	if len(attestations) == 0 {
		return nil, fmt.Errorf("%w: no attestations found in public response", errPublicAttestationPredicateUnavailable)
	}
	var bundles bytes.Buffer
	for _, attestation := range attestations {
		if !bytes.HasPrefix(bytes.TrimSpace(attestation.Bundle), []byte("{")) {
			return nil, fmt.Errorf("%w: attestation bundle must be an object", ErrSignatureFailed)
		}
		// Compact each bundle to one JSONL record. gh validates its signature and schema.
		if err := json.Compact(&bundles, attestation.Bundle); err != nil {
			return nil, fmt.Errorf("%w: invalid attestation bundle: %w", ErrSignatureFailed, err)
		}
		bundles.WriteByte('\n')
	}
	return bundles.Bytes(), nil
}

// anonymousAttestationDownloader is deliberately separate from authenticated
// release downloads. No credential headers or redirect requests are sent.
type anonymousAttestationDownloader struct{ transport http.RoundTripper }

func (d anonymousAttestationDownloader) Download(ctx context.Context, endpoint string) ([]byte, error) {
	defer perf.Track(nil, "verification.anonymousAttestationDownloader.Download")()
	const timeout = 30 * time.Second
	const maxResponseBytes = 8 << 20
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if request.URL.Scheme != "https" || request.URL.Host != "api.github.com" || request.URL.User != nil {
		return nil, fmt.Errorf("%w: invalid public attestation endpoint", ErrSignatureFailed)
	}
	client := &http.Client{Timeout: timeout, Transport: d.transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: public attestation API HTTP %d", ErrSignatureFailed, response.StatusCode)
	}
	if attestationResponseIsPaginated(response.Header) {
		return nil, fmt.Errorf("%w: public attestation response is paginated", ErrSignatureFailed)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("%w: public attestation response exceeds size limit", ErrSignatureFailed)
	}
	return body, nil
}

// GitHub also sends non-pagination Link headers (for example deprecation).
// Reject pagination without following more requests or claiming absence.
func attestationResponseIsPaginated(headers http.Header) bool {
	links := strings.Join(headers.Values("Link"), ",")
	return strings.Contains(links, `rel="next"`) || strings.Contains(links, `rel="last"`)
}
