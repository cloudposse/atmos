package exec

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeStacksExec(t *testing.T) {
	ctrl := gomock.NewController(t)
	d := &describeStacksExec{
		pageCreator: pager.NewMockPageCreator(ctrl),
		isTTYSupportForStdout: func() bool {
			return false
		},
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
			return nil
		},
		executeDescribeStacks: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string) (map[string]any, error) {
			return map[string]any{
				"hello": "test",
			}, nil
		},
	}
	err := d.Execute(&schema.AtmosConfiguration{}, &DescribeStacksArgs{})
	assert.NoError(t, err)
}
