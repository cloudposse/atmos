package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	legacyReconfigure = "components:\n  terraform:\n    base_path: components/terraform\n    init_run_reconfigure: false\n"
	legacyArgument    = "commands:\n  - name: inspect\n    arguments:\n      - name: target\n        type: component\n"
	legacyHookStack   = "hooks:\n  notify:\n    events: [after-terraform-apply]\n    command: store\n"
	genericBlockText  = "Dynamic or layered configuration requires"
)

func TestRepairsAreBlockedOnlyByInputsThatCanChangeThem(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		options func(*Options)
		rule    string
		status  string
		reasons []string
	}{
		{
			name:   "stacks.name_template elsewhere does not block the reconfigure repair",
			files:  map[string]string{"atmos.yaml": legacyReconfigure + "stacks:\n  base_path: stacks\n  name_template: \"{{ .vars.stage }}\"\n"},
			rule:   ruleTerraformReconfigure,
			status: "available",
		},
		{
			name:   "templated command steps and descriptions do not block the reconfigure repair",
			files:  map[string]string{"atmos.yaml": legacyReconfigure + "commands:\n  - name: run\n    description: \"Run {{ .Arguments.x }}\"\n    steps:\n      - echo \"{{ .Arguments.x }}\"\n"},
			rule:   ruleTerraformReconfigure,
			status: "available",
		},
		{
			name:   "a YAML function elsewhere does not block the reconfigure repair",
			files:  map[string]string{"atmos.yaml": legacyReconfigure + "vars:\n  dynamic: !exec 'echo hi'\n"},
			rule:   ruleTerraformReconfigure,
			status: "available",
		},
		{
			name:   "a dynamic sibling key does not block the reconfigure repair",
			files:  map[string]string{"atmos.yaml": "components:\n  terraform:\n    base_path: \"{{ .base }}\"\n    init_run_reconfigure: false\n"},
			rule:   ruleTerraformReconfigure,
			status: "available",
		},
		{
			name:    "a dynamic init block blocks the reconfigure repair and is named",
			files:   map[string]string{"atmos.yaml": "components:\n  terraform:\n    init_run_reconfigure: false\n    init: !include init.yaml\n"},
			rule:    ruleTerraformReconfigure,
			status:  statusVerificationIncomplete,
			reasons: []string{"YAML function !include", "atmos.yaml:4", "components.terraform.init"},
		},
		{
			name:    "an import blocks the reconfigure repair and is named",
			files:   map[string]string{"atmos.yaml": "import:\n  - extra.yaml\n" + legacyReconfigure, "extra.yaml": "logs:\n  level: Debug\n"},
			rule:    ruleTerraformReconfigure,
			status:  statusVerificationIncomplete,
			reasons: []string{"Imports layer other files into this configuration", "atmos.yaml:1"},
		},
		{
			name:    "an atmos.d layer blocks the reconfigure repair and is named",
			files:   map[string]string{"atmos.yaml": legacyReconfigure, "atmos.d/extra.yaml": "logs:\n  level: Debug\n"},
			rule:    ruleTerraformReconfigure,
			status:  statusVerificationIncomplete,
			reasons: []string{"Configuration is layered", "atmos.d/extra.yaml"},
		},
		{
			name:    "an unverifiable input blocks configuration repairs and names the layer",
			files:   map[string]string{"atmos.yaml": legacyReconfigure},
			options: func(o *Options) { o.UnverifiedInputs = []string{"Home configuration exists outside this project"} },
			rule:    ruleTerraformReconfigure,
			status:  statusVerificationIncomplete,
			reasons: []string{"Home configuration exists outside this project"},
		},
		{
			name:   "a dynamic value outside the argument does not block the argument repair",
			files:  map[string]string{"atmos.yaml": legacyArgument + "    description: \"Inspect {{ .Arguments.target }}\"\n    steps: [\"echo {{ .Arguments.target }}\"]\n"},
			rule:   ruleCommandProvides,
			status: "available",
		},
		{
			name:   "a dynamic default beside the argument type does not block the argument repair",
			files:  map[string]string{"atmos.yaml": "commands:\n  - name: inspect\n    arguments:\n      - name: target\n        type: component\n        default: \"{{ .x }}\"\n"},
			rule:   ruleCommandProvides,
			status: "available",
		},
		{
			name:    "a config import blocks the argument repair and is named",
			files:   map[string]string{"atmos.yaml": "import: [extra.yaml]\n" + legacyArgument, "extra.yaml": "logs:\n  level: Debug\n"},
			rule:    ruleCommandProvides,
			status:  statusVerificationIncomplete,
			reasons: []string{"Imports layer other files into this configuration", "atmos.yaml:1"},
		},
		{
			name:   "stack imports and templates do not block the hook repair",
			files:  map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": "import: [base]\nvars:\n  chart: \"{{ .Values.replicas }}\"\n  run: !exec 'echo hi'\n" + legacyHookStack, "stacks/base.yaml": "vars: {}\n"},
			rule:   ruleHookEvent,
			status: "available",
		},
		{
			name:   "config imports do not block the hook repair",
			files:  map[string]string{"atmos.yaml": "import: [extra.yaml]\n", "extra.yaml": "logs:\n  level: Debug\n", "stacks/dev.yaml": legacyHookStack},
			rule:   ruleHookEvent,
			status: "available",
		},
		{
			name:    "an unverifiable configuration input does not block the hook repair",
			files:   map[string]string{"atmos.yaml": "{}\n", "stacks/dev.yaml": legacyHookStack},
			options: func(o *Options) { o.UnverifiedInputs = []string{"Home configuration exists outside this project"} },
			rule:    ruleHookEvent,
			status:  "available",
		},
		{
			name:   "a malformed stack file does not block the configuration repair",
			files:  map[string]string{"atmos.yaml": legacyReconfigure, "stacks/broken.yaml": "a: [unterminated\n"},
			rule:   ruleTerraformReconfigure,
			status: "available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			for path, content := range tt.files {
				projectFile(t, root, path, content)
			}
			options := Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{categoryDeprecations}}
			if tt.options != nil {
				tt.options(&options)
			}

			r, err := testEngine(t).Run(context.Background(), options)

			require.NoError(t, err)
			found := ecFindings(&r, tt.rule)
			require.Len(t, found, 1, "%+v", r.Findings)
			require.Equal(t, tt.status, found[0].Status, found[0].Reason)
			require.Equal(t, tt.status == "available", found[0].Fixable)
			require.NotContains(t, found[0].Reason, genericBlockText)
			for _, want := range tt.reasons {
				require.Contains(t, found[0].Reason, want)
			}
			if tt.status != "available" {
				require.NotContains(t, found[0].Remediation, "--fix", "a blocked repair must not suggest --fix")
			}
		})
	}
}

