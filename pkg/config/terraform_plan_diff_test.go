package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadConfigPlanDiff(t *testing.T) {
	for _, entry := range []string{"aws_db_instance.latest_restorable_time", "aws_db_instance.*"} {
		t.Run(entry, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "atmos.yaml")
			content := "base_path: ./\ncomponents:\n  terraform:\n    plan_diff:\n      ignore_changes:\n        - " + entry + "\n"
			require.NoError(t, os.WriteFile(configPath, []byte(content), 0o600))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{configPath}})
			if entry == "aws_db_instance.*" {
				assert.ErrorIs(t, err, schema.ErrInvalidPlanDiffIgnoreChange)
				hints := cockroachErrors.GetAllHints(err)
				require.Len(t, hints, 1)
				assert.Contains(t, strings.Join(hints, "\n"), "resource addresses")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{entry}, config.Components.Terraform.PlanDiff.IgnoreChanges)
		})
	}
}
