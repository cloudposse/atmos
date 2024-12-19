package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateComponent(t *testing.T) {
	info := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.Nil(t, err)

	_, err = e.ExecuteValidateComponent(
		atmosConfig,
		info,
		"infra/vpc",
		"tenant1-ue2-dev",
		"vpc/validate-infra-vpc-component.rego",
		"opa",
		[]string{"catalog"},
		0)
	u.LogError(atmosConfig, err)
	assert.Error(t, err)
}

func TestValidateComponent2(t *testing.T) {
	info := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.Nil(t, err)

	_, err = e.ExecuteValidateComponent(
		atmosConfig,
		info,
		"infra/vpc",
		"tenant1-ue2-prod",
		"",
		"",
		[]string{"catalog/constants"},
		0)
	u.LogError(atmosConfig, err)
	assert.Error(t, err)
}

func TestValidateComponent3(t *testing.T) {
	info := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.Nil(t, err)

	_, err = e.ExecuteValidateComponent(
		atmosConfig,
		info,
		"infra/vpc",
		"tenant1-ue2-staging",
		"",
		"",
		nil,
		0)
	u.LogError(atmosConfig, err)
	assert.Error(t, err)
}

func TestValidateComponent4(t *testing.T) {
	info := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.Nil(t, err)

	_, err = e.ExecuteValidateComponent(
		atmosConfig,
		info,
		"derived-component-3",
		"tenant1-ue2-test-1",
		"",
		"",
		nil,
		0)
	u.LogError(atmosConfig, err)
	assert.Error(t, err)
	assert.Equal(t, "'service_1_name' variable length must be greater than 10 chars", err.Error())
}
