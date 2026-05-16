package verification

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

//nolint:cyclop,gocognit,revive // This mirrors Aqua's independent signature metadata branches.
func (v *Verifier) verifySignatures(ctx context.Context, req *Request, result *Result) error {
	count := 0
	if hasCosign(&req.Tool.Cosign) {
		if err := v.verifyCosign(ctx, req, &req.Tool.Cosign, result); err != nil {
			return err
		}
		count++
	}
	if hasCosign(&req.Tool.Checksum.Cosign) {
		if err := v.verifyCosign(ctx, req, &req.Tool.Checksum.Cosign, result); err != nil {
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
	args := []string{"verify-blob"}
	if len(cfg.Opts) > 0 {
		rendered, err := renderArgs(cfg.Opts, req)
		if err != nil {
			return err
		}
		args = append(args, rendered...)
	} else {
		sidecars, cleanup, err := v.downloadCosignSidecars(ctx, req, cfg)
		if err != nil {
			return err
		}
		defer cleanup()
		args = append(args, sidecars...)
	}
	args = append(args, req.AssetPath)
	if err := runner(req).Run(ctx, "cosign", args...); err != nil {
		return err
	}
	result.SignatureMethods = append(result.SignatureMethods, "cosign")
	return nil
}

func (v *Verifier) verifySLSA(ctx context.Context, req *Request, cfg *registry.SLSAProvenance, result *Result) error {
	if cfg.Enabled != nil && !*cfg.Enabled {
		return nil
	}
	provenance := registry.DownloadedFile{
		Type: cfg.Type, RepoOwner: cfg.RepoOwner, RepoName: cfg.RepoName, Asset: cfg.Asset, URL: cfg.URL,
	}
	provenanceURL, err := sidecarURL(req.Tool, req.Version, req.AssetURL, &provenance, nil)
	if err != nil {
		return err
	}
	args := []string{"verify-artifact", req.AssetPath, "--provenance-path", provenanceURL}
	if cfg.SourceURI != "" {
		args = append(args, "--source-uri", cfg.SourceURI)
	}
	if cfg.SourceTag != "" {
		args = append(args, "--source-tag", cfg.SourceTag)
	}
	if err := runner(req).Run(ctx, "slsa-verifier", args...); err != nil {
		return err
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
	sigURL, err := sidecarURL(req.Tool, req.Version, req.AssetURL, &signature, nil)
	if err != nil {
		return err
	}
	args := []string{"-Vm", req.AssetPath, "-x", sigURL}
	if cfg.PublicKey != "" {
		args = append(args, "-P", cfg.PublicKey)
	}
	if err := runner(req).Run(ctx, "minisign", args...); err != nil {
		return err
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
		return err
	}
	result.SignatureMethods = append(result.SignatureMethods, "github_artifact_attestations")
	return nil
}

func (v *Verifier) downloadCosignSidecars(ctx context.Context, req *Request, cfg *registry.CosignConfig) ([]string, func(), error) {
	var args []string
	var files []string
	add := func(flag string, sidecar *registry.DownloadedFile) error {
		if !hasSidecar(sidecar) {
			return nil
		}
		u, err := sidecarURL(req.Tool, req.Version, req.AssetURL, sidecar, nil)
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
	for i, arg := range args {
		if strings.Contains(arg, "{{") {
			value, err := renderTemplateString(arg, req.Tool, req.Version, assetNameFromURL(req.AssetURL), nil)
			if err != nil {
				return nil, err
			}
			rendered[i] = value
		} else {
			rendered[i] = arg
		}
	}
	return rendered, nil
}

func sidecarURL(tool *registry.Tool, version, assetURL string, sidecar *registry.DownloadedFile, replacements map[string]string) (string, error) {
	assetName := assetNameFromURL(assetURL)
	if sidecar.URL != "" {
		return renderTemplateString(sidecar.URL, tool, version, assetName, replacements)
	}
	sidecarAsset, err := renderTemplateString(sidecar.Asset, tool, version, assetName, replacements)
	if err != nil {
		return "", err
	}
	if sidecar.Type == "http" {
		return sidecarAsset, nil
	}
	repoOwner := tool.RepoOwner
	if sidecar.RepoOwner != "" {
		repoOwner = sidecar.RepoOwner
	}
	repoName := tool.RepoName
	if sidecar.RepoName != "" {
		repoName = sidecar.RepoName
	}
	releaseVersion, err := renderTemplateString("{{.Version}}", tool, version, assetName, replacements)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", repoOwner, repoName, releaseVersion, sidecarAsset), nil
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
