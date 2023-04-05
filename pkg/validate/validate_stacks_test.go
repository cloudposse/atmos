package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateStacksCommand(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(nil, nil)
	u.LogError(err)
	assert.NotNil(t, err)
}
