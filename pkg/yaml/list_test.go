package yaml

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListPathEntries(t *testing.T) {
	input := []byte(`
components:
  terraform:
    "vpc.prod":
      enabled: true
      vars:
        cidr: 10.0.0.0/16
        azs:
          - us-east-1a
          - us-east-1b
        replicas: 2
        threshold: 2.5
        deleted: null
        foo"bar: value
        foo\bar: value
`)

	entries, err := ListPathEntries(input)
	require.NoError(t, err)

	require.Equal(t, []PathEntry{
		{Path: "components", Type: "object", Value: "{1 keys}"},
		{Path: "components.terraform", Type: "object", Value: "{1 keys}"},
		{Path: `components.terraform."vpc.prod"`, Type: "object", Value: "{2 keys}"},
		{Path: `components.terraform."vpc.prod".enabled`, Type: "bool", Value: "true"},
		{Path: `components.terraform."vpc.prod".vars`, Type: "object", Value: "{7 keys}"},
		{Path: `components.terraform."vpc.prod".vars."foo\"bar"`, Type: "string", Value: "value"},
		{Path: `components.terraform."vpc.prod".vars."foo\\bar"`, Type: "string", Value: "value"},
		{Path: `components.terraform."vpc.prod".vars.azs`, Type: "array", Value: "[2 items]"},
		{Path: `components.terraform."vpc.prod".vars.azs[0]`, Type: "string", Value: "us-east-1a"},
		{Path: `components.terraform."vpc.prod".vars.azs[1]`, Type: "string", Value: "us-east-1b"},
		{Path: `components.terraform."vpc.prod".vars.cidr`, Type: "string", Value: "10.0.0.0/16"},
		{Path: `components.terraform."vpc.prod".vars.deleted`, Type: "null", Value: "null"},
		{Path: `components.terraform."vpc.prod".vars.replicas`, Type: "number", Value: "2"},
		{Path: `components.terraform."vpc.prod".vars.threshold`, Type: "number", Value: "2.5"},
	}, entries)
}

func TestListPathEntriesInvalidYAML(t *testing.T) {
	_, err := ListPathEntries([]byte("invalid: ["))
	require.ErrorIs(t, err, ErrInvalidYAMLExpression)
}