func TestDynamicValuesAreNeverExecutedWhileRepairing(t *testing.T) {
	root := newProject(t)
	marker := filepath.Join(root, "forbidden")
	path := projectFile(t, root, "atmos.yaml", legacyReconfigure+"vars:\n  dynamic: !exec 'touch forbidden'\n  template: \"{{ .value }}\"\n")

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

	require.NoError(t, err)
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	require.NoFileExists(t, marker)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "reconfigure: never")
	require.Contains(t, string(content), "!exec 'touch forbidden'")
	require.Contains(t, string(content), "{{ .value }}")
}

func TestDefaultProfileIsHonoredLikeTheLoader(t *testing.T) {
	const base = legacyReconfigure + "profiles:\n  default: ci\n"
	tests := []struct {
		name     string
		profiles []string
		status   string
		reason   string
	}{
		{"profiles.default applies when no profile is selected", nil, statusVerificationIncomplete, "profiles/ci/atmos.yaml"},
		{"an explicit profile replaces profiles.default", []string{"other"}, statusVerificationIncomplete, "profiles/other/atmos.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", base)
			projectFile(t, root, "profiles/ci/atmos.yaml", "logs:\n  level: Debug\n")
			projectFile(t, root, "profiles/other/atmos.yaml", "logs:\n  level: Trace\n")

			r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: true, Profiles: tt.profiles, Only: []string{categoryDeprecations}})

			require.NoError(t, err)
			found := ecFindings(&r, ruleTerraformReconfigure)
			require.Len(t, found, 1)
			require.Equal(t, tt.status, found[0].Status)
			require.Contains(t, found[0].Reason, tt.reason)
		})
	}

	t.Run("no default and no profile leaves the project single-layer", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", legacyReconfigure)
		projectFile(t, root, "profiles/ci/atmos.yaml", "logs:\n  level: Debug\n")

		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{categoryDeprecations}})

		require.NoError(t, err)
		require.Equal(t, "available", ecFindings(&r, ruleTerraformReconfigure)[0].Status)
	})

	t.Run("a default profile that does not exist is reported", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", base)

		r, err := testEngine(t).Run(context.Background(), Options{Root: root, Offline: true, Only: []string{ruleImports}})

		require.NoError(t, err)
		found := ecFindings(&r, ruleImports)
		require.NotEmpty(t, found)
		require.Contains(t, found[0].Message, "Active profile was not found: ci")
	})
}
