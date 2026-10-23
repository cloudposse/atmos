package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type pipelineStep struct {
	If              string            `yaml:"if"`
	ID              string            `yaml:"id"`
	Name            string            `yaml:"name"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	With            map[string]string `yaml:"with"`
	Env             map[string]string `yaml:"env"`
	ContinueOnError bool              `yaml:"continue-on-error"`
}
type pipelineJob struct {
	Name        string            `yaml:"name"`
	Timeout     int               `yaml:"timeout-minutes"`
	Needs       any               `yaml:"needs"`
	If          string            `yaml:"if"`
	Uses        string            `yaml:"uses"`
	With        map[string]string `yaml:"with"`
	Environment string            `yaml:"environment"`
	Permissions map[string]string `yaml:"permissions"`
	Outputs     map[string]string `yaml:"outputs"`
	Secrets     any               `yaml:"secrets"`
	Steps       []pipelineStep    `yaml:"steps"`
}
type pipelineWorkflow struct {
	On          map[string]any    `yaml:"on"`
	RunName     string            `yaml:"run-name"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group  string `yaml:"group"`
		Cancel bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]pipelineJob `yaml:"jobs"`
}

func pipelineYAML(t *testing.T, path string, target any) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, target))
	return data
}

func pipelineAction(t *testing.T, name string) []pipelineStep {
	t.Helper()
	var action struct {
		Runs struct {
			Using string         `yaml:"using"`
			Steps []pipelineStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	pipelineYAML(t, ".github/actions/"+name+"/action.yml", &action)
	require.Equal(t, "composite", action.Runs.Using)
	require.NotEmpty(t, action.Runs.Steps)
	return action.Runs.Steps
}

func pipelineUses(t *testing.T, steps []pipelineStep, action string) pipelineStep {
	t.Helper()
	for _, step := range steps {
		if step.Uses == action || strings.HasPrefix(step.Uses, action+"@") {
			return step
		}
	}
	t.Fatalf("required pipeline action %s missing", action)
	return pipelineStep{}
}

func pipelineRunIndex(t *testing.T, steps []pipelineStep, command string) int {
	t.Helper()
	for i, step := range steps {
		if strings.Contains(step.Run, command) {
			return i
		}
	}
	t.Fatalf("required enforcement command %s missing", command)
	return -1
}

func TestReleaseEntryPointsShareGuardedPipelineFromDefaultBranch(t *testing.T) {
	for _, tc := range []struct{ file, mode string }{{"publish-release.yml", "${{ inputs.mode }}"}, {"nightlybuilds.yml", "nightly"}, {"feature-release.yml", "feature"}} {
		t.Run(tc.file, func(t *testing.T) {
			var wf pipelineWorkflow
			pipelineYAML(t, ".github/workflows/"+tc.file, &wf)
			require.Empty(t, wf.Permissions, "no implicit write permissions at workflow scope")
			require.Equal(t, "atmos-release", wf.Concurrency.Group, "stable, nightly and feature publishing must serialize")
			require.False(t, wf.Concurrency.Cancel, "a new request must not interrupt an upload/publication")
			require.Len(t, wf.Jobs, 1)
			job := wf.Jobs["release"]
			require.Equal(t, "./.github/workflows/release-pipeline.yml", job.Uses)
			require.Equal(t, tc.mode, job.With["mode"])
			require.Equal(t, "github.ref == format('refs/heads/{0}', github.event.repository.default_branch)", job.If, "workflow dispatch on another branch must not run privileged release tooling")
			require.Equal(t, map[string]any{
				"BOT_GITHUB_APP_PRIVATE_KEY": "${{ secrets.BOT_GITHUB_APP_PRIVATE_KEY }}",
				"OPENAI_API_KEY":             "${{ secrets.OPENAI_API_KEY }}",
			}, job.Secrets, "forward only the two declared secrets, including environment-only values")
			require.Equal(t, "read", job.Permissions["contents"])
			require.Equal(t, "write", job.Permissions["id-token"], "caller must grant signing permission used by the GoReleaser build job")
			if tc.file == "publish-release.yml" {
				require.Equal(t, "Atmos ${{ inputs.mode == 'candidate' && 'RC' || 'release' }} ${{ inputs.version }} [${{ inputs.request-id }}]", wf.RunName, "show release identity while retaining exact dispatch correlation")
				for _, input := range []string{"milestone", "version", "commit"} {
					require.Equal(t, "${{ inputs."+input+" }}", job.With[input])
				}
				require.Contains(t, wf.On, "workflow_dispatch")
			}
			if tc.file == "nightlybuilds.yml" {
				require.Contains(t, wf.On, "schedule")
				require.Contains(t, wf.On, "workflow_dispatch")
			}
			require.NotContains(t, wf.On, "release", "publishing must never cause a rebuild of attached binaries")
		})
	}
}

func TestReleasePipelineSeparatesBuildSigningFromPublicationCredentials(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &wf)
	require.Contains(t, wf.On, "workflow_call")
	call, ok := wf.On["workflow_call"].(map[string]any)
	require.True(t, ok)
	secrets, ok := call["secrets"].(map[string]any)
	require.True(t, ok)
	require.Len(t, secrets, 2, "declare only the publisher key and optional note summary key")
	for name, required := range map[string]bool{"BOT_GITHUB_APP_PRIVATE_KEY": true, "OPENAI_API_KEY": false} {
		declaration, ok := secrets[name].(map[string]any)
		require.True(t, ok, "missing secret declaration for %s", name)
		require.Equal(t, required, declaration["required"], name)
	}
	require.Empty(t, wf.Permissions)
	build := wf.Jobs["build"]
	require.Empty(t, build.Environment, "building feature source must never expose environment secrets")
	require.Equal(t, map[string]string{"contents": "read", "id-token": "write", "attestations": "write"}, build.Permissions, "build may sign and attest artifacts but cannot publish releases")
	for _, step := range build.Steps {
		data, err := yaml.Marshal(step)
		require.NoError(t, err)
		require.NotContains(t, string(data), "secrets.")
		require.NotContains(t, step.Uses, "create-github-app-token")
	}
	for _, name := range []string{"prepare", "verify", "publish"} {
		job := wf.Jobs[name]
		require.Equal(t, "${{ inputs.mode == 'feature' && 'feature-releases' || 'release' }}", job.Environment)
		require.Equal(t, "read", job.Permissions["contents"], "release writes require explicit scoped App tokens")
	}
	require.NotContains(t, wf.Jobs["prepare"].Permissions, "id-token", "tag reservation does not need signing credentials")
	require.NotContains(t, wf.Jobs["verify"].Permissions, "id-token", "verification must check existing signatures rather than mint new ones")
	require.NotEqual(t, "write", wf.Jobs["verify"].Permissions["attestations"])
	for name, job := range wf.Jobs {
		checkout := pipelineUses(t, job.Steps, "actions/checkout")
		require.Equal(t, "${{ github.workflow_sha }}", checkout.With["ref"], "%s must execute trusted tooling at the workflow SHA", name)
		require.Equal(t, "false", checkout.With["persist-credentials"])
		for _, step := range job.Steps {
			require.False(t, step.ContinueOnError, "security-critical release step must fail closed: %s", step.Name)
		}
	}
	pub := pipelineUses(t, wf.Jobs["verify"].Steps, "./.github/actions/release-verify")
	require.NotContains(t, wf.Jobs["publish"].Permissions, "id-token", "publication retries must reuse already signed artifacts")
	require.Equal(t, "${{ secrets.BOT_GITHUB_APP_PRIVATE_KEY }}", pub.With["app-private-key"])
	require.Equal(t, "${{ secrets.OPENAI_API_KEY }}", pub.With["openai-api-key"])
	require.Equal(t, "${{ github.repository }}/.github/workflows/release-pipeline.yml@${{ github.ref }}", pub.With["signing-workflow-ref"], "certificate identity must name the reusable signer, not the entry-point workflow")
}

func TestReleasePipelineTransfersOnlyExactRunArtifactIdentities(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &wf)
	require.Equal(t, "${{ steps.plan.outputs.artifact-id }}", wf.Jobs["prepare"].Outputs["plan-artifact"])
	require.Equal(t, "${{ steps.artifacts.outputs.artifact-id }}", wf.Jobs["build"].Outputs["artifact"])
	require.Equal(t, "${{ steps.sealed.outputs.artifact-id }}", wf.Jobs["verify"].Outputs["artifact"])
	for _, tc := range []struct{ job, artifact string }{{"verify", "${{ needs.build.outputs.artifact }}"}, {"publish", "${{ needs.verify.outputs.artifact }}"}} {
		found := map[string]bool{}
		for _, step := range wf.Jobs[tc.job].Steps {
			if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
				require.Empty(t, step.With["name"], "mutable artifact names must not choose the source of publishable bytes")
				require.Empty(t, step.With["pattern"])
				require.Empty(t, step.With["run-id"], "artifacts must belong to this run")
				found[step.With["artifact-ids"]] = true
			}
		}
		require.Equal(t, map[string]bool{"${{ needs.prepare.outputs.plan-artifact }}": true, tc.artifact: true}, found)
	}
	for _, tc := range []struct{ job, action string }{{"build", "release-build"}, {"verify", "release-verify"}, {"publish", "release-finalize"}} {
		step := pipelineUses(t, wf.Jobs[tc.job].Steps, "./.github/actions/"+tc.action)
		require.Equal(t, "${{ needs.prepare.outputs.tag }}", step.With["tag"])
		require.Equal(t, "${{ needs.prepare.outputs.commit }}", step.With["commit"])
	}
	complete := wf.Jobs["complete"]
	require.Equal(t, []any{"prepare", "publish"}, complete.Needs, "bookkeeping retries must not rebuild or republish")
	require.Equal(t, "inputs.mode == 'cut'", complete.If, "prereleases leave stable milestones open")
	require.Equal(t, "release", complete.Environment)
	require.NotContains(t, complete.Permissions, "id-token")
	pipelineRunIndex(t, complete.Steps, "release:complete")
	for _, step := range complete.Steps {
		require.NotContains(t, step.Uses, "release-build")
		require.NotContains(t, step.Uses, "release-finalize")
	}
}

