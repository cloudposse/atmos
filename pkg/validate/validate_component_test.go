package validate

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestValidateComponent(t *testing.T) {
	_, msg, err := e.ExecuteValidateComponent("infra/vpc", "tenant1-ue2-dev", "validate-infra-vpc-component.rego", "opa")
	u.PrintError(err)
	assert.Nil(t, err)
	u.PrintMessage(msg)
}
