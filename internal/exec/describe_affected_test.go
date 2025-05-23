package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDescribeAffected(t *testing.T) {
	d := describeAffectedExec{atmosConfig: &schema.AtmosConfiguration{}}
	d.IsTTYSupportForStdout = func() bool {
		return false
	}
	d.executeDescribeAffectedWithTargetRepoPath = func(atmosConfig schema.AtmosConfiguration, targetRefPath string, verbose, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}
	d.executeDescribeAffectedWithTargetRefClone = func(atmosConfig schema.AtmosConfiguration, ref, sha, sshKeyPath, sshKeyPassword string, verbose, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}
	d.executeDescribeAffectedWithTargetRefCheckout = func(atmosConfig schema.AtmosConfiguration, ref, sha string, verbose, includeSpaceliftAdminStacks, includeSettings bool, stack string, processTemplates, processYamlFunctions bool, skip []string) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error) {
		return []schema.Affected{}, nil, nil, "", nil
	}
	d.atmosConfig = &schema.AtmosConfiguration{}
	d.addDependentsToAffected = func(atmosConfig schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings bool) error {
		return nil
	}
	d.printOrWriteToFile = func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
		return nil
	}

	err := d.Execute(&DescribeAffectedCmdArgs{
		Format:   "json",
		RepoPath: "",
	})
	assert.NoError(t, err)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format:         "yaml",
		CloneTargetRef: true,
	})
	assert.NoError(t, err)

	d.IsTTYSupportForStdout = func() bool {
		return true
	}
	ctrl := gomock.NewController(t)
	mockPager := pager.NewMockPageCreator(ctrl)
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	d.pageCreator = mockPager
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format: "json",
	})
	assert.NoError(t, err)
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(&DescribeAffectedCmdArgs{
		Format: "yaml",
	})
	assert.NoError(t, err)
}