func TestTaggedBuildChecksIdentityAndCannotPublish(t *testing.T) {
	steps := pipelineAction(t, "release-build")
	checkout := pipelineUses(t, steps, "actions/checkout")
	require.Equal(t, "${{ inputs.commit }}", checkout.With["ref"])
	require.Equal(t, "false", checkout.With["persist-credentials"])
	require.Equal(t, "0", checkout.With["fetch-depth"], "tag identity verification needs fetched tags")
	verify := pipelineRunIndex(t, steps, "git rev-parse HEAD")
	require.Contains(t, steps[verify].Run, `git rev-parse "refs/tags/$RELEASE_TAG^{commit}"`)
	require.Contains(t, steps[verify].Run, `git status --porcelain`)
	require.Equal(t, "${{ inputs.tag }}", steps[verify].Env["RELEASE_TAG"])
	require.Equal(t, "${{ inputs.commit }}", steps[verify].Env["RELEASE_COMMIT"])
	build := pipelineUses(t, steps, "goreleaser/goreleaser-action")
	skipStages := []string{}
	for _, argument := range strings.Fields(build.With["args"]) {
		if strings.HasPrefix(argument, "--skip=") {
			skipStages = strings.Split(strings.TrimPrefix(argument, "--skip="), ",")
		}
	}
	require.Equal(t, []string{"publish"}, skipStages, "retain GoReleaser signing, SBOMs, and tool-install hooks")
	require.Contains(t, build.With["args"], "--config ../.goreleaser.yml", "build configuration comes from trusted tooling checkout")
	require.Equal(t, "${{ inputs.tag }}", build.Env["GORELEASER_CURRENT_TAG"])
	require.NotEqual(t, "false", pipelineUses(t, steps, "./.github/actions/setup-go-cache").With["cache"], "retain Go cache for release build performance")
	for i, step := range steps {
		if step.Uses == build.Uses {
			require.Less(t, verify, i, "source identity must be checked before executing source build hooks")
		}
		require.False(t, step.ContinueOnError)
		encoded, err := yaml.Marshal(step)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secrets.")
	}
	sbom := pipelineUses(t, steps, "anchore/sbom-action")
	require.Equal(t, "false", sbom.With["upload-release-assets"])
}

