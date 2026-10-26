package doctor

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudFormationRegionMigration(t *testing.T) {
	for _, prefix := range []string{"", "aws/cloudformation:\n", "components:\n  aws/cloudformation:\n    demo:\n", "components:\n  aws/cloudformation:\n    demo:\n      overrides:\n"} {
		for _, dry := range []bool{false, true} {
			t.Run(prefix+map[bool]string{true: "dry", false: "fix"}[dry], func(t *testing.T) {
				root := newProject(t)
				projectFile(t, root, "atmos.yaml", "{}\n")
				indent := strings.Repeat("  ", strings.Count(prefix, "\n"))
				content := prefix + indent + "settings:\n" + indent + "  aws_cloudformation:\n" + indent + "    # deployment region\n" + indent + "    region: us-west-2 # keep me\n" + indent + "  other: unchanged\n"
				path := projectFile(t, root, "stacks/dev.yaml", content)
				engine := vtStacksEngine(t, vtProductionValidator{})
				r, err := engine.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, DryRun: dry, Only: []string{"cloudformation-region"}})
				require.NoError(t, err)
				findings := vtFindingsByRule(&r, "cloudformation-region")
				require.Len(t, findings, 1, "%+v", r)
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				if dry {
					require.Equal(t, content, string(after))
					require.Empty(t, r.ChangedFiles)
					return
				}
				require.Equal(t, statusFixed, findings[0].Status, "%+v", r)
				require.NotEmpty(t, r.Backup)
				require.Contains(t, string(after), indent+"region: us-west-2")
				for _, text := range []string{"# deployment region", "# keep me", "other: unchanged"} {
					require.Contains(t, string(after), text)
				}
				again, err := engine.Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{"cloudformation-region"}})
				require.NoError(t, err)
				require.Empty(t, again.ChangedFiles)
			})
		}
	}
}

func TestCloudFormationRegionMigrationConflict(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	content := "region: us-east-1\nsettings:\n  aws_cloudformation:\n    region: us-west-2\n"
	path := projectFile(t, root, "stacks/dev.yaml", content)
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{"cloudformation-region"}})
	require.NoError(t, err)
	require.Empty(t, r.ChangedFiles)
	findings := vtFindingsByRule(&r, "cloudformation-region")
	require.Len(t, findings, 1)
	require.Equal(t, statusManual, findings[0].Status)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(after))
}

func TestCloudFormationMigrationRefusesImportPrecedenceChange(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  included_paths: [deploy/*]\n")
	projectFile(t, root, "stacks/catalog/base.yaml", "region: us-east-1\n")
	source := "import: [../catalog/base]\nsettings:\n  aws_cloudformation:\n    region: us-west-2\ncomponents:\n  aws/cloudformation:\n    demo:\n      path: template.yaml\n      stack_name: demo\n"
	path := projectFile(t, root, "stacks/deploy/dev.yaml", source)
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{ruleCloudFormationRegion}})
	require.NoError(t, err)
	require.Empty(t, r.ChangedFiles)
	findings := vtFindingsByRule(&r, ruleCloudFormationRegion)
	require.Len(t, findings, 1)
	require.Equal(t, statusVerificationFailed, findings[0].Status)
	require.Contains(t, findings[0].Reason, "effective CloudFormation")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, source, string(after))
}

func TestCloudFormationTimeoutMigration(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	path := projectFile(t, root, "stacks/dev.yaml", "components:\n  aws/cloudformation:\n    demo:\n      timeout_in_minutes: 30 # wait budget\n")
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{ruleCloudFormationTimeout}})
	require.NoError(t, err)
	findings := vtFindingsByRule(&r, ruleCloudFormationTimeout)
	require.Len(t, findings, 1)
	require.Equal(t, statusFixed, findings[0].Status, "%+v", r)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(after), "timeout: 30m # wait budget")
	require.NotContains(t, string(after), "timeout_in_minutes")
}

func TestCloudFormationMigrationDoesNotRewriteUserData(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	source := "vars:\n  settings:\n    aws_cloudformation:\n      region: arbitrary-data\ncomponents:\n  terraform:\n    demo:\n      settings:\n        aws_cloudformation:\n          region: arbitrary-data\n"
	path := projectFile(t, root, "stacks/dev.yaml", source)
	r, err := vtStacksEngine(t, vtProductionValidator{}).Run(context.Background(), Options{Root: root, Offline: true, Fix: true, Only: []string{ruleCloudFormationRegion}})
	require.NoError(t, err)
	require.Empty(t, r.ChangedFiles)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, source, string(after))
}
