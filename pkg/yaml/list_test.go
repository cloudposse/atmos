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
		{Path: "components", Type: "object"},
		{Path: "components.terraform", Type: "object"},
		{Path: `components.terraform."vpc.prod"`, Type: "object"},
		{Path: `components.terraform."vpc.prod".enabled`, Type: "bool"},
		{Path: `components.terraform."vpc.prod".vars`, Type: "object"},
		{Path: `components.terraform."vpc.prod".vars."foo\"bar"`, Type: "string"},
		{Path: `components.terraform."vpc.prod".vars."foo\\bar"`, Type: "string"},
		{Path: `components.terraform."vpc.prod".vars.azs`, Type: "array"},
		{Path: `components.terraform."vpc.prod".vars.azs[0]`, Type: "string"},
		{Path: `components.terraform."vpc.prod".vars.azs[1]`, Type: "string"},
		{Path: `components.terraform."vpc.prod".vars.cidr`, Type: "string"},
		{Path: `components.terraform."vpc.prod".vars.deleted`, Type: "null"},
		{Path: `components.terraform."vpc.prod".vars.replicas`, Type: "number"},
		{Path: `components.terraform."vpc.prod".vars.threshold`, Type: "number"},
	}, entries)
}

func TestListPathEntriesInvalidYAML(t *testing.T) {
	_, err := ListPathEntries([]byte("invalid: ["))
	require.ErrorIs(t, err, ErrInvalidYAMLExpression)
}
