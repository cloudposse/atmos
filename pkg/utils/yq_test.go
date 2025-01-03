package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	yq := ".settings.test"
	res, err := EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".settings.mode"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "test", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.tags.atmos_component"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "vpc", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2a", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.0.0/18", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, false, res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block = \"10.8.8.0/20\""
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.8.0/20", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0 = \"us-east-2d\""
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2d", res)
	err = PrintAsYAML(res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false | .vars.tags.terraform_workspace = \"plat-ue2-prod-override\" | .vars.max_subnet_count = 2 | .settings.test = false"
	res, err = EvaluateYqExpression(data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res1, err := EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res1)
	err = PrintAsYAML(res1)
	assert.Nil(t, err)
	yq = ".vars.tags.terraform_workspace"
	res2, err := EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.Equal(t, "plat-ue2-prod-override", res2)
	err = PrintAsYAML(res2)
	assert.Nil(t, err)
	yq = ".vars.max_subnet_count"
	res3, err := EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.Equal(t, 2, res3)
	err = PrintAsYAML(res3)
	assert.Nil(t, err)
	yq = ".settings.test"
	res4, err := EvaluateYqExpression(res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res4)
	err = PrintAsYAML(res4)
	assert.Nil(t, err)
}
