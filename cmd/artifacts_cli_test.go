package cmd

import (
	"bytes"
	"encoding/json"
	stdio "io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	artifactscmd "github.com/cloudposse/atmos/cmd/artifacts"
	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// artifactsTestSigner is the identity the fake signer signs as and the configuration trusts.
var artifactsTestSigner = &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}

// artifactsProject is a temporary Atmos project with real `local/dir` and `local/archive`
// repositories, loaded through the real configuration pipeline.
type artifactsProject struct {
	t      *testing.T
	root   string
	source string
}

// newArtifactsProject writes atmos.yaml, a source tree, and wires the artifacts commands to the real
// composition root with the fake signer and verifier. No registry, bucket, or Cosign is involved.
func newArtifactsProject(t *testing.T, trusted bool) *artifactsProject {
	t.Helper()
	_ = NewTestKit(t)
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	for _, name := range []string{
		"ATMOS_ARTIFACTS_REPOSITORY", "ATMOS_ARTIFACTS_NAME", "ATMOS_ARTIFACTS_VERSION", "ATMOS_ARTIFACTS_FORMAT",
		"ATMOS_ARTIFACTS_FORCE", "ATMOS_ARTIFACTS_DRY_RUN", "ATMOS_ARTIFACTS_LABELS",
	} {
		t.Setenv(name, "")
	}

	lines := []string{"artifacts:"}
	if trusted {
		lines = append(
			lines,
			"  verification:",
			"    trusted_identities:",
			"      - identity: "+artifactsTestSigner.Identity,
			"        issuer: "+artifactsTestSigner.Issuer,
		)
	}
	config := strings.Join(append(lines, []string{
		"  repositories:",
		"    work:",
		"      kind: local/dir",
		"      options:",
		"        path: ./repositories/work",
		"    backups:",
		"      kind: local/archive",
		"      options:",
		"        path: ./repositories/backups",
		"",
	}...), "\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(config), 0o600))

	project := &artifactsProject{t: t, root: root, source: filepath.Join(root, "dist")}
	project.writeSource("hello\n", "readme v1\n")

	loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	artifactscmd.SetAtmosConfig(&loaded)
	restore := artifactscmd.UseServiceFactory(func(config *schema.AtmosConfiguration, _ artifactscmd.ServiceOptions) (artifact.Service, error) {
		composition, err := compose.New(config, compose.WithVerifier(&artifacttest.FakeVerifier{}), compose.WithSigner(artifactsTestSigner))
		if err != nil {
			return nil, err
		}
		return composition.Service, nil
	})
	t.Cleanup(func() {
		restore()
		artifactscmd.SetAtmosConfig(nil)
	})
	return project
}

// writeSource replaces the source tree with a data file and distribution documents.
func (p *artifactsProject) writeSource(content, readme string) {
	p.t.Helper()
	require.NoError(p.t, os.RemoveAll(p.source))
	for path, body := range map[string]string{
		"data.txt":                       content,
		filepath.Join("nested", "n.txt"): "nested\n",
		filepath.Join(".atmos", "artifact", "README.md"):    readme,
		filepath.Join(".atmos", "artifact", "CHANGELOG.md"): "- first\n",
	} {
		full := filepath.Join(p.source, path)
		require.NoError(p.t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(p.t, os.WriteFile(full, []byte(body), 0o644))
	}
}

// exists reports whether a project-relative path exists.
func (p *artifactsProject) exists(parts ...string) bool {
	_, err := os.Stat(filepath.Join(append([]string{p.root}, parts...)...))
	return err == nil
}

// cliRun is the captured outcome of one real command run through RootCmd.
type cliRun struct {
	stdout string
	stderr string
	err    error
}

// artifactsCommands lists the registered artifacts command and every subcommand below it.
func artifactsCommands() []*cobra.Command {
	var found []*cobra.Command
	for _, command := range RootCmd.Commands() {
		if command.Name() == "artifacts" {
			walkCommandTree(command, func(c *cobra.Command) { found = append(found, c) })
		}
	}
	return found
}

// artifactsStreams is an io.Streams backed by buffers, so a test can tell stdout (data) from
// stderr (status).
type artifactsStreams struct {
	stdout, stderr *bytes.Buffer
}

func (s *artifactsStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *artifactsStreams) Output() stdio.Writer    { return s.stdout }
func (s *artifactsStreams) Error() stdio.Writer     { return s.stderr }
func (s *artifactsStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *artifactsStreams) RawError() stdio.Writer  { return s.stderr }

var artifactsANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// atmosArtifacts runs `atmos artifacts <args...>` on a fresh command tree built by the same
// constructor the registry uses, with the data and UI channels bound to separate buffers. A fresh
// tree starts from default flags, so runs never see each other's flag values.
func (p *artifactsProject) atmosArtifacts(args ...string) cliRun {
	p.t.Helper()
	captured := &artifactsStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(captured))
	require.NoError(p.t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	defer data.Reset()

	command := artifactscmd.NewCommand()
	command.SilenceUsage, command.SilenceErrors = true, true
	command.SetOut(stdio.Discard)
	command.SetErr(stdio.Discard)
	command.SetArgs(args)
	runErr := command.ExecuteContext(p.t.Context())
	return cliRun{stdout: captured.stdout.String(), stderr: artifactsANSI.ReplaceAllString(captured.stderr.String(), ""), err: runErr}
}

func jsonObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &object), raw)
	return object
}