func TestPublicationRequiresExistingSignaturesAndAttestationsBeforeBoundary(t *testing.T) {
	steps := pipelineAction(t, "release-verify")
	validate := pipelineRunIndex(t, steps, "release:validatePlan")
	verifyBuild := pipelineRunIndex(t, steps, "release:verifyBuild")
	sign := pipelineRunIndex(t, steps, "cosign verify-blob")
	seal := pipelineRunIndex(t, steps, "release:verifyArtifacts")
	draft := pipelineRunIndex(t, steps, "release:draft")
	attestVerify := pipelineRunIndex(t, steps, "release:verifyAttestations")
	require.LessOrEqual(t, validate, verifyBuild)
	require.Less(t, verifyBuild, sign, "inspect artifact identities before trusting their existing signature")
	require.Contains(t, steps[sign].Run, "cosign verify-blob")
	require.Contains(t, steps[sign].Run, `--certificate-identity="https://github.com/$SIGNING_WORKFLOW_REF"`)
	require.Equal(t, "${{ inputs.signing-workflow-ref }}", steps[sign].Env["SIGNING_WORKFLOW_REF"])
	require.NotContains(t, steps[sign].Run, "$GITHUB_WORKFLOW_REF", "GitHub caller workflow differs from reusable job_workflow_ref in certificate SAN")
	require.Contains(t, steps[sign].Run, "--certificate-oidc-issuer=https://token.actions.githubusercontent.com")
	require.Less(t, sign, seal, "manifest must seal the verified signature bundle too")
	require.Less(t, seal, draft)
	require.Less(t, draft, attestVerify)
	require.Equal(t, "${{ github.workflow_sha }}", steps[attestVerify].Env["RELEASE_WORKFLOW_SHA"])
	for _, step := range steps {
		require.False(t, step.ContinueOnError)
		require.NotContains(t, step.Run, "cosign sign-blob", "verify the GoReleaser signature without replacing it")
		require.NotContains(t, step.Uses, "actions/attest", "verification must check provenance emitted by the build job")
		if step.Uses == "./.github/actions/release-publish" {
			require.NotEqual(t, "publish", step.With["operation"])
		}
	}
	require.Equal(t, "v3.1.3", pipelineUses(t, steps, "sigstore/cosign-installer").With["cosign-release"], "use the same cosign version as GoReleaser's existing install hook")

	finalSteps := pipelineAction(t, "release-finalize")
	finalValidate := pipelineRunIndex(t, finalSteps, "release:validatePlan")
	publishIndex := -1
	for i, step := range finalSteps {
		require.NotContains(t, step.Run, "cosign sign-blob", "retrying uploads must not change signature or manifest bytes")
		require.NotContains(t, step.Run, "release:verifyArtifacts", "retrying uploads must not regenerate manifest")
		require.False(t, step.ContinueOnError)
		if step.Uses == "./.github/actions/release-publish" && step.With["operation"] == "publish" {
			require.Equal(t, -1, publishIndex, "publish once after all verification")
			publishIndex = i
			require.Equal(t, "${{ steps.publish-token.outputs.token }}", step.With["github-token"])
			require.Equal(t, "${{ inputs.tag }}", step.With["tag"])
			require.Equal(t, "${{ inputs.commit }}", step.With["commit"])
		}
	}
	require.Equal(t, publishIndex-1, finalValidate, "final live guard belongs immediately before publication")
}

