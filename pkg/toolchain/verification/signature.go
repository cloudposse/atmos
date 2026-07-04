package verification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

//nolint:gocognit,revive // This mirrors Aqua's independent signature metadata branches.
func (v *Verifier) verifySignatures(ctx context.Context, req *Request, result *Result) error {
	count := len(result.SignatureMethods)
	if hasCosign(&req.Tool.Cosign) {
		if err := v.verifyCosign(ctx, req, &req.Tool.Cosign, result); err != nil {
			return err
		}
		count++
	}
	if hasSLSA(&req.Tool.SLSAProvenance) {
		if err := v.verifySLSA(ctx, req, &req.Tool.SLSAProvenance, result); err != nil {
			return err
		}
		count++
	}
	if hasMinisign(&req.Tool.Minisign) || hasMinisign(&req.Tool.Checksum.Minisign) {
		cfg := &req.Tool.Minisign
		if !hasMinisign(cfg) {
			cfg = &req.Tool.Checksum.Minisign
		}
		if err := v.verifyMinisign(ctx, req, cfg, result); err != nil {
			return err
		}
		count++
	}
	if hasGitHubAttestation(&req.Tool.GitHubArtifactAttestations) || hasGitHubAttestation(&req.Tool.Checksum.GitHubArtifactAttestations) {
		cfg := &req.Tool.GitHubArtifactAttestations
		if !hasGitHubAttestation(cfg) {
			cfg = &req.Tool.Checksum.GitHubArtifactAttestations
		}
		if err := v.verifyGitHubAttestation(ctx, req, cfg, result); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		if req.Policy.Signatures == PolicyRequired {
			return fmt.Errorf("%w: %s/%s@%s has no signature metadata", ErrSignatureRequired, req.Tool.RepoOwner, req.Tool.RepoName, req.Version)
		}
		result.SkippedReasons = append(result.SkippedReasons, "signature metadata unavailable")
	}
	return nil
}

func (v *Verifier) verifyCosign(ctx context.Context, req *Request, cfg *registry.CosignConfig, result *Result) error {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	args, cleanup, err := v.cosignArgs(ctx, req, cfg, result)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return nil
	}
	if cleanup != nil {
		defer cleanup()
	}
	args = append(args, req.AssetPath)
	if err := runCosignWithRetry(ctx, req, args); err != nil {
		return handleSignatureVerificationError("cosign", req, result, err)
	}
	result.SignatureMethods = append(result.SignatureMethods, "cosign")
	return nil
}

// runCosignWithRetry invokes cosign with bounded exponential backoff,
// retrying only on transient Sigstore Rekor failures. All other cosign
// failures (tampering, expired cert, identity mismatch, missing signature)
// surface immediately on the first attempt.
func runCosignWithRetry(ctx context.Context, req *Request, args []string) error {
	attempt := 0
	return retry.WithPredicate(ctx, cosignRetryConfig(), func() error {
		attempt++
		runErr := classifyCosignError(runner(req).Run(ctx, "cosign", args...))
		// Only log the "retrying" warning when a retry will actually happen
		// — suppress on the terminal attempt where the retry budget is
		// exhausted and the error is about to surface unchanged.
		if runErr != nil && isRetryableCosignError(runErr) && attempt < cosignRetryMaxAttempts {
			log.Warn(
				"cosign verification hit transient Sigstore Rekor error; retrying",
				"attempt", attempt,
				"max_attempts", cosignRetryMaxAttempts,
				"error", runErr.Error(),
			)
		}
		return runErr
	}, isRetryableCosignError)
}

func (v *Verifier) cosignArgs(ctx context.Context, req *Request, cfg *registry.CosignConfig, result *Result) ([]string, func(), error) {
	args := []string{"verify-blob"}
	if len(cfg.Opts) > 0 {
		rendered, err := renderArgs(cfg.Opts, req)
		if err != nil {
			return nil, nil, err
		}
		args = append(args, rendered...)
	}
	sidecars, cleanup, err := v.downloadCosignSidecars(ctx, req, cfg)
	if err == nil {
		args = append(args, sidecars...)
		if len(args) == 1 {
			return nil, cleanup, nil
		}
		return args, cleanup, nil
	}
	if req.Policy.Signatures != PolicyRequired {
		result.SkippedReasons = append(result.SkippedReasons, fmt.Sprintf("cosign sidecar unavailable: %v", err))
		return nil, cleanup, nil
	}
	return nil, cleanup, fmt.Errorf("%w: %w", ErrSignatureRequired, err)
}

