package hcl

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureWithComments = `# Header comment.
resource "aws_instance" "web" {
  # AMI used for this instance.
  ami           = "ami-123456"
  instance_type = "t2.micro" # inline comment

  tags = {
    Name = "web"
  }
}

variable "region" {
  description = "AWS region"
  default     = "us-east-1"
}
`

const fixtureMultipleResources = `resource "aws_instance" "a" {
  instance_type = "t2.micro"
}

resource "aws_instance" "b" {
  instance_type = "t2.micro"
}
`

func TestGet_Attribute(t *testing.T) {
	value, err := Get([]byte(fixtureWithComments), "resource.aws_instance.web.instance_type", false)
	require.NoError(t, err)
	assert.Equal(t, `"t2.micro"`, value)
}

func TestGet_Attribute_WithComments(t *testing.T) {
	without, err := Get([]byte(fixtureWithComments), "resource.aws_instance.web.instance_type", false)
	require.NoError(t, err)
	assert.NotContains(t, without, "inline comment")

	with, err := Get([]byte(fixtureWithComments), "resource.aws_instance.web.instance_type", true)
	require.NoError(t, err)
	assert.Contains(t, with, "inline comment")
}

func TestGet_Block_Fallback(t *testing.T) {
	value, err := Get([]byte(fixtureWithComments), "resource.aws_instance.web", false)
	require.NoError(t, err)
	assert.Contains(t, value, `resource "aws_instance" "web"`)
	assert.Contains(t, value, "ami-123456")
	assert.Contains(t, value, "t2.micro")
}

func TestGet_NotFound(t *testing.T) {
	_, err := Get([]byte(fixtureWithComments), "resource.aws_instance.nonexistent.foo", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHCLAddressNotFound)
}

func TestSetAttribute_PreservesComments(t *testing.T) {
	out, err := SetAttribute([]byte(fixtureWithComments), "resource.aws_instance.web.instance_type", `"t3.micro"`)
	require.NoError(t, err)
	s := string(out)

	assert.Contains(t, s, "# Header comment.", "header comment preserved")
	assert.Contains(t, s, "# AMI used for this instance.", "head comment preserved")
	assert.Contains(t, s, "# inline comment", "inline comment preserved")
	assert.Contains(t, s, "t3.micro", "value updated")
	assert.NotContains(t, s, "t2.micro", "old value removed")
}

func TestSetAttribute_AddressNotFound_IsNoOp(t *testing.T) {
	// hcledit's own semantics: setting a nonexistent attribute silently does
	// nothing rather than erroring or creating it. This is intentional
	// upstream behavior (mirrored from the hcledit CLI), not a bug -- callers
	// that want to create a new attribute must use AppendAttribute instead.
	out, err := SetAttribute([]byte(fixtureWithComments), "resource.aws_instance.web.nonexistent", `"x"`)
	require.NoError(t, err)
	assert.Equal(t, fixtureWithComments, string(out))
}

func TestAppendAttribute_NewAttribute(t *testing.T) {
	out, err := AppendAttribute([]byte(fixtureWithComments), "resource.aws_instance.web.count", "3", true)
	require.NoError(t, err)
	assert.Contains(t, string(out), "count")
}

func TestAppendAttribute_AlreadyExists(t *testing.T) {
	_, err := AppendAttribute([]byte(fixtureWithComments), "resource.aws_instance.web.instance_type", `"t3.micro"`, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHCLUpdateFailed)
}

func TestRemoveAttribute(t *testing.T) {
	out, err := RemoveAttribute([]byte(fixtureWithComments), "variable.region.default")
	require.NoError(t, err)
	assert.NotContains(t, string(out), "us-east-1")
	assert.Contains(t, string(out), "description")
}

func TestNewBlock(t *testing.T) {
	out, err := NewBlock([]byte(fixtureWithComments), "resource.aws_instance.new", true)
	require.NoError(t, err)
	assert.Contains(t, string(out), `resource "aws_instance" "new"`)
}

func TestAppendBlock(t *testing.T) {
	out, err := AppendBlock([]byte(fixtureWithComments), "resource.aws_instance.web", "lifecycle", true)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "lifecycle")

	value, err := Get(out, "resource.aws_instance.web", false)
	require.NoError(t, err)
	assert.Contains(t, value, "lifecycle", "the new block must be nested inside the parent")
}

func TestRemoveBlock_SingleMatch(t *testing.T) {
	out, err := RemoveBlock([]byte(fixtureWithComments), "variable.region")
	require.NoError(t, err)
	s := string(out)
	assert.NotContains(t, s, `variable "region"`)
	assert.Contains(t, s, `resource "aws_instance" "web"`, "sibling block untouched")
}

func TestRemoveBlock_MultipleMatches_RemovesAll(t *testing.T) {
	// hcledit applies block_remove/block_append to EVERY block matching the
	// address, not just the first. A wildcard label ("*") matches any label
	// in that position, so this removes both "a" and "b".
	out, err := RemoveBlock([]byte(fixtureMultipleResources), "resource.aws_instance.*")
	require.NoError(t, err)
	s := string(out)
	assert.NotContains(t, s, `resource "aws_instance" "a"`)
	assert.NotContains(t, s, `resource "aws_instance" "b"`)
}

// TestValidateHCL_RejectsInvalidResult exercises the post-edit validity
// guard directly. Since hcledit's own filters mutate via hclwrite's AST and
// are well-behaved by design, they don't naturally produce invalid HCL
// through the public API -- this test targets the guard itself, which is
// what applyFilter runs after every edit before ever returning bytes a
// caller might persist.
func TestValidateHCL_RejectsInvalidResult(t *testing.T) {
	err := validateHCL([]byte("resource {"), "invalid.tf")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHCLInvalidResult)
}

func TestValidateHCL_AcceptsValidResult(t *testing.T) {
	require.NoError(t, validateHCL([]byte(fixtureWithComments), "main.tf"))
}

func TestSetAttributeFile_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.tf")
	require.NoError(t, os.WriteFile(filePath, []byte(fixtureWithComments), 0o644))

	err := SetAttributeFile(filePath, "resource.aws_instance.web.instance_type", `"t3.micro"`)
	require.NoError(t, err)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "# Header comment.")
	assert.Contains(t, s, "t3.micro")
	assert.NotContains(t, s, "t2.micro")
}

func TestGetFile_ReadOnlyDoesNotModify(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.tf")
	require.NoError(t, os.WriteFile(filePath, []byte(fixtureWithComments), 0o644))

	before, err := os.ReadFile(filePath)
	require.NoError(t, err)

	value, err := GetFile(filePath, "resource.aws_instance.web.instance_type", false)
	require.NoError(t, err)
	assert.Equal(t, `"t2.micro"`, value)

	after, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a read must never modify the file")
}

func TestRemoveBlockFile_NotFoundAfterRemoval(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.tf")
	require.NoError(t, os.WriteFile(filePath, []byte(fixtureWithComments), 0o644))

	require.NoError(t, RemoveBlockFile(filePath, "variable.region"))

	_, err := GetFile(filePath, "variable.region.default", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrHCLAddressNotFound))
}
