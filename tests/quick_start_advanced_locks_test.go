package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestQuickStartAdvancedInstanceLocks keeps every newcomer-facing deployment
// target pinned before its first init. Workdir components restore these files as
// .terraform.lock.hcl, avoiding OpenTofu's host-only checksum warning when the
// registry cache is enabled.
func TestQuickStartAdvancedInstanceLocks(t *testing.T) {
	components := []string{
		"app-config",
		"dynamodb-table",
		"kms-key",
		"s3-bucket",
		"sns-topic",
		"sqs-queue",
	}
	stacks := []string{
		"plat-ue2-dev",
		"plat-ue2-staging",
		"plat-ue2-prod",
		"plat-uw2-dev",
		"plat-uw2-staging",
		"plat-uw2-prod",
	}

	for _, component := range components {
		for _, stack := range stacks {
			t.Run(stack+"/"+component, func(t *testing.T) {
				path := filepath.Join(
					repoRoot,
					"examples", "quick-start-advanced", "components", "terraform", component,
					"."+stack+"-"+component+".terraform.lock.hcl",
				)
				data, err := os.ReadFile(path)
				require.NoErrorf(t, err, "missing committed per-instance lock %s", path)

				lock := string(data)
				require.Contains(t, lock, `provider "registry.opentofu.org/hashicorp/aws"`)
				// `tofu providers lock` writes one h1 checksum per requested platform.
				// Advanced Quick Start declares darwin_arm64 and linux_amd64.
				require.GreaterOrEqualf(t, strings.Count(lock, `"h1:`), 2,
					"%s must include checksums for both configured platforms", path)
			})
		}
	}
}
