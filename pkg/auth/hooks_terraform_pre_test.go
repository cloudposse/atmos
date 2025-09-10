package auth

import (
    "testing"

    "github.com/stretchr/testify/assert"

    "github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformPreHook_NoAuthConfigEarlyExit(t *testing.T) {
    atmosCfg := &schema.AtmosConfiguration{}
    stack := &schema.ConfigAndStacksInfo{ComponentAuthSection: schema.AtmosSectionMapType{}}
    err := TerraformPreHook(atmosCfg, stack)
    assert.NoError(t, err)
}

func TestTerraformPreHook_InvalidAuthConfig(t *testing.T) {
    atmosCfg := &schema.AtmosConfiguration{}
    // Malformed auth section
    stack := &schema.ConfigAndStacksInfo{ComponentAuthSection: schema.AtmosSectionMapType{"providers": 42}}
    err := TerraformPreHook(atmosCfg, stack)
    assert.Error(t, err)
}

