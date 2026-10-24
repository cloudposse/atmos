package release

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	attestationRepository  = "cloudposse/atmos"
	attestationSourceSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	attestationWorkflowSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func provenanceStatement() map[string]any {
	return map[string]any{"predicateType": releaseSourcePredicateType, "predicate": map[string]any{"buildDefinition": map[string]any{
		"buildType": "https://atmos.tools/release/v1", "externalParameters": map[string]string{"tag": "v1.234.0", "commit": attestationSourceSHA},
		"resolvedDependencies": []any{map[string]any{"uri": "git+https://github.com/cloudposse/atmos@refs/tags/v1.234.0", "digest": map[string]string{"gitCommit": attestationSourceSHA}}},
	}}}
}

func verifiedProvenance(t *testing.T, statements ...map[string]any) []byte {
	t.Helper()
	results := make([]map[string]any, len(statements))
	for i, s := range statements {
		results[i] = map[string]any{"verificationResult": map[string]any{"statement": s}}
	}
	data, err := json.Marshal(results)
	require.NoError(t, err)
	return data
}

func TestReleaseSourceProducerMatchesVerifier(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required to exercise the release action's predicate producer")
	}
	// Use the actual JavaScript producer, with different workflow/source SHAs.
	// The wrapper models gh's authenticated output, not raw unsigned JSON.
	cmd := exec.Command("node", "-e", `
const { provenance } = require('../../.github/actions/release-publish/publish.cjs');
const predicate = provenance({repo: {owner: 'cloudposse', repo: 'atmos'}, runId: 42},
  {tag: 'v1.234.0', commit: process.argv[1]}, process.argv[2]);
process.stdout.write(JSON.stringify(predicate));
`, attestationSourceSHA, attestationWorkflowSHA)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	var predicate map[string]any
	require.NoError(t, json.Unmarshal(output, &predicate))
	statement := map[string]any{"predicateType": releaseSourcePredicateType, "predicate": predicate}
	require.NoError(t, checkAttestedSource(verifiedProvenance(t, statement), attestationRepository, "v1.234.0", attestationSourceSHA))
	require.ErrorIs(t, checkAttestedSource(verifiedProvenance(t, statement), attestationRepository, "v1.234.0", attestationWorkflowSHA), ErrReleaseVerification)
}

func TestAttestedSourceRequiresAuthenticatedReleaseIdentity(t *testing.T) {
	for _, mode := range []string{"valid", "wrong predicate", "legacy SLSA predicate", "wrong build type", "wrong tag", "tooling instead of source", "missing dependencies", "wrong repository", "wrong ref", "wrong dependency commit", "no results", "unverified statement", "malformed JSON"} {
		t.Run(mode, func(t *testing.T) {
			s := provenanceStatement()
			build := s["predicate"].(map[string]any)["buildDefinition"].(map[string]any)
			switch mode {
			case "wrong predicate":
				s["predicateType"] = "https://spdx.dev/Document/v2.3"
			case "legacy SLSA predicate":
				s["predicateType"] = "https://slsa.dev/provenance/v1"
			case "wrong build type":
				build["buildType"] = "https://other.example/build"
			case "wrong tag":
				build["externalParameters"].(map[string]string)["tag"] = "v1.235.0"
			case "tooling instead of source":
				build["externalParameters"].(map[string]string)["commit"] = attestationWorkflowSHA
			case "missing dependencies":
				build["resolvedDependencies"] = []any{}
			case "wrong repository":
				build["resolvedDependencies"].([]any)[0].(map[string]any)["uri"] = "git+https://github.com/other/atmos@refs/tags/v1.234.0"
			case "wrong ref":
				build["resolvedDependencies"].([]any)[0].(map[string]any)["uri"] = "git+https://github.com/cloudposse/atmos@refs/heads/main"
			case "wrong dependency commit":
				build["resolvedDependencies"].([]any)[0].(map[string]any)["digest"].(map[string]string)["gitCommit"] = attestationWorkflowSHA
			}
			output := verifiedProvenance(t, s)
			switch mode {
			case "no results":
				output = []byte("[]")
			case "unverified statement":
				var err error
				output, err = json.Marshal([]map[string]any{{"statement": s}})
				require.NoError(t, err)
			case "malformed JSON":
				output = []byte("{")
			}
			err := checkAttestedSource(output, attestationRepository, "v1.234.0", attestationSourceSHA)
			if mode == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if mode != "malformed JSON" {
					require.ErrorIs(t, err, ErrReleaseVerification)
				}
			}
		})
	}
	valid, irrelevant := provenanceStatement(), provenanceStatement()
	irrelevant["predicateType"] = "https://spdx.dev/Document/v2.3"
	require.NoError(t, checkAttestedSource(verifiedProvenance(t, irrelevant, valid), attestationRepository, "v1.234.0", attestationSourceSHA), "a matching authenticated predicate may coexist with other attestation types")
}

