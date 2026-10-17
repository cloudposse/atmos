// Package checksum provides primitives for the toolchain lockfile's reproducible-builds story:
// streaming SHA-256 computation, parsing of common checksum file formats, and network helpers
// that fetch either an upstream checksum manifest (Tier 1) or download a single asset and hash
// it on the wire (Tier 2). Higher layers (install, lock) orchestrate the two tiers.
package checksum

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Sentinel errors. Static per CLAUDE.md error handling rules; wrap with `fmt.Errorf("%w: …", err, …)`
// at call sites so callers can use `errors.Is` to detect cause.
var (
	// ErrChecksumUnavailable indicates that no checksum could be obtained for an asset —
	// neither the upstream checksum manifest nor a successful download was available.
	ErrChecksumUnavailable = errors.New("checksum unavailable")

	// ErrEmptyChecksumFile indicates a checksum manifest parsed cleanly but contained no entries.
	ErrEmptyChecksumFile = errors.New("checksum file is empty")

	// ErrAssetNotInChecksumFile indicates the requested asset filename was not present in the
	// checksum manifest (manifest parsed, entry missing).
	ErrAssetNotInChecksumFile = errors.New("asset not in checksum file")

	// ErrChecksumFileTooLarge indicates the checksum manifest exceeded the read cap. Defends against
	// a malicious or misconfigured registry returning multi-gigabyte content.
	ErrChecksumFileTooLarge = errors.New("checksum file too large")

	// ErrHTTPStatus indicates a non-2xx response. The wrapper includes the status code.
	ErrHTTPStatus = errors.New("unexpected HTTP status")
)

// maxChecksumFileSize caps how many bytes we'll read from a checksum manifest URL.
// Real manifests are typically a few kilobytes; 1 MiB is a generous ceiling that still
// rejects pathological inputs.
const maxChecksumFileSize = 1 << 20 // 1 MiB.

// Compute streams r through SHA-256 and returns the lowercase hex hash and total bytes read.
// Reads to EOF. Returns the underlying error if reading r fails — partial reads count in the
// returned byte total even on error so callers can log/diagnose.
func Compute(r io.Reader) (string, int64, error) {
	defer perf.Track(nil, "checksum.Compute")()

	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", n, fmt.Errorf("compute sha256: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// FetchFromChecksumFile retrieves the checksum manifest at url, parses it, and returns the
// SHA-256 entry for assetFilename. Wraps ErrChecksumUnavailable on transport / status failures
// and ErrAssetNotInChecksumFile when the manifest parses but lacks the requested filename.
//
// The caller is responsible for choosing a sensible client (auth headers, timeouts). The
// context governs the HTTP request lifecycle.
func FetchFromChecksumFile(ctx context.Context, client *http.Client, url, assetFilename string) (string, error) {
	defer perf.Track(nil, "checksum.FetchFromChecksumFile")()

	data, err := getCapped(ctx, client, url, maxChecksumFileSize)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrChecksumUnavailable, err)
	}

	entries, err := ParseChecksumsFile(data)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrChecksumUnavailable, err)
	}

	hash, ok := lookupAsset(entries, assetFilename)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrAssetNotInChecksumFile, assetFilename)
	}
	return hash, nil
}

// FetchByDownload streams the asset at assetURL through SHA-256 without persisting the bytes
// to disk. Returns the hex hash and total size. Useful for populating lockfile entries for
// platforms other than the one we're running on — we don't want the artifacts, only their hashes.
func FetchByDownload(ctx context.Context, client *http.Client, assetURL string) (string, int64, error) {
	defer perf.Track(nil, "checksum.FetchByDownload")()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("%w: build request: %w", ErrChecksumUnavailable, err)
	}

	resp, err := client.Do(req) //nolint:gosec // URL is supplied by the caller (toolchain registry config), not by user input on the wire.
	if err != nil {
		return "", 0, fmt.Errorf("%w: %w", ErrChecksumUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("%w: %w %d", ErrChecksumUnavailable, ErrHTTPStatus, resp.StatusCode)
	}

	return Compute(resp.Body)
}

// getCapped issues a GET that aborts (via io.LimitReader) if the body exceeds maxBytes.
// Returns ErrChecksumFileTooLarge when the cap is reached so callers can distinguish
// pathological responses from legitimate transport failures.
func getCapped(ctx context.Context, client *http.Client, url string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req) //nolint:gosec // URL is supplied by the caller (toolchain registry config), not by user input on the wire.
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w %d", ErrHTTPStatus, resp.StatusCode)
	}

	// Read maxBytes+1 so we can detect overruns deterministically.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrChecksumFileTooLarge
	}
	return data, nil
}
