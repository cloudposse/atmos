package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const sampleLock = `# This file is maintained automatically by "terraform init".
# Manual edits may be lost in future updates.

provider "registry.terraform.io/hashicorp/aws" {
  version     = "5.95.0"
  constraints = ">= 5.0.0"
  hashes = [
    "h1:abcdef==",
    "zh:0123456789",
  ]
}

provider "registry.terraform.io/hashicorp/null" {
  version = "3.2.2"
  hashes  = ["h1:zzz=="]
}

provider "registry.opentofu.org/hashicorp/random" {
  version     = "3.6.0"
  constraints = "~> 3.0"
}
`

func TestParse(t *testing.T) {
	providers, err := Parse([]byte(sampleLock), Name)
	require.NoError(t, err)
	require.Len(t, providers, 3)

	// Assert element contents, not just length: first and last by value.
	assert.Equal(t, Provider{Source: "registry.terraform.io/hashicorp/aws", Version: "5.95.0", Constraints: ">= 5.0.0", Hashes: []string{"h1:abcdef==", "zh:0123456789"}}, providers[0])
	assert.Equal(t, Provider{Source: "registry.opentofu.org/hashicorp/random", Version: "3.6.0", Constraints: "~> 3.0"}, providers[2])
	assert.Equal(t, Provider{Source: "registry.terraform.io/hashicorp/null", Version: "3.2.2", Hashes: []string{"h1:zzz=="}}, providers[1])
}

func TestParseEmpty(t *testing.T) {
	providers, err := Parse([]byte("# empty lock file\n"), Name)
	require.NoError(t, err)
	assert.Empty(t, providers)
}

func TestParseInvalidHCL(t *testing.T) {
	_, err := Parse([]byte(`provider "x" {`), Name)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestParseMissingVersion(t *testing.T) {
	// A provider block without a version attribute is malformed for our purposes.
	_, err := Parse([]byte(`provider "registry.terraform.io/hashicorp/aws" {
  constraints = ">= 5.0.0"
}
`), Name)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestParseFileMissing(t *testing.T) {
	_, err := ParseFile(filepath.Join(t.TempDir(), "nope", Name))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFileNotFound)
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, Name), []byte(sampleLock), 0o600))

	providers, err := ParseDir(dir)
	require.NoError(t, err)
	require.Len(t, providers, 3)
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", providers[0].Source)
}
