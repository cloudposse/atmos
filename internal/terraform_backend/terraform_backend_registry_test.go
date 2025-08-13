package terraform_backend_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestGetTerraformBackendReadFunc(t *testing.T) {
	tb.RegisterTerraformBackends()

	assert.NotNil(t, tb.GetTerraformBackendReadFunc(cfg.BackendTypeLocal))
	assert.NotNil(t, tb.GetTerraformBackendReadFunc(cfg.BackendTypeS3))
}
