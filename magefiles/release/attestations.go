package release

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

const releaseSourcePredicateType = "https://atmos.tools/release/v1"

// VerifyAttestations verifies cryptographic signatures and expected signer, then
// checks the authenticated predicate's actual build SHA (not the tooling SHA).
func (Release) VerifyAttestations(dir, repo, tag, commit, workflowSHA string) error {
	data, err := os.ReadFile(filepath.Join(dir, "release-manifest.json"))
	if err != nil {
		return err
	}
	var manifest ArtifactManifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Tag != tag || manifest.Commit != commit {
		return fmt.Errorf("%w: attestation manifest identity mismatch", ErrReleaseVerification)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	verifier := attestationVerifier{Directory: dir, Repository: repo, WorkflowSHA: workflowSHA, Run: runReleaseTool}
	return verifier.verify(ctx, &manifest)
}

// The signer identifies trusted workflow code; the authenticated predicate must
// independently identify the source tag and commit whose bytes were built.
type attestationVerifier struct {
	Directory   string
	Repository  string
	WorkflowSHA string
	Run         releaseRunner
}

func (v *attestationVerifier) verify(ctx context.Context, manifest *ArtifactManifest) error {
	if len(manifest.Assets) == 0 || !githubapi.RepoPattern.MatchString(v.Repository) || !githubapi.SHAPattern.MatchString(v.WorkflowSHA) {
		return fmt.Errorf("%w: invalid attestation inputs", ErrReleaseVerification)
	}
	// Every uploaded build artifact must have provenance from the build job.
	// The verification manifest is created later on this trusted runner, so it
	// is not a build subject; it travels with the sealed immutable artifact set.
	for name := range manifest.Assets {
		if err := v.verifySubject(ctx, name, manifest); err != nil {
			return err
		}
	}
	checksum := "atmos_" + strings.TrimPrefix(manifest.Tag, "v") + "_SHA256SUMS"
	args := append(v.arguments(checksum), "--predicate-type", "https://spdx.dev/Document/v2.3", "--deny-self-hosted-runners")
	_, err := v.Run(ctx, nil, "gh", args...)
	return err
}

func (v *attestationVerifier) arguments(subject string) []string {
	return []string{
		"attestation", "verify", filepath.Join(v.Directory, subject),
		"--repo", v.Repository,
		"--signer-workflow", v.Repository + "/.github/workflows/release-pipeline.yml",
		"--signer-digest", v.WorkflowSHA,
	}
}

func (v *attestationVerifier) verifySubject(ctx context.Context, name string, manifest *ArtifactManifest) error {
	if !validAssetName(name) {
		return fmt.Errorf("%w: invalid attestation subject", ErrReleaseVerification)
	}
	// Restore the standard GitHub-generated SLSA provenance used before the
	// release refactor. Its source is the workflow source, not necessarily the
	// tag selected for this build, so require a separate authenticated binding.
	args := append(v.arguments(name), "--predicate-type", "https://slsa.dev/provenance/v1", "--deny-self-hosted-runners")
	if _, err := v.Run(ctx, nil, "gh", args...); err != nil {
		return err
	}
	args = append(v.arguments(name), "--predicate-type", releaseSourcePredicateType, "--deny-self-hosted-runners", "--format", "json")
	output, err := v.Run(ctx, nil, "gh", args...)
	if err != nil {
		return err
	}
	return checkAttestedSource(output, v.Repository, manifest.Tag, manifest.Commit)
}

type attestationDependency struct {
	URI    string `json:"uri"`
	Digest struct {
		Commit string `json:"gitCommit"`
	} `json:"digest"`
}

type attestationBuild struct {
	Type     string `json:"buildType"`
	External struct {
		Tag    string `json:"tag"`
		Commit string `json:"commit"`
	} `json:"externalParameters"`
	Dependencies []attestationDependency `json:"resolvedDependencies"`
}

type attestationStatement struct {
	PredicateType string `json:"predicateType"`
	Predicate     struct {
		Build attestationBuild `json:"buildDefinition"`
	} `json:"predicate"`
}

type verifiedAttestation struct {
	Verification struct {
		Statement attestationStatement `json:"statement"`
	} `json:"verificationResult"`
}

func checkAttestedSource(output []byte, repo, tag, commit string) error {
	var results []verifiedAttestation
	if err := json.Unmarshal(output, &results); err != nil {
		return err
	}
	for _, result := range results {
		if result.Verification.Statement.bindsSource(repo, tag, commit) {
			return nil
		}
	}
	return fmt.Errorf("%w: no verified provenance binds the selected tag and source SHA", ErrReleaseVerification)
}

func (statement *attestationStatement) bindsSource(repo, tag, commit string) bool {
	build := statement.Predicate.Build
	if statement.PredicateType != releaseSourcePredicateType || build.Type != releaseSourcePredicateType {
		return false
	}
	if build.External.Tag != tag || build.External.Commit != commit {
		return false
	}
	expectedURI := "git+https://github.com/" + repo + "@refs/tags/" + tag
	for _, dependency := range build.Dependencies {
		if dependency.URI == expectedURI && dependency.Digest.Commit == commit {
			return true
		}
	}
	return false
}
