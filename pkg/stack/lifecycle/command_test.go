package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestHandlerArgumentsRejectInvalidInvocations(t *testing.T) {
	node := &dependency.Node{Component: "api", Stack: "dev"}
	declared := errors.New("declaration failed")
	for _, tc := range []struct {
		name    string
		handler func() *Handler
		node    *dependency.Node
		wantIs  error
		want    string
	}{
		{"nil handler", func() *Handler { return nil }, node, errUtils.ErrNilParam, ""},
		{"nil node", func() *Handler { return NativeHandler("terraform", "deploy") }, nil, errUtils.ErrNilParam, ""},
		{"retained declaration error", func() *Handler { return &Handler{Path: []string{"widget", "deploy"}, Err: declared} }, node, declared, ""},
		{"non-string semantic flag", func() *Handler {
			h := NativeHandler("terraform", "deploy")
			h.Flags[0].Type = "bool"
			return h
		}, node, errUtils.ErrInvalidConfig, "semantic flag --stack must be a string"},
		{"disallowed flag value", func() *Handler {
			h := NativeHandler("terraform", "deploy")
			h.Flags[0].Values = []string{"prod"}
			return h
		}, node, errUtils.ErrInvalidConfig, `stack "dev" is not an allowed value`},
		{"unsupported semantic input", func() *Handler {
			h := NativeHandler("terraform", "deploy")
			h.Flags = append(h.Flags, schema.CommandFlag{Name: "region", Provides: "region"})
			return h
		}, node, errUtils.ErrInvalidConfig, "unsupported semantic input region"},
		{
			"positional starting with dash", func() *Handler { return NativeHandler("terraform", "deploy") },
			&dependency.Node{Component: "-evil", Stack: "dev"}, errUtils.ErrInvalidConfig, "positional argument component starts with '-'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, err := tc.handler().Arguments(tc.node)
			require.ErrorIs(t, err, tc.wantIs)
			assert.Nil(t, args)
			if tc.want != "" {
				assert.Contains(t, err.Error(), tc.want)
			}
		})
	}
}

func TestHandlerArgumentsTrimTrailingEmptyPositionals(t *testing.T) {
	node := &dependency.Node{Component: "api", Stack: "dev"}
	h := NativeHandler("terraform", "deploy")
	h.Args = append(h.Args, schema.CommandArgument{Name: "extra"}, schema.CommandArgument{Name: "tail"})

	args, err := h.Arguments(node)
	require.NoError(t, err)
	// Optional trailing arguments without a default are omitted instead of passed as empty strings.
	assert.Equal(t, []string{"terraform", "deploy", "--stack=dev", "api"}, args)

	// An empty argument followed by a value stays in place so positions are preserved.
	h.Args[2].Default = "x"
	args, err = h.Arguments(node)
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "deploy", "--stack=dev", "api", "", "x"}, args)
}
