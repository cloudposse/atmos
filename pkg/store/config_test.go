package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestStoreConfig_YAML_OmitsEmptyType(t *testing.T) {
	cfg := StoreConfig{
		Kind:     "aws/ssm",
		Identity: "prod-admin",
		Options: map[string]interface{}{
			"prefix": "/atmos/prod",
		},
	}

	out, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.NotContains(t, string(out), "type:")
	assert.Contains(t, string(out), "kind: aws/ssm")
}

func TestStoreConfig_YAML_IncludesLegacyType(t *testing.T) {
	cfg := StoreConfig{
		Type: "aws-ssm-parameter-store",
	}

	out, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.Contains(t, string(out), "type: aws-ssm-parameter-store")
}
