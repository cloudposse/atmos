package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
)

func TestDoctorNamingFixUsesProductionResults(t *testing.T) {
	for _, tc := range []struct {
		name, config, stack, want string
		fix                       bool
	}{
		{"stack", "stacks:\n  base_path: stacks\n  name_pattern: '{stage}' # retain\n", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app: {}\n", "name_template", true},
		{"workspace", "stacks:\n  base_path: stacks\n  name_template: '{{ .vars.stage }}'\n", "vars:\n  stage: dev\nmetadata:\n  terraform_workspace_pattern: '{component}' # retain\ncomponents:\n  terraform:\n    app: {}\n", "terraform_workspace_template", true},
		{"cluster", "stacks:\n  base_path: stacks\ncomponents:\n  helmfile:\n    cluster_name_pattern: '{stage}' # retain\n", "vars:\n  stage: dev\ncomponents:\n  helmfile:\n    app: {}\n", "cluster_name_template", true},
		{"mixed", "stacks:\n  base_path: stacks\n", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace_template: '{{ .vars.stage }}-{component}'\n", "", false},
		{"mixed-pattern", "templates:\n  settings:\n    enabled: true\nstacks:\n  base_path: stacks\n", "vars:\n  stage: dev\nmetadata:\n  terraform_workspace_pattern: '{{ .vars.stage }}-{component}' # retain\ncomponents:\n  terraform:\n    app: {}\n", "terraform_workspace_template", true},
		{"mixed-pattern-disabled-templating", "stacks:\n  base_path: stacks\n", "vars:\n  stage: dev\nmetadata:\n  terraform_workspace_pattern: '{{ .vars.stage }}-{component}'\ncomponents:\n  terraform:\n    app: {}\n", "", false},
		{"unsafe", "stacks:\n  base_path: stacks\n", "components:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace_pattern: '{{ env \"TOKEN\" }}-{component}'\n", "", false},
		{"missing-input", "stacks:\n  base_path: stacks\n", "components:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace_pattern: '{region}'\n", "", false},
		{"no-components", "stacks:\n  base_path: stacks\n  name_pattern: '{stage}'\n", "vars:\n  stage: dev\n", "", false},
		{"yaml-function", "stacks:\n  base_path: stacks\n", "vars:\n  stage: '!terraform.output vpc dev id'\ncomponents:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace_pattern: '{component}'\n", "", false},
		{"coordinated", "stacks:\n  base_path: stacks\n  name_pattern: '{stage}' # retain\n", "vars:\n  stage: dev\nmetadata:\n  terraform_workspace_pattern: '{stage}-{component}'\ncomponents:\n  terraform:\n    app: {}\n", "terraform_workspace_template", true},
		{"stack-literal-changes-name", "stacks:\n  base_path: stacks\n  name_pattern: 'prefix-{stage}'\n", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app: {}\n", "", false},
		{"cluster-component-changes-name", "stacks:\n  base_path: stacks\ncomponents:\n  helmfile:\n    cluster_name_pattern: '{component}'\n", "components:\n  helmfile:\n    app: {}\n", "", false},
		{"conflict", "stacks:\n  base_path: stacks\n  name_pattern: '{stage}'\n  name_template: '{{ .vars.stage }}'\n", "vars:\n  stage: dev\ncomponents:\n  terraform:\n    app: {}\n", "", false},
		{"unknown", "stacks:\n  base_path: stacks\n", "components:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace_pattern: '{env}-{name}-{stack}'\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", "")
			t.Setenv("ATMOS_PROFILE", "")
			t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "")
			require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o700))
			configPath := filepath.Join(root, "atmos.yaml")
			stackPath := filepath.Join(root, "stacks", "dev.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(tc.config), 0o600))
			require.NoError(t, os.WriteFile(stackPath, []byte(tc.stack), 0o600))
			engine := diagnostic.Engine{Stacks: staticStackValidator{}, BackupDir: func() (string, error) { return t.TempDir(), nil }}
			options := diagnostic.Options{Root: root, Only: []string{"legacy-naming"}, Fix: true, DryRun: true, Offline: true}
			report, err := engine.Run(context.Background(), options)
			require.NoError(t, err)
			wantStatus := "incomplete"
			if tc.fix || tc.name == "mixed" || tc.name == "mixed-pattern-disabled-templating" || tc.name == "stack-literal-changes-name" || tc.name == "cluster-component-changes-name" {
				wantStatus = "findings"
			}
			require.NotEmpty(t, report.Findings)
			require.Equal(t, wantStatus, namingCheck(t, &report).Status, "%+v", report.Findings)
			bytes, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, tc.config, string(bytes))
			require.Empty(t, report.ChangedFiles)
			options.DryRun = false
			report, err = engine.Run(context.Background(), options)
			require.NoError(t, err)
			if !tc.fix {
				require.Empty(t, report.ChangedFiles, "%+v", report.Findings)
				require.NotEmpty(t, report.Findings)
				for _, f := range report.Findings {
					require.False(t, f.Fixable)
					require.NotEmpty(t, f.Reason)
				}
				bytes, err = os.ReadFile(stackPath)
				require.NoError(t, err)
				require.Equal(t, tc.stack, string(bytes))
				return
			}
			expectedFiles := 1
			if tc.name == "coordinated" {
				expectedFiles = 2
			}
			require.Len(t, report.ChangedFiles, expectedFiles, "%+v", report.Findings)
			require.NotEmpty(t, report.Backup)
			a, err := os.ReadFile(configPath)
			require.NoError(t, err)
			b, err := os.ReadFile(stackPath)
			require.NoError(t, err)
			require.Contains(t, string(a)+string(b), tc.want)
			require.Contains(t, string(a)+string(b), "# retain")
			report, err = engine.Run(context.Background(), options)
			require.NoError(t, err)
			require.Empty(t, report.ChangedFiles)
			require.Empty(t, report.Findings)
			require.Equal(t, "passed", namingCheck(t, &report).Status)
		})
	}
}

func TestDoctorNamingRefusesSourceDrift(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_PROFILE", "")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "")
	path := filepath.Join(root, "atmos.yaml")
	original := "stacks:\n  base_path: stacks\n  name_pattern: '{stage}'\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "stacks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte("vars:\n  stage: dev\ncomponents:\n  terraform:\n    app: {}\n"), 0o600))
	engine := diagnostic.Engine{Stacks: staticStackValidator{}, BackupDir: func() (string, error) { return t.TempDir(), nil }}
	engine.Observe = func(event diagnostic.Event) {
		if event.Phase == "Back up" && event.Done {
			require.NoError(t, os.WriteFile(path, []byte(original+"# concurrent edit\n"), 0o600))
		}
	}
	report, err := engine.Run(context.Background(), diagnostic.Options{Root: root, Only: []string{"legacy-naming"}, Fix: true, Offline: true})
	require.ErrorIs(t, err, diagnostic.ErrChanged)
	require.Empty(t, report.ChangedFiles)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original+"# concurrent edit\n", string(content))
}

func namingCheck(t *testing.T, report *diagnostic.Report) diagnostic.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == "legacy-naming" {
			return check
		}
	}
	t.Fatal("legacy-naming coverage is missing")
	return diagnostic.Check{}
}
