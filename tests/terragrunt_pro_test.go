package tests

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proRequest is one call the fake Atmos Pro server received.
type proRequest struct {
	Method string
	Path   string
	Query  string
	Body   []byte
}

// fakeAtmosPro records every request and acknowledges it the way the real API does. It is a
// stand-in for the Atmos Pro API: no request ever leaves the machine.
type fakeAtmosPro struct {
	server   *httptest.Server
	mutex    sync.Mutex
	requests []proRequest
}

func newFakeAtmosPro(t *testing.T) *fakeAtmosPro {
	t.Helper()
	fake := &fakeAtmosPro{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body bytes.Buffer
		_, _ = body.ReadFrom(r.Body)
		fake.mutex.Lock()
		fake.requests = append(fake.requests, proRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: body.Bytes()})
		fake.mutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"status":200}`))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// take returns and forgets the requests with the given method whose path ends with suffix.
func (f *fakeAtmosPro) take(method, suffix string) []proRequest {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	var matched, rest []proRequest
	for _, request := range f.requests {
		if request.Method == method && strings.HasSuffix(request.Path, suffix) {
			matched = append(matched, request)
		} else {
			rest = append(rest, request)
		}
	}
	f.requests = rest
	return matched
}

// terragruntProject is a throwaway Git repository with Terragrunt units, imported into stack "dev".
type terragruntProject struct {
	t    *testing.T
	root string
	repo *gogit.Repository
	pro  *fakeAtmosPro
}

const proUnitModule = `variable "greeting" {
  default = "unset"
}
resource "terraform_data" "unit" {
  input = var.greeting
}
output "value" {
  value = terraform_data.unit.output
}
`

// The unit directory is spelled with forward slashes only. On Windows, get_terragrunt_dir() returns
// backslashes, and joining it with a slash-separated suffix would produce mixed separators.
const proUnitDir = `${replace(get_terragrunt_dir(), "\\", "/")}`

const proNetworkUnit = `terraform { source = "../../../modules/unit" }
remote_state {
  backend  = "local"
  config   = { path = "` + proUnitDir + `/state/terraform.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}
inputs = { greeting = "network" }
`

const proAppUnit = `terraform {
  source = "../../../modules/unit"
  extra_arguments "vars" {
    commands           = ["plan", "apply"]
    required_var_files = ["` + proUnitDir + `/app.tfvars"]
  }
}
remote_state {
  backend  = "local"
  config   = { path = "` + proUnitDir + `/state/terraform.tfstate" }
  generate = { path = "backend.tf", if_exists = "overwrite" }
}
dependency "network" {
  config_path  = "../network"
  mock_outputs = { value = "mock" }
}
inputs = { greeting = "app" }
`

// buildTerragruntPluginPackage compiles the Terragrunt plugin from this checkout into a package
// directory and returns it. The plugin is its own Go module; the package needs the manifest,
// its schemas, and the executable for this platform.
func buildTerragruntPluginPackage(t *testing.T) string {
	t.Helper()
	source := filepath.Join(repoRoot, "plugins", "terragrunt")
	pkg := t.TempDir()
	for _, name := range []string{"plugin.yaml"} {
		data, err := os.ReadFile(filepath.Join(source, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkg, name), data, 0o644))
	}
	schemas, err := filepath.Glob(filepath.Join(source, "schemas", "*"))
	require.NoError(t, err)
	require.Positive(t, len(schemas), "the plugin ships its configuration schema")
	require.NoError(t, os.MkdirAll(filepath.Join(pkg, "schemas"), 0o755))
	for _, schema := range schemas {
		data, err := os.ReadFile(schema)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkg, "schemas", filepath.Base(schema)), data, 0o644))
	}
	executable := "plugin"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	output := filepath.Join(pkg, "bin", runtime.GOOS+"-"+runtime.GOARCH, executable)
	// A missing Go toolchain is an optional prerequisite; a plugin that does not compile is a failure.
	RequireExecutable(t, "go", "building the Terragrunt plugin from this checkout")
	build := exec.Command("go", "build", "-o", output, ".")
	build.Dir = source
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	combined, err := build.CombinedOutput()
	require.NoError(t, err, "the Terragrunt plugin must compile:\n%s", combined)
	return pkg
}

func newTerragruntProject(t *testing.T, pluginPackage string, pro *fakeAtmosPro) *terragruntProject {
	t.Helper()
	root := t.TempDir()
	project := &terragruntProject{t: t, root: root, pro: pro}
	tofu := RequireTofuPath(t)
	project.write("atmos.yaml", `base_path: "."
settings:
  pro:
    base_url: `+pro.server.URL+`
    endpoint: api/v1
    token: test-token
components:
  terraform:
    base_path: components/terraform
    command: `+filepath.ToSlash(tofu)+`
    terragrunt:
      enabled: true
      mode: native
      discovery:
        enabled: false
plugins:
  packages:
    terragrunt:
      path: `+filepath.ToSlash(pluginPackage)+`
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
`)
	project.write("modules/unit/main.tf", proUnitModule)
	project.write("live/dev/network/terragrunt.hcl", proNetworkUnit)
	project.write("live/dev/app/terragrunt.hcl", proAppUnit)
	project.write("live/dev/app/app.tfvars", "greeting = \"from-file\"\n")
	// A unit carries no vars, labels, or settings of its own; the stack that imports it supplies
	// them, and it is where Atmos Pro enablement for a unit is declared.
	project.write("stacks/dev.yaml", `import:
  - terragrunt://live/dev
components:
  terraform:
    app:
      settings:
        pro:
          enabled: true
          drift_detection:
            enabled: true
`)
	repo, err := gogit.PlainInit(root, false)
	require.NoError(t, err)
	project.repo = repo
	_, err = repo.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{"https://github.com/acme/infra.git"}})
	require.NoError(t, err)
	project.commit("initial units")
	return project
}

func (p *terragruntProject) write(path, content string) {
	p.t.Helper()
	target := filepath.Join(p.root, filepath.FromSlash(path))
	require.NoError(p.t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(p.t, os.WriteFile(target, []byte(content), 0o644))
}

func (p *terragruntProject) head() string {
	p.t.Helper()
	ref, err := p.repo.Head()
	require.NoError(p.t, err)
	return ref.Hash().String()
}

func (p *terragruntProject) commit(message string) {
	p.t.Helper()
	worktree, err := p.repo.Worktree()
	require.NoError(p.t, err)
	require.NoError(p.t, worktree.AddWithOptions(&gogit.AddOptions{All: true}))
	_, err = worktree.Commit(message, &gogit.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.invalid", When: time.Now()}})
	require.NoError(p.t, err)
}

// atmos runs the CLI built from this checkout in the project and returns its stdout and stderr.
func (p *terragruntProject) atmos(args ...string) (string, string, error) {
	p.t.Helper()
	cmd := atmosRunner.Command(args...)
	cmd.Dir = p.root
	cmd.Env = append(cmd.Env, "NO_COLOR=1", "ATMOS_TELEMETRY_ENABLED=false", "TF_IN_AUTOMATION=true")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// exitCode returns the process exit code of a finished command, or -1 when it did not exit normally.
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err == nil {
		return 0
	}
	return -1
}

func (p *terragruntProject) mustAtmos(args ...string) {
	p.t.Helper()
	stdout, stderr, err := p.atmos(args...)
	require.NoError(p.t, err, "atmos %s:\n%s\n%s", strings.Join(args, " "), stdout, stderr)
}

// affectedUpload runs `describe affected --upload` against base and returns the component names that
// reached Atmos Pro, with the dependents of each.
func (p *terragruntProject) affectedUpload(base string) map[string][]string {
	p.t.Helper()
	p.mustAtmos("describe", "affected", "--upload", "--base", base)
	posted := p.pro.take(http.MethodPost, "/affected-stacks")
	require.Len(p.t, posted, 1, "exactly one affected-stacks upload")
	var payload struct {
		Stacks []struct {
			Component  string `json:"component"`
			Stack      string `json:"stack"`
			Dependents []struct {
				Component string `json:"component"`
				Stack     string `json:"stack"`
			} `json:"dependents"`
		} `json:"stacks"`
	}
	require.NoError(p.t, json.Unmarshal(posted[0].Body, &payload))
	result := map[string][]string{}
	for _, stack := range payload.Stacks {
		assert.Equal(p.t, "dev", stack.Stack)
		dependents := []string{}
		for _, dependent := range stack.Dependents {
			dependents = append(dependents, dependent.Component)
		}
		result[stack.Component] = dependents
	}
	return result
}

// TestTerragruntUnitsAreOrdinaryAtmosProInstances proves, with the real CLI, real OpenTofu, and a
// fake Atmos Pro API, that native Terragrunt units reach Atmos Pro as ordinary Terraform components.
func TestTerragruntUnitsAreOrdinaryAtmosProInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the Terragrunt plugin and runs OpenTofu")
	}
	ensureAtmosRunner(t)
	pluginPackage := buildTerragruntPluginPackage(t)
	pro := newFakeAtmosPro(t)
	project := newTerragruntProject(t, pluginPackage, pro)

	t.Run("instance inventory lists units with component, stack, type and the settings.pro of their stack", func(t *testing.T) {
		project.mustAtmos("list", "instances", "--upload")
		posted := pro.take(http.MethodPost, "/instances")
		require.Len(t, posted, 1)
		var payload struct {
			Instances []struct {
				Component     string         `json:"component"`
				Stack         string         `json:"stack"`
				ComponentType string         `json:"component_type"`
				Settings      map[string]any `json:"settings"`
			} `json:"instances"`
		}
		require.NoError(t, json.Unmarshal(posted[0].Body, &payload))
		byComponent := map[string]map[string]any{}
		for _, instance := range payload.Instances {
			assert.Equal(t, "dev", instance.Stack)
			assert.Equal(t, "terraform", instance.ComponentType, "a unit is an ordinary Terraform component")
			byComponent[instance.Component] = instance.Settings
		}
		require.Contains(t, byComponent, "app")
		require.Contains(t, byComponent, "network")
		assert.Equal(t, map[string]any{"pro": map[string]any{"enabled": true, "drift_detection": map[string]any{"enabled": true}}}, byComponent["app"],
			"settings.pro declared by the importing stack enables Atmos Pro for the unit")
		assert.NotContains(t, byComponent["network"], "pro", "a unit the stack does not enable carries no Atmos Pro settings")
	})

	// Each case commits one change and asks which components Atmos Pro is told about, relative to the commit before it.
	change := func(path, content, message string) map[string][]string {
		base := project.head()
		project.write(path, content)
		project.commit(message)
		return project.affectedUpload(base)
	}

	t.Run("a changed unit selects it", func(t *testing.T) {
		assert.Equal(t, map[string][]string{"app": {}}, change("live/dev/app/terragrunt.hcl", proAppUnit+"# reviewed change\n", "change app unit"))
	})

	t.Run("a changed dependency selects its dependents and the upload carries the relation", func(t *testing.T) {
		affected := change("live/dev/network/terragrunt.hcl", proNetworkUnit+"# reviewed change\n", "change network unit")
		assert.Equal(t, []string{"app"}, affected["network"])
	})

	t.Run("a changed module file selects every unit that uses the module", func(t *testing.T) {
		affected := change("modules/unit/main.tf", proUnitModule+"# changed\n", "change module")
		assert.Contains(t, affected, "app")
		assert.Contains(t, affected, "network")
	})

	t.Run("a changed extra_arguments var file selects the unit that reads it", func(t *testing.T) {
		assert.Equal(t, map[string][]string{"app": {}}, change("live/dev/app/app.tfvars", "greeting = \"changed-file\"\n", "change var file"))
	})

	t.Run("the CI plan artifact of a unit carries its plan record", func(t *testing.T) {
		testPlanArtifactCarriesThePlanRecord(t, pluginPackage)
	})

	t.Run("planning and applying one unit reports its status like any component", func(t *testing.T) {
		// --upload-status makes plan report changes through its exit code, exactly as for ordinary components.
		_, planErr, err := project.atmos("terraform", "plan", "app", "-s", "dev", "--upload-status")
		require.Equal(t, 2, exitCode(err), planErr)
		plan := pro.take(http.MethodPatch, "/instances")
		require.Len(t, plan, 1, "one status upload for the planned unit")
		assert.Equal(t, http.MethodPatch, plan[0].Method)
		assert.Equal(t, "/api/v1/repos/acme/infra/instances", plan[0].Path)
		assert.Equal(t, "stack=dev&component=app", plan[0].Query)
		assert.JSONEq(t, `{"command":"plan","exit_code":2}`, statusBody(t, plan[0].Body))

		project.mustAtmos("terraform", "apply", "app", "-s", "dev", "--upload-status", "--", "-auto-approve")
		apply := pro.take(http.MethodPatch, "/instances")
		require.Len(t, apply, 1)
		assert.Equal(t, "stack=dev&component=app", apply[0].Query)
		assert.JSONEq(t, `{"command":"apply","exit_code":0}`, statusBody(t, apply[0].Body))

		// A unit whose stack does not enable Atmos Pro uploads nothing.
		_, _, err = project.atmos("terraform", "plan", "network", "-s", "dev", "--upload-status")
		require.Contains(t, []int{0, 2}, exitCode(err))
		assert.Empty(t, pro.take(http.MethodPatch, "/instances"), "Atmos Pro is enabled per component by settings.pro.enabled")
	})
}

// testPlanArtifactCarriesThePlanRecord checks that the CI plan store receives the record that lets
// another runner verify a unit's plan, next to the plan itself.
func testPlanArtifactCarriesThePlanRecord(t *testing.T, pluginPackage string) {
	t.Helper()
	project := newTerragruntProject(t, pluginPackage, newFakeAtmosPro(t))
	store := t.TempDir()
	config, err := os.ReadFile(filepath.Join(project.root, "atmos.yaml"))
	require.NoError(t, err)
	config = bytes.Replace(config, []byte("    command: "), []byte("    planfiles:\n      default: local\n      stores:\n        local:\n          type: local/dir\n          options:\n            path: "+filepath.ToSlash(store)+"\n    command: "), 1)
	project.write("atmos.yaml", string(config)+"ci:\n  enabled: true\n")

	_, stderr, err := project.atmos("terraform", "plan", "app", "-s", "dev", "--ci")
	require.NoError(t, err, stderr)

	bundles, err := filepath.Glob(filepath.Join(store, "dev", "app", "*.tfplan.tar"))
	require.NoError(t, err)
	require.Len(t, bundles, 1, "one planfile bundle for the unit in the CI plan store")
	file, err := os.Open(bundles[0])
	require.NoError(t, err)
	defer file.Close()
	var names []string
	reader := tar.NewReader(file)
	for {
		header, nextErr := reader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		require.NoError(t, nextErr)
		names = append(names, header.Name)
	}
	assert.Contains(t, names, "plan.tfplan")
	assert.Contains(t, names, "plan.tfplan.atmos.json", "the record that lets another runner verify the plan travels with it")
}

// statusBody keeps the stable fields of a status upload (the request also carries a timestamp).
func statusBody(t *testing.T, body []byte) string {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	kept := map[string]any{"command": payload["command"], "exit_code": payload["exit_code"]}
	data, err := json.Marshal(kept)
	require.NoError(t, err)
	return string(data)
}
