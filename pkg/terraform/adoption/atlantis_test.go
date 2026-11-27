package adoption

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAtlantisPlanWorkflow(t *testing.T) {
	for _, tc := range []struct {
		steps, err string
		files      []string
	}{
		{"[init, plan]", "", nil},
		{"[{init: {}}, {plan: {extra_args: [-var-file, vars/base.tfvars, -var-file=vars/dev.tfvars]}}]", "", []string{"vars/base.tfvars", "vars/dev.tfvars"}},
		{"[]", "must contain", nil},
		{"[plan]", "plan step 1 (plan)", nil},
		{"[init, {run: echo}]", "plan step 2 (run)", nil},
		{"[init, {env: {name: TEST}}]", "plan step 2 (env)", nil},
		{"[{init: {extra_args: [-upgrade]}}, plan]", "step 1 (init)", nil},
		{"[init, {plan: {extra_args: [-target=foo]}}]", "unsupported plan argument", nil},
		{"[init, {plan: {extra_args: [-var-file]}}]", "expected a literal", nil},
		{"[init, {plan: {extra_args: [-var-file, '$INPUT']}}]", "expected a literal", nil},
		{"[init, {plan: {args: []}}]", "args", nil},
		{"[init, {plan: bogus}]", "step 2 (plan)", nil},
		{"[init, [foo]]", "expected a built-in step", nil},
		{"[init, {run: value, extra: true}]", "expected a built-in step", nil},
		{"[init, plan, apply]", "step 3 (apply)", nil},
		{"[&first init, *first]", "step 2 (init)", nil},
	} {
		t.Run(tc.steps, func(t *testing.T) {
			var w atlantisWorkflow
			require.NoError(t, yaml.Unmarshal([]byte("plan:\n  steps: "+tc.steps), &w))
			got, err := w.files()
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.files, got)
		})
	}
	for _, tc := range []struct{ body, err string }{
		{"apply: {steps: [{run: a-custom-apply}]}", ""},
		{"plan: {}", ""},
		{"plan: {unexpected: true}", "unexpected"},
		{"before_plan: {steps: [run]}", "before_plan"},
	} {
		var w atlantisWorkflow
		require.NoError(t, yaml.Unmarshal([]byte(tc.body), &w))
		_, err := w.files()
		if tc.err != "" {
			require.ErrorContains(t, err, tc.err)
		} else {
			require.NoError(t, err)
		}
	}
}

const atlantisProjects = `version: 3
projects:
  - name: production
    dir: infra/net
    workflow: generated
    apply_requirements: [approved]
  - name: sandbox
    dir: infra/net
    workflow: regular
    terraform_distribution: opentofu
    terraform_version: v8.7.6
workflows:
  generated:
    plan:
      steps:
        - run: touch never-created
  regular:
    plan:
      steps:
        - init
        - plan:
            extra_args: [-var-file, common.tfvars, -var-file=dev.tfvars]
    apply:
      steps:
        - run: a-custom-apply
`

func TestAtlantisRegistration(t *testing.T) {
	r := registration(t)
	module := r.Dir
	r.Dir = filepath.Join(r.Project, "atlantis.yaml")
	write(t, r.Dir, atlantisProjects)
	for _, f := range []string{"common.tfvars", "dev.tfvars", "env/default.tfvars"} {
		write(t, filepath.Join(module, f), `value = "test"`)
	}
	var out bytes.Buffer
	r.Output = &out
	r.DryRun = true
	require.NoError(t, r.saveAtlantis(t.Context()))
	for _, line := range []string{"production", "sandbox", "plan step 1 (run)", "approved", "Atlantis retains"} {
		require.Contains(t, out.String(), line)
	}
	stackPath := filepath.Join(r.Project, "stacks", "dev.yaml")
	require.NoFileExists(t, stackPath)
	r.DryRun = false
	require.ErrorContains(t, r.saveAtlantis(t.Context()), "--project")
	r.AtlantisProject = "production"
	require.ErrorContains(t, r.saveAtlantis(t.Context()), "workflow generated: plan step 1 (run)")
	require.NoFileExists(t, filepath.Join(module, "never-created"))
	require.NoFileExists(t, stackPath)
	r.AtlantisProject = "unknown"
	require.ErrorContains(t, r.saveAtlantis(t.Context()), "unknown --project")
	r.AtlantisProject = "sandbox"
	r.DryRun = true
	out.Reset()
	require.NoError(t, r.saveAtlantis(t.Context()))
	require.Contains(t, out.String(), "atlantis.yaml#sandbox")
	require.NoFileExists(t, stackPath)
	r.DryRun = false
	require.NoError(t, r.saveAtlantis(t.Context()))
	actual, err := os.ReadFile(stackPath)
	require.NoError(t, err)
	data := string(actual)
	require.Contains(t, data, "atlantis_project: atlantis.yaml#sandbox")
	require.Contains(t, data, "terraform_workspace: default")
	require.Contains(t, data, "command: tofu")
	require.Contains(t, data, "tofu: 8.7.6")
	for _, f := range []string{"common.tfvars", "dev.tfvars", "env/default.tfvars"} {
		require.Contains(t, data, f)
	}
	require.Less(t, strings.Index(data, "common.tfvars"), strings.Index(data, "dev.tfvars"))
	require.Less(t, strings.Index(data, "dev.tfvars"), strings.Index(data, "env/default.tfvars"))
	var config schema.AtmosConfiguration
	config.Stacks.IncludedPaths = []string{"*.yaml"}
	config.StacksBaseAbsolutePath = filepath.Dir(stackPath)
	config.TerraformDirAbsolutePath = r.Project
	r.Config = &config
	require.NoError(t, r.saveAtlantis(t.Context()))
	retry, err := os.ReadFile(stackPath)
	require.NoError(t, err)
	require.Equal(t, actual, retry)
	write(t, stackPath, strings.Replace(data, "atlantis.yaml#sandbox", "atlantis.yaml#other", 1))
	require.ErrorContains(t, r.saveAtlantis(t.Context()), "not bound")
}