func TestVerifyAttestationsPinsSignerAndActualSourceForEveryAsset(t *testing.T) {
	dir := t.TempDir()
	assets := map[string]string{
		"atmos_1.234.0_linux_amd64":              "binary digest",
		"atmos_1.234.0_SHA256SUMS":               "checksum digest",
		"atmos_1.234.0_SHA256SUMS.sigstore.json": "signature digest",
		"atmos-sbom.spdx.json":                   "source SBOM digest",
		"metadata.json":                          "metadata digest",
	}
	manifest := ArtifactManifest{Tag: "v1.234.0", Commit: attestationSourceSHA, Assets: assets}
	verified := map[string]int{}
	sourceVerified := map[string]int{}
	sbomCalls := 0
	verifier := attestationVerifier{Directory: dir, Repository: attestationRepository, WorkflowSHA: attestationWorkflowSHA, Run: func(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
		require.NoError(t, ctx.Err())
		require.Nil(t, input)
		require.Equal(t, "gh", name)
		require.GreaterOrEqual(t, len(args), 3)
		base := filepath.Base(args[2])
		require.Equal(t, dir, filepath.Dir(args[2]))
		expected := []string{"attestation", "verify", args[2], "--repo", attestationRepository, "--signer-workflow", attestationRepository + "/.github/workflows/release-pipeline.yml", "--signer-digest", attestationWorkflowSHA}
		require.Equal(t, "--predicate-type", args[9])
		switch args[10] {
		case "https://spdx.dev/Document/v2.3":
			require.Equal(t, append(expected, "--predicate-type", "https://spdx.dev/Document/v2.3", "--deny-self-hosted-runners"), args)
			require.Equal(t, "atmos_1.234.0_SHA256SUMS", base)
			sbomCalls++
			return nil, nil
		case "https://slsa.dev/provenance/v1":
			require.Equal(t, append(expected, "--predicate-type", "https://slsa.dev/provenance/v1", "--deny-self-hosted-runners"), args)
			verified[base]++
			return nil, nil
		default:
			require.Equal(t, append(expected, "--predicate-type", releaseSourcePredicateType, "--deny-self-hosted-runners", "--format", "json"), args)
			sourceVerified[base]++
			return verifiedProvenance(t, provenanceStatement()), nil
		}
	}}
	err := verifier.verify(context.Background(), &manifest)
	require.NotContains(t, assets, "release-manifest.json", "verification must not mutate the sealed inventory")
	require.NoError(t, err)
	require.Equal(t, map[string]int{
		"atmos_1.234.0_linux_amd64":              1,
		"atmos_1.234.0_SHA256SUMS":               1,
		"atmos_1.234.0_SHA256SUMS.sigstore.json": 1,
		"atmos-sbom.spdx.json":                   1,
		"metadata.json":                          1,
	}, verified, "all build assets, including the source SBOM created after checksums, require their own provenance")
	require.Equal(t, verified, sourceVerified, "every subject needs both standard provenance and the selected-source attestation")
	require.NotContains(t, verified, "release-manifest.json", "the verification manifest did not exist in the build job")
	require.Equal(t, 1, sbomCalls)
}

