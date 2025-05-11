package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestDescribeAffected(t *testing.T) {
	d := NewDescribeAffectedExec(&schema.AtmosConfiguration{})
	d.IsTTYSupportForStdout = func() bool {
		return false
	}
	err := d.Execute(DescribeAffectedCmdArgs{
		Format: "json",
	})
	assert.NoError(t, err)
	err = d.Execute(DescribeAffectedCmdArgs{
		Format: "yaml",
	})
	assert.NoError(t, err)

	d.IsTTYSupportForStdout = func() bool {
		return true
	}
	ctrl := gomock.NewController(t)
	mockPager := pager.NewMockPageCreator(ctrl)
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	d.pageCreator = mockPager
	err = d.Execute(DescribeAffectedCmdArgs{
		Format: "json",
	})
	assert.NoError(t, err)
	mockPager.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil)
	err = d.Execute(DescribeAffectedCmdArgs{
		Format: "yaml",
	})
	assert.NoError(t, err)

}
