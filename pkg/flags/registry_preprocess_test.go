package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestFlagRegistry_PreprocessNoOptDefValArgs(t *testing.T) {
	tests := []struct {
		name     string
		flags    []Flag
		input    []string
		expected []string
	}{
		{
			name: "identity flag with space syntax → equals syntax",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"--identity", "prod", "plan"},
			expected: []string{"--identity=prod", "plan"},
		},
		{
			name: "identity shorthand with space syntax → equals syntax",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"-i", "prod", "plan"},
			expected: []string{"-i=prod", "plan"},
		},
		{
			name: "identity flag with equals syntax → unchanged",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"--identity=prod", "plan"},
			expected: []string{"--identity=prod", "plan"},
		},
		{
			name: "identity flag with value (not a flag) → rewritten to equals syntax",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"--identity", "plan"},
			expected: []string{"--identity=plan"}, // "plan" is treated as the identity value, not a command
		},
		{
			name: "identity flag followed by another flag → unchanged",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"--identity", "--dry-run"},
			expected: []string{"--identity", "--dry-run"},
		},
		{
			name: "identity at end of args → unchanged",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
			},
			input:    []string{"plan", "--identity"},
			expected: []string{"plan", "--identity"},
		},
		{
			name: "multiple NoOptDefVal flags",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
				&StringFlag{
					Name:        "pager",
					Shorthand:   "p",
					NoOptDefVal: "true",
				},
			},
			input:    []string{"--identity", "prod", "--pager", "less", "plan"},
			expected: []string{"--identity=prod", "--pager=less", "plan"},
		},
		{
			name: "flag without NoOptDefVal → unchanged",
			flags: []Flag{
				&StringFlag{
					Name:        "stack",
					Shorthand:   "s",
					NoOptDefVal: "", // No NoOptDefVal
				},
			},
			input:    []string{"--stack", "dev", "plan"},
			expected: []string{"--stack", "dev", "plan"},
		},
		{
			name: "mixed NoOptDefVal and regular flags",
			flags: []Flag{
				&StringFlag{
					Name:        "identity",
					Shorthand:   "i",
					NoOptDefVal: cfg.IdentityFlagSelectValue,
				},
				&StringFlag{
					Name:        "stack",
					Shorthand:   "s",
					NoOptDefVal: "", // No NoOptDefVal
				},
			},
			input:    []string{"--identity", "prod", "--stack", "dev", "plan"},
			expected: []string{"--identity=prod", "--stack", "dev", "plan"},
		},
		{
			name:     "empty registry → unchanged",
			flags:    []Flag{},
			input:    []string{"--identity", "prod", "plan"},
			expected: []string{"--identity", "prod", "plan"},
		},
		{
			name:     "empty args → unchanged",
			flags:    []Flag{},
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewFlagRegistry()
			for _, flag := range tt.flags {
				registry.Register(flag)
			}

			result := registry.PreprocessNoOptDefValArgs(tt.input)

			assert.Equal(t, tt.expected, result, "preprocessed args should match expected")
		})
	}
}

func TestFlagRegistry_PreprocessNoOptDefValArgs_EdgeCases(t *testing.T) {
	t.Run("flag with value containing equals", func(t *testing.T) {
		registry := NewFlagRegistry()
		registry.Register(&StringFlag{
			Name:        "identity",
			Shorthand:   "i",
			NoOptDefVal: cfg.IdentityFlagSelectValue,
		})

		input := []string{"--identity", "prod=value", "plan"}
		expected := []string{"--identity=prod=value", "plan"}

		result := registry.PreprocessNoOptDefValArgs(input)
		assert.Equal(t, expected, result)
	})

	t.Run("flag with value containing spaces (single arg)", func(t *testing.T) {
		registry := NewFlagRegistry()
		registry.Register(&StringFlag{
			Name:        "identity",
			Shorthand:   "i",
			NoOptDefVal: cfg.IdentityFlagSelectValue,
		})

		input := []string{"--identity", "prod user", "plan"}
		expected := []string{"--identity=prod user", "plan"}

		result := registry.PreprocessNoOptDefValArgs(input)
		assert.Equal(t, expected, result)
	})

	t.Run("double dash prefix", func(t *testing.T) {
		registry := NewFlagRegistry()
		registry.Register(&StringFlag{
			Name:        "identity",
			Shorthand:   "i",
			NoOptDefVal: cfg.IdentityFlagSelectValue,
		})

		input := []string{"--identity", "--", "plan"}
		expected := []string{"--identity", "--", "plan"}

		result := registry.PreprocessNoOptDefValArgs(input)
		assert.Equal(t, expected, result, "-- should not be consumed as flag value")
	})
}