func TestVerifyAttestationsRejectsCryptoAndSourceFailures(t *testing.T) {
	toolErr := errors.New("cryptographic verification failed")
	for _, mode := range []string{"unsafe asset", "signature failure", "source signature failure", "source mismatch", "SBOM signature failure"} {
		t.Run(mode, func(t *testing.T) {
			assets := map[string]string{"atmos_1.234.0_SHA256SUMS": "digest"}
			if mode == "unsafe asset" {
				assets = map[string]string{"../secret": "digest"}
			}
			manifest := ArtifactManifest{Tag: "v1.234.0", Commit: attestationSourceSHA, Assets: assets}
			sbomCalls := 0
			verifier := attestationVerifier{Directory: t.TempDir(), Repository: attestationRepository, WorkflowSHA: attestationWorkflowSHA, Run: func(_ context.Context, _ []byte, name string, args ...string) ([]byte, error) {
				require.Equal(t, "gh", name)
				require.NotContains(t, args, "../secret")
				switch args[10] {
				case "https://slsa.dev/provenance/v1":
					if mode == "signature failure" {
						return nil, toolErr
					}
					return nil, nil
				case "https://spdx.dev/Document/v2.3":
					sbomCalls++
					return nil, toolErr
				case releaseSourcePredicateType:
					if mode == "source signature failure" {
						return nil, toolErr
					}
				default:
					t.Fatalf("unexpected predicate type: %s", args[10])
				}
				s := provenanceStatement()
				if mode == "source mismatch" {
					s["predicate"].(map[string]any)["buildDefinition"].(map[string]any)["externalParameters"].(map[string]string)["commit"] = attestationWorkflowSHA
				}
				return verifiedProvenance(t, s), nil
			}}
			err := verifier.verify(context.Background(), &manifest)
			require.Error(t, err)
			if strings.Contains(mode, "signature failure") {
				require.ErrorIs(t, err, toolErr)
			} else {
				require.ErrorIs(t, err, ErrReleaseVerification)
			}
			if mode == "SBOM signature failure" {
				require.Equal(t, 1, sbomCalls)
			} else {
				require.Zero(t, sbomCalls)
			}
		})
	}
}

func TestVerifyAttestationManifestInput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "release-manifest.json")
	require.Error(t, (Release{}).VerifyAttestations(dir, attestationRepository, "v1.234.0", attestationSourceSHA, attestationWorkflowSHA))
	require.NoError(t, os.WriteFile(file, []byte("{"), 0o600))
	require.Error(t, (Release{}).VerifyAttestations(dir, attestationRepository, "v1.234.0", attestationSourceSHA, attestationWorkflowSHA))
	for _, manifest := range []ArtifactManifest{
		{Tag: "v1.235.0", Commit: attestationSourceSHA},
		{Tag: "v1.234.0", Commit: attestationWorkflowSHA},
	} {
		data, err := json.Marshal(manifest)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(file, data, 0o600))
		require.ErrorIs(t, (Release{}).VerifyAttestations(dir, attestationRepository, "v1.234.0", attestationSourceSHA, attestationWorkflowSHA), ErrReleaseVerification)
	}
}

func TestVerifyAttestationsFailsClosedOnInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name, repo, workflow string
		assets               map[string]string
	}{
		{"nil assets", attestationRepository, attestationWorkflowSHA, nil},
		{"empty assets", attestationRepository, attestationWorkflowSHA, map[string]string{}},
		{"invalid repository", "invalid", attestationWorkflowSHA, map[string]string{"asset": "digest"}},
		{"invalid signer digest", attestationRepository, "short", map[string]string{"asset": "digest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := ArtifactManifest{Tag: "v1.234.0", Commit: attestationSourceSHA, Assets: tc.assets}
			verifier := attestationVerifier{Directory: t.TempDir(), Repository: tc.repo, WorkflowSHA: tc.workflow, Run: func(context.Context, []byte, string, ...string) ([]byte, error) {
				t.Fatal("invalid inputs must not invoke attestation verification")
				return nil, nil
			}}
			err := verifier.verify(context.Background(), &manifest)
			require.ErrorIs(t, err, ErrReleaseVerification)
		})
	}
	dir := t.TempDir()
	data, err := json.Marshal(ArtifactManifest{Tag: "v1.234.0", Commit: attestationSourceSHA})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release-manifest.json"), data, 0o600))
	require.ErrorIs(t, (Release{}).VerifyAttestations(dir, attestationRepository, "v1.234.0", attestationSourceSHA, attestationWorkflowSHA), ErrReleaseVerification)
}