func TestArtifactsCommandsAreRegisteredDocumentedAndConflictFree(t *testing.T) {
	_ = NewTestKit(t)
	commands := artifactsCommands()
	require.NotEmpty(t, commands, "the artifacts command registers through the command registry")

	paths := map[string]*cobra.Command{}
	for _, command := range commands {
		paths[command.CommandPath()] = command
		// Merging inherited persistent flags panics on a shorthand or name conflict with the root flags.
		require.NotPanics(t, func() { _ = command.InheritedFlags() }, command.CommandPath())
	}
	for _, path := range []string{
		"atmos artifacts", "atmos artifacts publish", "atmos artifacts pull", "atmos artifacts copy", "atmos artifacts inspect",
		"atmos artifacts list", "atmos artifacts search", "atmos artifacts delete", "atmos artifacts sign", "atmos artifacts verify",
		"atmos artifacts show", "atmos artifacts show readme", "atmos artifacts show changelog",
	} {
		command, found := paths[path]
		require.True(t, found, path)
		assert.NotEmpty(t, internal.GetCommandMetadata(command).DocumentationURL, "%s links to its documentation page", path)
		assert.Equal(t, "true", command.Annotations["experimental"], "%s is labeled experimental", path)
	}
	assert.NotContains(t, paths, "atmos artifacts catalog", "catalog commands belong to a later work package")
}

func TestArtifactsFlagsMatchTheContractTable(t *testing.T) {
	_ = NewTestKit(t)
	expected := map[string][]string{
		"publish":        {"repository", "name", "version", "kind", "force", "dry-run", "labels", "readme", "changelog", "format"},
		"pull":           {"repository", "output", "force", "format"},
		"copy":           {"from", "to", "force", "format"},
		"inspect":        {"repository", "format"},
		"sign":           {"repository", "format"},
		"verify":         {"repository", "format"},
		"list":           {"repository", "labels", "refresh", "format"},
		"search":         {"repository", "labels", "refresh", "format"},
		"delete":         {"repository", "format"},
		"show readme":    {"repository", "format"},
		"show changelog": {"repository", "format"},
	}
	for _, command := range artifactsCommands() {
		path := strings.TrimPrefix(command.CommandPath(), "atmos artifacts ")
		want, ok := expected[path]
		if !ok {
			continue
		}
		for _, name := range want {
			assert.NotNil(t, command.Flags().Lookup(name), "%s has --%s", path, name)
		}
	}
}

