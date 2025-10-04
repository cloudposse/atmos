package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetAnsiblePlaybookFromSettings(t *testing.T) {
	settings := schema.AtmosSectionMapType{
		cfg.AnsibleSectionName: map[string]any{
			cfg.AnsiblePlaybookSectionName: "site.yml",
		},
	}
	pb, err := GetAnsiblePlaybookFromSettings(&settings)
	assert.NoError(t, err)
	assert.Equal(t, "site.yml", pb)

	empty, err := GetAnsiblePlaybookFromSettings(nil)
	assert.NoError(t, err)
	assert.Equal(t, "", empty)
}

func TestGetAnsibleInventoryFromSettings(t *testing.T) {
	settings := schema.AtmosSectionMapType{
		cfg.AnsibleSectionName: map[string]any{
			cfg.AnsibleInventorySectionName: "inventory",
		},
	}
	inv, err := GetAnsibleInventoryFromSettings(&settings)
	assert.NoError(t, err)
	assert.Equal(t, "inventory", inv)

	empty, err := GetAnsibleInventoryFromSettings(nil)
	assert.NoError(t, err)
	assert.Equal(t, "", empty)
}
