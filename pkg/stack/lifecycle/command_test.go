package lifecycle

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemanticCommandBindings(t *testing.T) {
	n := &dependency.Node{Component: "api with spaces", Stack: "dev"}
	c := &Handler{Path: []string{"tools", "widget", "deploy"}, Args: []schema.CommandArgument{{Name: "environment", Provides: "stack"}, {Name: "target", Provides: "component"}}, Flags: []schema.CommandFlag{{Name: "region", Required: true, Default: "west"}}}
	args, err := c.Arguments(n)
	require.NoError(t, err)
	assert.Equal(t, []string{"tools", "widget", "deploy", "--region=west", "dev", "api with spaces"}, args)
	c.Flags = append(c.Flags, schema.CommandFlag{Name: "duplicate", Provides: "component"})
	_, err = c.Arguments(n)
	require.ErrorContains(t, err, "exactly one")
	c.Flags = []schema.CommandFlag{{Name: "token", Required: true}}
	_, err = c.Arguments(n)
	require.ErrorContains(t, err, "required flag")
	c.Flags = nil
	c.Args = append(c.Args, schema.CommandArgument{Name: "token", Required: true})
	_, err = c.Arguments(n)
	require.ErrorContains(t, err, "required argument")
	c = NativeHandler("terraform", "deploy")
	args, err = c.Arguments(n)
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "deploy", "--stack=dev", "api with spaces"}, args)
	c.Args[0].Values = []string{"other"}
	_, err = c.Arguments(n)
	require.ErrorContains(t, err, "allowed value")
}
