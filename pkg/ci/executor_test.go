package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecute(t *testing.T) {
	t.Run("returns nil when platform not detected and force mode disabled", func(t *testing.T) {
		// Clear any registered providers to ensure no detection.
		providersMu.Lock()
		originalProviders := providers
		providers = make(map[string]Provider)
		providersMu.Unlock()

		defer func() {
			providersMu.Lock()
			providers = originalProviders
			providersMu.Unlock()
		}()

		err := Execute(ExecuteOptions{
			Event:       "after.terraform.plan",
			ForceCIMode: false,
		})
		assert.NoError(t, err)
	})

	t.Run("uses generic provider when force mode enabled and no platform detected", func(t *testing.T) {
		// Clear any registered providers to ensure no detection.
		providersMu.Lock()
		originalProviders := providers
		providers = make(map[string]Provider)
		providersMu.Unlock()

		defer func() {
			providersMu.Lock()
			providers = originalProviders
			providersMu.Unlock()
		}()

		// Force CI mode should use generic provider.
		err := Execute(ExecuteOptions{
			Event:       "after.terraform.plan",
			ForceCIMode: true,
			Info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Stack:            "dev",
			},
		})
		// Should not error - generic provider will handle it.
		assert.NoError(t, err)
	})
}

func TestExtractComponentType(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{"after.terraform.plan", "terraform"},
		{"before.terraform.apply", "terraform"},
		{"after.helmfile.diff", "helmfile"},
		{"invalid", ""},
		{"single", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := extractComponentType(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{"after.terraform.plan", "plan"},
		{"before.terraform.apply", "apply"},
		{"after.helmfile.diff", "diff"},
		{"invalid", ""},
		{"single.part", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := extractCommand(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}