func TestAtlantisProjectInputs(t *testing.T) {
	r := registration(t)
	dir := r.Dir
	r.Dir = filepath.Join(r.Project, "atlantis.yaml")
	_, err := readAtlantis(r.Dir)
	require.Error(t, err)
	for _, tc := range []struct{ body, err string }{
		{"version: 2", "version 3"},
		{"version: 3\n---\nversion: 3", "one YAML document"},
		{"version: 3\nunknown: true", "unknown"},
		{"version: 3\nprojects: []", "no explicit"},
		{"version: 3\nprojects: [{name: same, dir: x},{name: same, dir: y}]", "duplicate"},
		{"version: 3\nprojects: [{dir: ../x}]", "literal project directory"},
		{"version: 3\nprojects: [{dir: '*.tf'}]", "literal project directory"},
		{"version: 3\nprojects: [{dir: infra/net, unknown: true}]", "unknown"},
		{"version: 3\nprojects: [{dir: infra/net, autoplan: {apply_requirements: [approved]}}]", "autoplan"},
		{"version: 3\nprojects: [{dir: infra/net, autoplan: []}]", "unmarshal"},
		{"version: 3\nprojects: [{dir: infra/net, workflow: external}]", "effective server workflow"},
		{"version: 3\nprojects: [{dir: infra/net, workspace: ../prod}]", "workspace"},
		{"version: 3\nprojects: [{dir: infra/net, workspace: blue}]", "select that existing workspace"},
		{"version: 3\nprojects: [{dir: infra/net, terraform_version: latest}]", "exact release"},
		{"version: 3\nprojects: [{dir: infra/net, terraform_version: 1.2.3}]", "requires Terraform v1.2.3"},
		{"version: 3\nprojects: [{dir: infra/net, terraform_distribution: other}]", "unsupported terraform_distribution"},
		{"version: 3\nprojects: [{dir: missing}]", "missing"},
		{"version: 3\nprojects: [{dir: infra/net, name: x/y}]", "--name"},
	} {
		write(t, r.Dir, tc.body)
		require.ErrorContains(t, r.saveAtlantis(t.Context()), tc.err, tc.body)
	}
	write(t, r.Dir, "version: 3\nprojects: [{dir: infra/net}]")
	r.Explicit = tofu
	require.NoError(t, r.saveAtlantis(t.Context()))
	p := atlantisProject{Distribution: "opentofu"}
	_, err = p.engine(terraform)
	require.ErrorContains(t, err, "conflicts")
	require.NoError(t, os.Remove(r.Dir))
	require.NoError(t, os.Mkdir(r.Dir, 0o700))
	_, err = readAtlantis(r.Dir)
	require.Error(t, err)
	write(t, filepath.Join(dir, "terragrunt.hcl"), "")
	_, err = r.atlantisRoot(r.Project, "infra/net")
	require.ErrorContains(t, err, "Terragrunt unit")
	require.NoError(t, os.Remove(filepath.Join(dir, "terragrunt.hcl")))
	write(t, filepath.Join(dir, "Chart.yaml"), "")
	require.NoError(t, os.Remove(filepath.Join(dir, "main.tf")))
	_, err = r.atlantisRoot(r.Project, "infra/net")
	require.ErrorContains(t, err, "expected a Terraform or OpenTofu")
	write(t, filepath.Join(dir, "env"), "occupied")
	_, err = atlantisWorkspaceFiles(dir, "blue", nil)
	require.Error(t, err)
}
