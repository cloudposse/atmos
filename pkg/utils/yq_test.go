package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEvaluateYqExpression(t *testing.T) {
	input := `
settings:
  test: true
  mode: test
vars:
  assign_generated_ipv6_cidr_block: false
  availability_zones:
    - us-east-2a
    - us-east-2b
    - us-east-2c
  enabled: true
  environment: ue2
  ipv4_primary_cidr_block: 10.8.0.0/18
  map_public_ip_on_launch: false
  max_subnet_count: 3
  name: common
  namespace: acme
  nat_eip_aws_shield_protection_enabled: false
  nat_gateway_enabled: true
  nat_instance_enabled: false
  region: us-east-2
  stage: prod
  tags:
    atmos_component: vpc
    atmos_manifest: orgs/acme/plat/prod/us-east-2
    atmos_stack: plat-ue2-prod
    terraform_component: vpc
    terraform_workspace: plat-ue2-prod
  tenant: plat
  vpc_flow_logs_enabled: true
  vpc_flow_logs_log_destination_type: s3
  vpc_flow_logs_traffic_type: ALL
`

	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Trace",
		},
	}

	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	// Test with nil atmosConfig to ensure it doesn't panic
	yq := ".settings.test"
	res, err := EvaluateYqExpression(nil, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)

	yq = ".settings.test"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".settings.mode"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "test", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.tags.atmos_component"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "vpc", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2a", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.0.0/18", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, false, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block = \"10.8.8.0/20\""
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.8.0/20", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0 = \"us-east-2d\""
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2d", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false | .vars.tags.terraform_workspace = \"plat-ue2-prod-override\" | .vars.max_subnet_count = 2 | .settings.test = false"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res1, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res1)
	err = PrintAsYAML(res1)
	assert.Nil(t, err)
	yq = ".vars.tags.terraform_workspace"
	res2, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, "plat-ue2-prod-override", res2)
	err = PrintAsYAML(res2)
	assert.Nil(t, err)
	yq = ".vars.max_subnet_count"
	res3, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, 2, res3)
	err = PrintAsYAML(res3)
	assert.Nil(t, err)
	yq = ".settings.test"
	res4, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res4)
	err = PrintAsYAML(atmosConfig, res4)
	assert.Nil(t, err)
}

func TestEvaluateYqExpressionWithNilConfig(t *testing.T) {
	input := `
settings:
  test: true
  mode: test
vars:
  enabled: true
  name: test
`
	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	// Test various expressions with nil atmosConfig
	testCases := []struct {
		name     string
		yq       string
		expected any
	}{
		{
			name:     "get boolean value",
			yq:       ".settings.test",
			expected: true,
		},
		{
			name:     "get string value",
			yq:       ".settings.mode",
			expected: "test",
		},
		{
			name:     "modify boolean value",
			yq:       ".vars.enabled = false | .vars.enabled",
			expected: false,
		},
		{
			name:     "add new field",
			yq:       ".vars.new_field = \"new value\" | .vars.new_field",
			expected: "new value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := EvaluateYqExpression(nil, data, tc.yq)
			assert.Nil(t, err)
			assert.NotNil(t, res)
			assert.Equal(t, tc.expected, res)
		})
	}
}