func TestPublicationTokenCannotAgeDuringBuild(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &wf)
	job := wf.Jobs["publish"]
	require.Positive(t, job.Timeout, "publication must have a bounded job lifetime")
	require.Less(t, job.Timeout, 60, "publication must finish before its fresh App token expires")
	require.Contains(t, job.Needs, "verify", "publication starts only after build verification")
	steps := pipelineAction(t, "release-finalize")
	setup, mint, validate := -1, -1, pipelineRunIndex(t, steps, "release:validatePlan")
	for i, step := range steps {
		if step.Uses == "./.github/actions/setup-go-cache" {
			setup = i
		}
		if strings.HasPrefix(step.Uses, "actions/create-github-app-token@") {
			require.Equal(t, -1, mint, "one fresh publication token")
			mint = i
			require.Equal(t, "publish-token", step.ID)
		}
	}
	require.NotEqual(t, -1, setup)
	require.Greater(t, mint, setup, "mint after setup rather than spending token lifetime on cache restoration")
	require.Less(t, mint, validate, "final validation and publication use fresh credentials")
	require.Equal(t, "${{ steps.publish-token.outputs.token }}", steps[validate].Env["GH_TOKEN"])
}

func TestReleaseStagesReportOutcomesAndReservedIdentityEvenOnFailure(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &wf)
	for name, job := range wf.Jobs {
		summary := job.Steps[len(job.Steps)-1]
		require.Equal(t, "./.github/actions/release-summary", summary.Uses)
		require.Equal(t, "always()", summary.If, "%s must report failures too", name)
		require.Equal(t, name, summary.With["stage"])
		require.Equal(t, "${{ job.status }}", summary.With["status"])
		require.Equal(t, "${{ inputs.mode }}", summary.With["mode"])
		require.Contains(t, summary.With["checks"], ".outcome }}")
		require.NotContains(t, summary.With["checks"], "toJSON(steps)", "token-producing step outputs must never enter a summary")
		require.NotContains(t, summary.With["checks"], ".outputs.")
		if name == "prepare" {
			require.Equal(t, "${{ steps.prepare.outputs.tag }}", summary.With["tag"])
			require.Equal(t, "${{ steps.prepare.outputs.commit || inputs.commit }}", summary.With["commit"])
		} else {
			require.Contains(t, job.Name, "${{ needs.prepare.outputs.tag }}")
			require.Equal(t, "${{ needs.prepare.outputs.tag }}", summary.With["tag"])
			require.Equal(t, "${{ needs.prepare.outputs.commit }}", summary.With["commit"])
		}
	}
	for _, name := range []string{"prepare", "complete"} {
		job := wf.Jobs[name]
		token := pipelineUses(t, job.Steps, "actions/create-github-app-token")
		require.Equal(t, "${{ vars.BOT_GITHUB_APP_CLIENT_ID || vars.BOT_GITHUB_APP_ID }}", token.With["client-id"])
		require.NotContains(t, token.With, "app-id")
		check := pipelineRunIndex(t, job.Steps, "::error::Release credentials unavailable")
		require.Equal(t, "credentials", job.Steps[check].ID)
		require.Equal(t, token.With["private-key"], job.Steps[check].Env["APP_PRIVATE_KEY"])
		for index, step := range job.Steps {
			if step.ID == "token" {
				require.Less(t, check, index)
			}
		}
	}
	for _, action := range []string{"release-verify", "release-finalize"} {
		for _, step := range pipelineAction(t, action) {
			if strings.HasPrefix(step.Uses, "actions/create-github-app-token@") {
				require.Equal(t, "${{ inputs.client-id }}", step.With["client-id"])
				require.NotContains(t, step.With, "app-id")
			}
		}
	}
}