// The full lifecycle, end to end, on real local repositories: publish, list, inspect, show, pull,
// copy, sign, verify, delete. Every step goes through the real flag parsing, validation,
// composition root, service, and repositories.
func TestArtifactsLifecycleOnRealLocalRepositories(t *testing.T) {
	p := newArtifactsProject(t, false)

	// A dry run plans the publication and creates nothing.
	dry := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--dry-run", "--format=json")
	require.NoError(t, dry.err)
	assert.Equal(t, "dry-run", jsonObject(t, dry.stdout)["outcome"])
	assert.False(t, p.exists("repositories"), "a dry run creates no repository directory")

	// Publish.
	published := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--labels=team=platform,env=dev", "--format=json")
	require.NoError(t, published.err)
	object := jsonObject(t, published.stdout)
	for _, name := range []string{"ref", "location", "resolved_digest", "record_digest", "content_digest", "representation_digest", "size", "outcome"} {
		assert.Contains(t, object, name, "the named output %s", name)
	}
	assert.Equal(t, "created", object["outcome"])
	assert.Equal(t, "team/app:1.0.0", object["ref"])
	assert.True(t, p.exists("repositories", "work"))
	recordDigest, _ := object["record_digest"].(string)
	require.NotEmpty(t, recordDigest)

	// An identical retry is a no-op; different content needs --force.
	again := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--labels=team=platform,env=dev", "--format=json")
	require.NoError(t, again.err)
	assert.Equal(t, "unchanged", jsonObject(t, again.stdout)["outcome"])

	p.writeSource("changed\n", "readme v2\n")
	conflict := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--labels=team=platform,env=dev", "--format=json")
	require.ErrorIs(t, conflict.err, errUtils.ErrArtifactConflict, "different content at the same reference fails without --force")
	assert.Equal(t, "failed", jsonObject(t, conflict.stdout)["outcome"], "the envelope is still emitted")
	assert.Contains(t, strings.Join(cockroach.GetAllHints(conflict.err), "\n"), "--force")
	forced := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--labels=team=platform,env=dev", "--force", "--format=json")
	require.NoError(t, forced.err)
	assert.Equal(t, "replaced", jsonObject(t, forced.stdout)["outcome"])
	p.writeSource("hello\n", "readme v1\n")
	require.NoError(t, p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=2.0.0", "--labels=team=platform,env=dev", "--format=json").err)

	// List and search.
	listed := p.atmosArtifacts("list", "--format=json")
	require.NoError(t, listed.err)
	entries, ok := jsonObject(t, listed.stdout)["artifacts"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 2)
	assert.Equal(t, "team/app:1.0.0", entries[0].(map[string]any)["ref"])
	assert.Equal(t, "work", entries[0].(map[string]any)["repository"])
	filtered := p.atmosArtifacts("list", "team/app", "--repository=work", "--labels=team=platform", "--format=yaml")
	require.NoError(t, filtered.err)
	var yamlListing struct {
		Artifacts []map[string]any `yaml:"artifacts"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(filtered.stdout), &yamlListing))
	assert.Len(t, yamlListing.Artifacts, 2)
	noMatch := p.atmosArtifacts("search", "app", "--labels=team=other", "--format=json")
	require.NoError(t, noMatch.err)
	assert.JSONEq(t, `{"artifacts": [], "repositories": [{"repository": "backups", "count": 0}, {"repository": "work", "count": 0}]}`, noMatch.stdout)
	table := p.atmosArtifacts("list")
	require.NoError(t, table.err)
	assert.Contains(t, table.stdout, "team/app")
	assert.Contains(t, table.stdout, "env=dev,team=platform")

	// Inspect and show read metadata and documents without downloading the payload.
	inspected := p.atmosArtifacts("inspect", "team/app:1.0.0", "--repository=work")
	require.NoError(t, inspected.err)
	assert.Contains(t, inspected.stdout, "record_digest:")
	assert.Contains(t, inspected.stdout, "verification:          unchecked")
	readme := p.atmosArtifacts("show", "readme", "team/app:2.0.0", "--repository=work")
	require.NoError(t, readme.err)
	assert.Equal(t, "readme v1\n", readme.stdout)
	changelog := p.atmosArtifacts("show", "changelog", "team/app:2.0.0", "--repository=work")
	require.NoError(t, changelog.err)
	assert.Equal(t, "- first\n", changelog.stdout)

	// Pull materializes the content; a different existing output needs --force and is replaced, not overlaid.
	out := filepath.Join(p.root, "out")
	pulled := p.atmosArtifacts("pull", "team/app:2.0.0", "--repository=work", "--output="+out)
	require.NoError(t, pulled.err)
	body, err := os.ReadFile(filepath.Join(out, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(body))
	require.NoError(t, os.WriteFile(filepath.Join(out, "stale.txt"), []byte("stale"), 0o600))
	refused := p.atmosArtifacts("pull", "team/app:2.0.0", "--repository=work", "--output="+out)
	require.Error(t, refused.err, "an existing, different output directory needs --force")
	forcedPull := p.atmosArtifacts("pull", "team/app:2.0.0", "--repository=work", "--output="+out, "--force")
	require.NoError(t, forcedPull.err)
	assert.NoFileExists(t, filepath.Join(out, "stale.txt"), "stale files are never overlaid")

	// Copy from the directory repository to the archive repository.
	copied := p.atmosArtifacts("copy", "team/app:1.0.0", "--from=work", "--to=backups", "--format=json")
	require.NoError(t, copied.err)
	assert.Equal(t, "created", jsonObject(t, copied.stdout)["outcome"])
	assert.NotEmpty(t, jsonObject(t, copied.stdout)["representation_digest"])
	assert.True(t, p.exists("repositories", "backups"))

	// Sign by digest. Without trust configuration nothing can be verified: verify fails as unchecked.
	current := p.atmosArtifacts("inspect", "team/app:1.0.0", "--repository=work", "--format=json")
	require.NoError(t, current.err)
	currentDigest, _ := jsonObject(t, current.stdout)["record_digest"].(string)
	require.NotEqual(t, recordDigest, currentDigest, "--force created a replacement record; the old digest is no longer retained")
	signed := p.atmosArtifacts("sign", "team/app@"+currentDigest, "--repository=work", "--format=json")
	require.NoError(t, signed.err)
	assert.Equal(t, "record", jsonObject(t, signed.stdout)["subject"].(map[string]any)["kind"])
	notDigest := p.atmosArtifacts("sign", "team/app:1.0.0", "--repository=work")
	require.ErrorIs(t, notDigest.err, errUtils.ErrArtifactRefInvalid, "sign accepts digest references only")
	unchecked := p.atmosArtifacts("verify", "team/app:1.0.0", "--repository=work", "--format=json")
	require.ErrorIs(t, unchecked.err, errUtils.ErrArtifactConfigInvalid, "signing alone never makes an artifact verified")
	assert.Equal(t, "unchecked", jsonObject(t, unchecked.stdout)["verification"].(map[string]any)["status"])

	// Delete removes exactly the selected reference.
	deleted := p.atmosArtifacts("delete", "team/app:1.0.0", "--repository=work", "--format=json")
	require.NoError(t, deleted.err)
	assert.Equal(t, "deleted", jsonObject(t, deleted.stdout)["outcome"])
	missing := p.atmosArtifacts("delete", "team/app:1.0.0", "--repository=work", "--format=json")
	require.ErrorIs(t, missing.err, errUtils.ErrArtifactNotFound)
	assert.Equal(t, "failed", jsonObject(t, missing.stdout)["outcome"])
	remaining := p.atmosArtifacts("list", "--repository=work", "--format=json")
	require.NoError(t, remaining.err)
	assert.Len(t, jsonObject(t, remaining.stdout)["artifacts"], 1)
}

// With a trust policy configured, consumption verifies before it reads or writes anything, and a
// signature added by `sign` is the only thing that makes an artifact verified.
func TestArtifactsTrustPolicyGatesConsumptionAndSignAndVerifyAgree(t *testing.T) {
	p := newArtifactsProject(t, true)
	published := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0", "--format=json")
	require.NoError(t, published.err)
	object := jsonObject(t, published.stdout)
	resolved, record := object["resolved_digest"].(string), object["record_digest"].(string)
	out := filepath.Join(p.root, "out")

	unsigned := p.atmosArtifacts("verify", "team/app:1.0.0", "--repository=work", "--format=json")
	require.ErrorIs(t, unsigned.err, errUtils.ErrArtifactVerificationFailed)
	assert.Equal(t, "failed", jsonObject(t, unsigned.stdout)["outcome"])
	refused := p.atmosArtifacts("pull", "team/app:1.0.0", "--repository=work", "--output="+out)
	require.ErrorIs(t, refused.err, errUtils.ErrArtifactVerificationFailed, "required verification runs before materialization")
	assert.False(t, p.exists("out"), "nothing was materialized")
	refusedCopy := p.atmosArtifacts("copy", "team/app:1.0.0", "--from=work", "--to=backups")
	require.ErrorIs(t, refusedCopy.err, errUtils.ErrArtifactVerificationFailed)
	assert.False(t, p.exists("repositories", "backups"), "nothing was published to the destination")
	forcedCopy := p.atmosArtifacts("copy", "team/app:1.0.0", "--from=work", "--to=backups", "--force")
	require.ErrorIs(t, forcedCopy.err, errUtils.ErrArtifactVerificationFailed, "--force never bypasses verification")

	// The digest users see (resolved) and the Record digest both sign the same subject.
	require.Equal(t, resolved, record, "on a local directory the resolved digest is the Record digest")
	signed := p.atmosArtifacts("sign", "team/app@"+resolved, "--repository=work", "--format=json")
	require.NoError(t, signed.err)

	verified := p.atmosArtifacts("verify", "team/app:1.0.0", "--repository=work", "--format=json")
	require.NoError(t, verified.err)
	assert.Equal(t, "verified", jsonObject(t, verified.stdout)["verification"].(map[string]any)["status"])
	text := p.atmosArtifacts("verify", "team/app:1.0.0", "--repository=work")
	require.NoError(t, text.err)
	assert.Contains(t, text.stdout, "release@example.com (issuer https://issuer.example.com)")

	require.NoError(t, p.atmosArtifacts("pull", "team/app:1.0.0", "--repository=work", "--output="+out).err)
	assert.FileExists(t, filepath.Join(out, "data.txt"))
}

func TestArtifactsStdoutCarriesDataAndStderrCarriesStatus(t *testing.T) {
	p := newArtifactsProject(t, false)

	published := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app", "--version=1.0.0")
	require.NoError(t, published.err)
	assert.Contains(t, published.stdout, "record_digest:")
	assert.NotContains(t, published.stdout, "Published", "human status never lands on stdout")
	assert.Contains(t, published.stderr, "Published team/app:1.0.0 to repository work (created)")

	structured := p.atmosArtifacts("inspect", "team/app:1.0.0", "--repository=work", "--format=json")
	require.NoError(t, structured.err)
	assert.True(t, json.Valid([]byte(structured.stdout)), "stdout is pure JSON that can be piped: %q", structured.stdout)
}

func TestArtifactsCommandsNeverInventDefaults(t *testing.T) {
	p := newArtifactsProject(t, false)

	noRepository := p.atmosArtifacts("publish", p.source, "--name=team/app", "--version=1.0.0")
	require.ErrorIs(t, noRepository.err, errUtils.ErrRequiredFlagNotProvided, "no default repository, even with two configured")
	assert.False(t, p.exists("repositories"), "nothing was created")

	noVersion := p.atmosArtifacts("publish", p.source, "--repository=work", "--name=team/app")
	require.Error(t, noVersion.err, "no generated version")

	noLatest := p.atmosArtifacts("pull", "team/app", "--repository=work", "--output="+filepath.Join(p.root, "out"))
	require.ErrorIs(t, noLatest.err, errUtils.ErrArtifactRefInvalid, "no implicit latest")
	assert.False(t, p.exists("out"))

	unknown := p.atmosArtifacts("inspect", "team/app:1.0.0", "--repository=ghost")
	require.ErrorIs(t, unknown.err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, strings.Join(cockroach.GetAllHints(unknown.err), "\n"), "backups, work")
}

func TestArtifactsEnvironmentVariablesFeedFlags(t *testing.T) {
	p := newArtifactsProject(t, false)
	t.Setenv("ATMOS_ARTIFACTS_REPOSITORY", "work")
	t.Setenv("ATMOS_ARTIFACTS_FORMAT", "json")
	t.Setenv("ATMOS_ARTIFACTS_VERSION", "3.0.0")

	published := p.atmosArtifacts("publish", p.source, "--name=team/app")
	require.NoError(t, published.err)
	assert.Equal(t, "team/app:3.0.0", jsonObject(t, published.stdout)["ref"])

	override := p.atmosArtifacts("publish", p.source, "--name=team/app", "--version=3.1.0", "--repository=backups")
	require.NoError(t, override.err)
	assert.Equal(t, "backups", jsonObject(t, override.stdout)["repository"], "a flag overrides the environment")
}
