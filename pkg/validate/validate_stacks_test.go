package validate

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestValidateStacksCommand(t *testing.T) {
	err := e.ExecuteValidateStacks(nil, nil)
	u.PrintError(err)
	assert.NotNil(t, err)
}