func TestReleaseCredentialPreflightDoesNotExposeSecrets(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-pipeline.yml", &wf)
	steps := wf.Jobs["prepare"].Steps
	check := steps[pipelineRunIndex(t, steps, "::error::Release credentials unavailable")]
	for _, tc := range []struct {
		name, id, key string
		valid         bool
	}{
		{"missing key", "855608", "", false},
		{"missing ID", "", "private-key-sentinel", false},
		{"legacy app ID", "855608", "private-key-sentinel", true},
		{"client ID", "Iv1.example", "private-key-sentinel", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := exec.Command("bash", "-c", check.Run)
			command.Env = []string{"APP_CLIENT_ID=" + tc.id, "APP_PRIVATE_KEY=" + tc.key}
			output, err := command.CombinedOutput()
			require.NotContains(t, string(output), "private-key-sentinel")
			if tc.valid {
				require.NoError(t, err)
				require.Empty(t, output)
			} else {
				require.Error(t, err)
				require.Contains(t, string(output), "::error::Release credentials unavailable")
				require.Contains(t, string(output), "explicit secret forwarding")
			}
		})
	}
}

func TestReleaseActionsAreDocumentedAndDoNotUseImplicitSecretsOrReleaseDrafter(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", ".github", "actions", "release-*", "action.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, dirs)
	for _, actionPath := range dirs {
		dir := filepath.Dir(actionPath)
		data, err := os.ReadFile(filepath.Join(dir, "README.md"))
		require.NoError(t, err, "action %s requires README", dir)
		require.NotEmpty(t, data)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	require.NoError(t, err)
	actions, err := filepath.Glob(filepath.Join("..", "..", ".github", "actions", "*", "action.yml"))
	require.NoError(t, err)
	for _, file := range append(files, actions...) {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var node yaml.Node
		require.NoError(t, yaml.Unmarshal(data, &node))
		var check func(*yaml.Node)
		check = func(n *yaml.Node) {
			if n.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(n.Content); i += 2 {
					key, value := n.Content[i].Value, n.Content[i+1]
					if key == "secrets" {
						require.NotEqual(t, "inherit", value.Value, "%s must scope secrets explicitly", file)
					}
					if key == "uses" {
						require.NotContains(t, value.Value, "release-drafter")
						require.NotContains(t, value.Value, "auto-release")
						require.NotContains(t, value.Value, "guarded-release")
					}
				}
			}
			for _, child := range n.Content {
				check(child)
			}
		}
		check(&node)
	}
}