func (v *Verifier) verifySLSA(ctx context.Context, req *Request, cfg *registry.SLSAProvenance, result *Result) error {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	provenance := registry.DownloadedFile{
		Type: cfg.Type, RepoOwner: cfg.RepoOwner, RepoName: cfg.RepoName, Asset: cfg.Asset, URL: cfg.URL,
	}
	provenanceURL, err := sidecarURL(req.Tool, req.Version, req.AssetURL, &provenance)
	if err != nil {
		return err
	}
	provenanceSidecar, err := v.resolveRemoteSignatureSidecar(ctx, req, provenanceURL, "slsa provenance", result)
	if err != nil || provenanceSidecar.skipped {
		return err
	}
	if provenanceSidecar.cleanup != nil {
		defer provenanceSidecar.cleanup()
	}
	args := []string{"verify-artifact", req.AssetPath, "--provenance-path", provenanceSidecar.path}
	if cfg.SourceURI != "" {
		args = append(args, "--source-uri", cfg.SourceURI)
	}
	if cfg.SourceTag != "" {
		args = append(args, "--source-tag", cfg.SourceTag)
	}
	if err := runner(req).Run(ctx, "slsa-verifier", args...); err != nil {
		return handleSignatureVerificationError("slsa provenance", req, result, err)
	}
	result.SignatureMethods = append(result.SignatureMethods, "slsa_provenance")
	return nil
}

func (v *Verifier) verifyMinisign(ctx context.Context, req *Request, cfg *registry.MinisignConfig, result *Result) error {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	signature := registry.DownloadedFile{
		Type: cfg.Type, RepoOwner: cfg.RepoOwner, RepoName: cfg.RepoName, Asset: cfg.Asset, URL: cfg.URL,
	}
	sigURL, err := sidecarURL(req.Tool, req.Version, req.AssetURL, &signature)
	if err != nil {
		return err
	}
	signatureSidecar, err := v.resolveRemoteSignatureSidecar(ctx, req, sigURL, "minisign signature", result)
	if err != nil || signatureSidecar.skipped {
		return err
	}
	if signatureSidecar.cleanup != nil {
		defer signatureSidecar.cleanup()
	}
	args := []string{"-Vm", req.AssetPath, "-x", signatureSidecar.path}
	if cfg.PublicKey != "" {
		args = append(args, "-P", cfg.PublicKey)
	}
	if err := runner(req).Run(ctx, "minisign", args...); err != nil {
		return handleSignatureVerificationError("minisign signature", req, result, err)
	}
	result.SignatureMethods = append(result.SignatureMethods, "minisign")
	return nil
}

func (v *Verifier) verifyGitHubAttestation(ctx context.Context, req *Request, cfg *registry.GitHubArtifactAttestations, result *Result) error {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	args := []string{"attestation", "verify", req.AssetPath, "--repo", req.Tool.RepoOwner + "/" + req.Tool.RepoName}
	if cfg.SignerWorkflow != "" {
		args = append(args, "--signer-workflow", cfg.SignerWorkflow)
	}
	if cfg.PredicateType != "" {
		args = append(args, "--predicate-type", cfg.PredicateType)
	}
	if err := runner(req).Run(ctx, "gh", args...); err != nil {
		return handleSignatureVerificationError("github artifact attestations", req, result, err)
	}
	result.SignatureMethods = append(result.SignatureMethods, "github_artifact_attestations")
	return nil
}

type signatureSidecarResolution struct {
	path    string
	cleanup func()
	skipped bool
}

func (v *Verifier) resolveRemoteSignatureSidecar(
	ctx context.Context,
	req *Request,
	sidecarURL string,
	method string,
	result *Result,
) (signatureSidecarResolution, error) {
	if !isHTTPURL(sidecarURL) {
		return signatureSidecarResolution{path: sidecarURL}, nil
	}
	path, err := v.downloadTempSidecar(ctx, req, sidecarURL)
	if err != nil {
		if req.Policy.Signatures != PolicyRequired {
			result.SkippedReasons = append(result.SkippedReasons, fmt.Sprintf("%s unavailable: %v", method, err))
			return signatureSidecarResolution{skipped: true}, nil
		}
		return signatureSidecarResolution{}, fmt.Errorf("%w: %w", ErrSignatureRequired, err)
	}
	cleanup := func() {
		// #nosec G703 -- file is a temporary signature sidecar created by this process.
		_ = os.Remove(path)
	}
	return signatureSidecarResolution{path: path, cleanup: cleanup}, nil
}

func handleSignatureVerificationError(method string, req *Request, result *Result, err error) error {
	if err == nil {
		return nil
	}
	if isSignatureEvidenceUnavailable(err) {
		if req.Policy.Signatures != PolicyRequired {
			result.SkippedReasons = append(result.SkippedReasons, fmt.Sprintf("%s unavailable: %v", method, err))
			return nil
		}
		return fmt.Errorf("%w: %w", ErrSignatureRequired, err)
	}
	return err
}

func isSignatureEvidenceUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDownloadFailed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range signatureUnavailableMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

var signatureUnavailableMarkers = []string{
	"404 not found",
	"http 404",
	"status 404",
	"no attestations found",
	"no attestations were found",
	"no matching attestations",
	"could not find any attestations",
	"unable to find any attestations",
	"attestation not found",
	"signature not found",
	"provenance not found",
}

func isHTTPURL(rawURL string) bool {
	return strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
}

