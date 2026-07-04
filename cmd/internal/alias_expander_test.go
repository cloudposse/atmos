package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAliasExpanderExpand(t *testing.T) {
	tests := []struct {
		name    string
		aliases schema.CommandAliases
		args    []string
		want    []string
	}{
		{
			name: "shortcut alias",
			aliases: schema.CommandAliases{
				"tp": "terraform plan",
			},
			args: []string{"tp", "vpc"},
			want: []string{"terraform", "plan", "vpc"},
		},
		{
			name: "shortcut chains into same-name default alias",
			aliases: schema.CommandAliases{
				"tf":        "terraform",
				"terraform": "terraform --identity=false",
			},
			args: []string{"tf", "plan", "vpc"},
			want: []string{"terraform", "--identity=false", "plan", "vpc"},
		},
		{
			name: "same-name default alias preserves CLI last-wins order",
			aliases: schema.CommandAliases{
				"terraform": "terraform --identity=false",
			},
			args: []string{"terraform", "plan", "--identity=true"},
			want: []string{"terraform", "--identity=false", "plan", "--identity=true"},
		},
		{
			name: "subcommand alias inserts defaults before user args",
			aliases: schema.CommandAliases{
				"terraform apply": "terraform apply -auto-approve",
			},
			args: []string{"terraform", "apply", "vpc", "-s", "dev"},
			want: []string{"terraform", "apply", "-auto-approve", "vpc", "-s", "dev"},
		},
		{
			name: "longest matching command path wins",
			aliases: schema.CommandAliases{
				"describe":           "list stacks",
				"describe component": "describe component --process-functions=false",
			},
			args: []string{"describe", "component", "vpc"},
			want: []string{"describe", "component", "--process-functions=false", "vpc"},
		},
		{
			name: "quoted alias target values",
			aliases: schema.CommandAliases{
				"say": `custom "hello world"`,
			},
			args: []string{"say", "again"},
			want: []string{"custom", "hello world", "again"},
		},
		{
			name: "invalid target expands and remains a normal command-resolution problem",
			aliases: schema.CommandAliases{
				"terraform apply": "terraform applly -auto-approve",
			},
			args: []string{"terraform", "apply", "vpc"},
			want: []string{"terraform", "applly", "-auto-approve", "vpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expander, err := NewAliasExpander(tt.aliases)
			require.NoError(t, err)

			got, err := expander.Expand(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAliasExpanderEnvSuppressesDefaultAliasFlags(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "true")

	expander, err := NewAliasExpander(schema.CommandAliases{
		"terraform": "terraform --identity=false plan",
	})
	require.NoError(t, err)

	got, err := expander.Expand([]string{"terraform", "apply"})
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "plan", "apply"}, got)
}

func TestAliasExpanderEnvSuppressesSeparatedDefaultValue(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "true")

	expander, err := NewAliasExpander(schema.CommandAliases{
		"terraform": "terraform --identity default",
	})
	require.NoError(t, err)

	got, err := expander.Expand([]string{"terraform", "plan"})
	require.NoError(t, err)
	assert.Equal(t, []string{"terraform", "plan"}, got)
}

func TestAliasExpanderDetectsCycles(t *testing.T) {
	expander, err := NewAliasExpander(schema.CommandAliases{
		"a": "b",
		"b": "a",
	})
	require.NoError(t, err)

	_, err = expander.Expand([]string{"a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alias cycle")
}

func TestAliasExpanderRejectsInvalidAliasSyntax(t *testing.T) {
	_, err := NewAliasExpander(schema.CommandAliases{
		"bad": `"unterminated`,
	})
	require.Error(t, err)
}