func TestGoReleaserRetainsSigningAndSBOMToolHooks(t *testing.T) {
	var config struct {
		Before struct {
			Hooks []string `yaml:"hooks"`
		} `yaml:"before"`
		Signs []struct {
			Command   string   `yaml:"cmd"`
			Args      []string `yaml:"args"`
			Artifacts string   `yaml:"artifacts"`
			Signature string   `yaml:"signature"`
		} `yaml:"signs"`
		SBOMs []struct {
			Artifacts string `yaml:"artifacts"`
		} `yaml:"sboms"`
	}
	pipelineYAML(t, ".goreleaser.yml", &config)
	hooks := strings.Join(config.Before.Hooks, "\n")
	require.Contains(t, hooks, "go install github.com/sigstore/cosign/v3/cmd/cosign@v3.1.3")
	require.Contains(t, hooks, "go install github.com/anchore/syft/cmd/syft@v1.51.1")
	require.Len(t, config.Signs, 1)
	require.Equal(t, "cosign", config.Signs[0].Command)
	require.Equal(t, []string{"sign-blob", "--yes", "--bundle=${signature}", "${artifact}"}, config.Signs[0].Args)
	require.Equal(t, "checksum", config.Signs[0].Artifacts)
	require.Equal(t, "${artifact}.sigstore.json", config.Signs[0].Signature)
	formats := []string{}
	for _, sbom := range config.SBOMs {
		formats = append(formats, sbom.Artifacts)
	}
	require.ElementsMatch(t, []string{"binary", "package"}, formats)
}

func TestBuildAttestsEveryStagedArtifactAfterGeneratingSourceSBOM(t *testing.T) {
	steps := pipelineAction(t, "release-build")
	stageIndex := pipelineRunIndex(t, steps, "release:stageArtifacts")
	require.Contains(t, steps[stageIndex].Run, `source "$RUNNER_TEMP/release-build-attestation"`)
	sbom := pipelineUses(t, steps, "anchore/sbom-action")
	require.Equal(t, "v1.51.1", sbom.With["syft-version"])
	for i, step := range steps {
		if step.Uses == sbom.Uses {
			require.Less(t, i, stageIndex, "source SBOM must exist before collecting provenance subjects")
		}
	}
	provenanceIndex, attestIndex := -1, -1
	for i, step := range steps {
		if step.Uses == "./.github/actions/release-publish" {
			require.Equal(t, "provenance", step.With["operation"], "build must never call publishing operations")
			require.Empty(t, step.With["release-id"], "build provenance exists before any draft release")
			require.Equal(t, "${{ inputs.tag }}", step.With["tag"])
			require.Equal(t, "${{ inputs.commit }}", step.With["commit"])
			provenanceIndex = i
		}
		if strings.HasPrefix(step.Uses, "actions/attest-build-provenance@") {
			attestIndex = i
			require.Equal(t, "https://slsa.dev/provenance/v1", step.With["predicate-type"])
			require.Equal(t, "${{ runner.temp }}/release-provenance.json", step.With["predicate-path"])
			require.Equal(t, "${{ runner.temp }}/release-build-attestation/*", step.With["subject-path"], "attest every staged build output, including signatures and the source SBOM")
		}
	}
	require.Less(t, stageIndex, provenanceIndex)
	require.Less(t, provenanceIndex, attestIndex)
	sbomAttestation := pipelineUses(t, steps, "actions/attest")
	require.Equal(t, "${{ runner.temp }}/release-build-attestation/*_SHA256SUMS", sbomAttestation.With["subject-path"])
	require.Equal(t, "${{ runner.temp }}/release-build-attestation/atmos-sbom.spdx.json", sbomAttestation.With["sbom-path"])
}
