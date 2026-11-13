package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
)

// parseTfvars parses generated content the same way Terraform reads a .tfvars file.
func parseTfvars(t *testing.T, content []byte) map[string]cty.Value {
	t.Helper()

	file, diags := hclsyntax.ParseConfig(content, "test.tfvars", hcl.InitialPos)
	require.False(t, diags.HasErrors(), "generated HCL does not parse: %s\n%s", diags.Error(), content)

	attrs, diags := file.Body.JustAttributes()
	require.False(t, diags.HasErrors(), "generated HCL is not a flat list of attributes: %s\n%s", diags.Error(), content)

	values := make(map[string]cty.Value, len(attrs))
	for name, attr := range attrs {
		value, diags := attr.Expr.Value(nil)
		require.False(t, diags.HasErrors(), "attribute %q: %s", name, diags.Error())
		values[name] = value
	}
	return values
}

func TestWriteToFileAsHclNestedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.tfvars")

	err := WriteToFileAsHcl(path, map[string]any{
		"enabled": true,
		"mymap": map[string]any{
			"somekey": map[string]any{
				"somevalue": true,
			},
		},
		"subnets": []any{
			map[string]any{"name": "a", "cidr": "10.0.0.0/24"},
			map[string]any{"name": "b", "cidr": "10.0.1.0/24"},
		},
		"tags":     map[string]any{"Name": "vpc", "kubernetes.io/role/elb": "1"},
		"template": "${not_interpolated}",
		"nothing":  nil,
	}, 0o644)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	values := parseTfvars(t, content)

	assert.Equal(t, cty.True, values["enabled"])
	assert.Equal(t, cty.True, values["mymap"].GetAttr("somekey").GetAttr("somevalue"))

	subnets := values["subnets"]
	require.Equal(t, 2, subnets.LengthInt())
	assert.Equal(t, "a", subnets.Index(cty.NumberIntVal(0)).GetAttr("name").AsString())
	assert.Equal(t, "10.0.1.0/24", subnets.Index(cty.NumberIntVal(1)).GetAttr("cidr").AsString())

	assert.Equal(t, "1", values["tags"].GetAttr("kubernetes.io/role/elb").AsString())
	assert.Equal(t, "${not_interpolated}", values["template"].AsString())
	assert.True(t, values["nothing"].IsNull())
}

func TestWriteToFileAsHclTruncatesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.tfvars")

	require.NoError(t, WriteToFileAsHcl(path, map[string]any{
		"enabled":     true,
		"environment": "ue2",
		"description": "a long description that makes the first file noticeably longer",
	}, 0o644))

	require.NoError(t, WriteToFileAsHcl(path, map[string]any{"enabled": false}, 0o644))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "enabled = false\n", string(got))
}

func TestConvertToHclAttributesRejectsNonMap(t *testing.T) {
	_, err := ConvertToHclAttributes([]any{"a", "b"})
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}