func (v *Verifier) downloadCosignSidecars(ctx context.Context, req *Request, cfg *registry.CosignConfig) ([]string, func(), error) {
	var args []string
	var files []string
	add := func(flag string, sidecar *registry.DownloadedFile) error {
		if !hasSidecar(sidecar) {
			return nil
		}
		u, err := sidecarURL(req.Tool, req.Version, req.AssetURL, sidecar)
		if err != nil {
			return err
		}
		path, err := v.downloadTempSidecar(ctx, req, u)
		if err != nil {
			return err
		}
		files = append(files, path)
		args = append(args, flag, path)
		return nil
	}
	if err := add("--signature", &cfg.Signature); err != nil {
		return nil, func() {}, err
	}
	if err := add("--certificate", &cfg.Certificate); err != nil {
		return nil, func() {}, err
	}
	if err := add("--key", &cfg.Key); err != nil {
		return nil, func() {}, err
	}
	if err := add("--bundle", &cfg.Bundle); err != nil {
		return nil, func() {}, err
	}
	cleanup := func() {
		for _, file := range files {
			// #nosec G703 -- file is a temporary sidecar created by this process.
			_ = os.Remove(file)
		}
	}
	return args, cleanup, nil
}

func (v *Verifier) downloadTempSidecar(ctx context.Context, req *Request, url string) (string, error) {
	downloader := req.Downloader
	if downloader == nil {
		downloader = v.Downloader
	}
	if downloader == nil {
		downloader = HTTPDownloader{}
	}
	data, err := downloader.Download(ctx, url)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "atmos-verify-*"+filepath.Ext(url))
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		// #nosec G703 -- file is a temporary sidecar created by this process.
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func runner(req *Request) CommandRunner {
	if req.Runner != nil {
		return req.Runner
	}
	return ExecRunner{}
}

func renderArgs(args []string, req *Request) ([]string, error) {
	rendered := make([]string, len(args))
	effectiveVersion := effectiveReleaseVersionFromAssetURL(req.AssetURL, req.Version)
	for i, arg := range args {
		if strings.Contains(arg, "{{") {
			value, err := renderTemplateString(arg, req.Tool, req.Version, assetNameFromURL(req.AssetURL), nil)
			if err != nil {
				return nil, err
			}
			rendered[i] = replaceVersionSegmentInURL(value, req.Version, effectiveVersion)
		} else {
			rendered[i] = replaceVersionSegmentInURL(arg, req.Version, effectiveVersion)
		}
	}
	return rendered, nil
}

func sidecarURL(tool *registry.Tool, version, assetURL string, sidecar *registry.DownloadedFile) (string, error) {
	assetName := assetNameFromURL(assetURL)
	effectiveVersion := effectiveReleaseVersionFromAssetURL(assetURL, version)
	if sidecar.URL != "" {
		rendered, err := renderTemplateString(sidecar.URL, tool, version, assetName, nil)
		if err != nil {
			return "", err
		}
		return replaceVersionSegmentInURL(rendered, version, effectiveVersion), nil
	}
	sidecarAsset, err := renderTemplateString(sidecar.Asset, tool, version, assetName, nil)
	if err != nil {
		return "", err
	}
	if sidecar.Type == "http" {
		return replaceVersionSegmentInURL(sidecarAsset, version, effectiveVersion), nil
	}
	repoOwner := tool.RepoOwner
	if sidecar.RepoOwner != "" {
		repoOwner = sidecar.RepoOwner
	}
	repoName := tool.RepoName
	if sidecar.RepoName != "" {
		repoName = sidecar.RepoName
	}
	releaseVersion, err := sidecarReleaseVersion(tool, version, assetURL, assetName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, releaseVersion, sidecarAsset), nil
}

func sidecarReleaseVersion(tool *registry.Tool, version, assetURL, assetName string) (string, error) {
	if releaseVersion := effectiveReleaseVersionFromAssetURL(assetURL, version); releaseVersion != "" {
		return releaseVersion, nil
	}
	return renderTemplateString("{{.Version}}", tool, version, assetName, nil)
}

func hasCosign(cfg *registry.CosignConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled != nil || len(cfg.Opts) > 0 || hasSidecar(&cfg.Signature) || hasSidecar(&cfg.Certificate) || hasSidecar(&cfg.Key) || hasSidecar(&cfg.Bundle)
}

func hasSLSA(cfg *registry.SLSAProvenance) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled != nil || cfg.Asset != "" || cfg.URL != "" || cfg.SourceURI != "" || cfg.SourceTag != ""
}

func hasMinisign(cfg *registry.MinisignConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled != nil || cfg.Asset != "" || cfg.URL != "" || cfg.PublicKey != ""
}

func hasGitHubAttestation(cfg *registry.GitHubArtifactAttestations) bool {
	if cfg == nil {
		return false
	}
	return cfg.Enabled != nil || cfg.SignerWorkflow != "" || cfg.PredicateType != ""
}

func hasSidecar(sidecar *registry.DownloadedFile) bool {
	if sidecar == nil {
		return false
	}
	return sidecar.Type != "" || sidecar.Asset != "" || sidecar.URL != ""
}
