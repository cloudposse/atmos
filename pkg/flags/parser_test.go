package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// NOTE: Tests for ToTerraformOptions() were removed.
//
// The ToTerraformOptions() method was deleted from ParsedConfig to avoid circular dependency
// between pkg/flags and pkg/flags/terraform. The terraform-specific flag parsing functionality
// is now tested directly in pkg/flags/terraform/options_test.go via terraform.ParseFlags().
//
// The test coverage that was previously here has been migrated to:
//   - pkg/flags/terraform/options_test.go - Tests terraform.ParseFlags()
//   - pkg/flags/terraform/parser_test.go - Tests terraform.Parser.Parse()
//
// This maintains the same level of test coverage while respecting package boundaries.

var _ = cfg.IdentityFlagSelectValue // Prevent unused import error.
var _ = assert.Equal                // Prevent unused import error.
var _ = testing.T{}                 // Prevent unused import error.
