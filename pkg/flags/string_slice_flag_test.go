package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSliceFlag_GetName(t *testing.T) {
	flag := &StringSliceFlag{Name: "config"}
	assert.Equal(t, "config", flag.GetName())
}

func TestStringSliceFlag_GetShorthand(t *testing.T) {
	tests := []struct {
		name      string
		shorthand string
		want      string
	}{
		{
			name:      "with shorthand",
			shorthand: "c",
			want:      "c",
		},
		{
			name:      "without shorthand",
			shorthand: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringSliceFlag{Shorthand: tt.shorthand}
			assert.Equal(t, tt.want, flag.GetShorthand())
		})
	}
}

func TestStringSliceFlag_GetDescription(t *testing.T) {
	flag := &StringSliceFlag{Description: "Config file paths"}
	assert.Equal(t, "Config file paths", flag.GetDescription())
}

func TestStringSliceFlag_GetDefault(t *testing.T) {
	tests := []struct {
		name        string
		default_val []string
		want        interface{}
	}{
		{
			name:        "empty slice",
			default_val: []string{},
			want:        []string{},
		},
		{
			name:        "nil slice",
			default_val: nil,
			want:        []string(nil), // nil slice of type []string
		},
		{
			name:        "single value",
			default_val: []string{"config.yaml"},
			want:        []string{"config.yaml"},
		},
		{
			name:        "multiple values",
			default_val: []string{"config1.yaml", "config2.yaml"},
			want:        []string{"config1.yaml", "config2.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringSliceFlag{Default: tt.default_val}
			assert.Equal(t, tt.want, flag.GetDefault())
		})
	}
}

func TestStringSliceFlag_IsRequired(t *testing.T) {
	tests := []struct {
		name     string
		required bool
		want     bool
	}{
		{
			name:     "required flag",
			required: true,
			want:     true,
		},
		{
			name:     "optional flag",
			required: false,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringSliceFlag{Required: tt.required}
			assert.Equal(t, tt.want, flag.IsRequired())
		})
	}
}

func TestStringSliceFlag_GetNoOptDefVal(t *testing.T) {
	// StringSlice flags don't support NoOptDefVal.
	flag := &StringSliceFlag{}
	assert.Empty(t, flag.GetNoOptDefVal(), "StringSlice flags should not use NoOptDefVal")
}

func TestStringSliceFlag_GetEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		envVars []string
		want    []string
	}{
		{
			name:    "no env vars",
			envVars: nil,
			want:    nil,
		},
		{
			name:    "empty env vars",
			envVars: []string{},
			want:    []string{},
		},
		{
			name:    "single env var",
			envVars: []string{"ATMOS_CONFIG"},
			want:    []string{"ATMOS_CONFIG"},
		},
		{
			name:    "multiple env vars",
			envVars: []string{"ATMOS_CONFIG", "CONFIG"},
			want:    []string{"ATMOS_CONFIG", "CONFIG"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringSliceFlag{EnvVars: tt.envVars}
			assert.Equal(t, tt.want, flag.GetEnvVars())
		})
	}
}

func TestStringSliceFlag_Interface(t *testing.T) {
	// Test that StringSliceFlag implements Flag interface.
	var _ Flag = &StringSliceFlag{}

	flag := &StringSliceFlag{
		Name:        "config",
		Shorthand:   "c",
		Default:     []string{"default.yaml"},
		Description: "Configuration files",
		Required:    false,
		EnvVars:     []string{"ATMOS_CONFIG"},
	}

	// Test all interface methods.
	assert.Equal(t, "config", flag.GetName())
	assert.Equal(t, "c", flag.GetShorthand())
	assert.Equal(t, "Configuration files", flag.GetDescription())
	assert.Equal(t, []string{"default.yaml"}, flag.GetDefault())
	assert.False(t, flag.IsRequired())
	assert.Empty(t, flag.GetNoOptDefVal())
	assert.Equal(t, []string{"ATMOS_CONFIG"}, flag.GetEnvVars())
}

func TestStringSliceFlag_UsageScenarios(t *testing.T) {
	tests := []struct {
		name        string
		flag        *StringSliceFlag
		description string
	}{
		{
			name: "config files flag",
			flag: &StringSliceFlag{
				Name:        "config",
				Shorthand:   "",
				Default:     []string{},
				Description: "Paths to configuration files",
				EnvVars:     []string{"ATMOS_CONFIG"},
			},
			description: "User can provide multiple config files: --config file1.yaml --config file2.yaml",
		},
		{
			name: "config path flag",
			flag: &StringSliceFlag{
				Name:        "config-path",
				Shorthand:   "",
				Default:     []string{},
				Description: "Paths to configuration directories",
				EnvVars:     []string{"ATMOS_CONFIG_PATH"},
			},
			description: "User can provide multiple config paths: --config-path /path1 --config-path /path2",
		},
		{
			name: "required slice flag",
			flag: &StringSliceFlag{
				Name:        "targets",
				Shorthand:   "t",
				Default:     []string{},
				Description: "Target resources",
				Required:    true,
			},
			description: "Required flag that must have at least one value",
		},
		{
			name: "slice flag with defaults",
			flag: &StringSliceFlag{
				Name:        "include",
				Shorthand:   "",
				Default:     []string{"*.yaml", "*.yml"},
				Description: "File patterns to include",
			},
			description: "Slice flag with default values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Scenario: %s", tt.description)

			// Test that flag can be created and accessed.
			assert.NotNil(t, tt.flag)
			assert.NotEmpty(t, tt.flag.GetName())
			assert.NotEmpty(t, tt.flag.GetDescription())
		})
	}
}

func TestStringSliceFlag_EmptyDefault(t *testing.T) {
	// Test zero value behavior.
	var flag StringSliceFlag

	assert.Empty(t, flag.Name)
	assert.Empty(t, flag.Shorthand)
	assert.Nil(t, flag.Default)
	assert.Empty(t, flag.Description)
	assert.False(t, flag.Required)
	assert.Nil(t, flag.EnvVars)
}

func TestStringSliceFlag_CommaAndRepeat(t *testing.T) {
	// Document the two ways to provide slice values.
	t.Run("repeated flag", func(t *testing.T) {
		// User provides: --config file1.yaml --config file2.yaml
		// Cobra/Viper handles this automatically.
		flag := &StringSliceFlag{
			Name:        "config",
			Description: "Config files (can be repeated)",
		}

		assert.Equal(t, "config", flag.GetName())
		t.Log("Usage: --config file1.yaml --config file2.yaml")
	})

	t.Run("comma-separated", func(t *testing.T) {
		// User provides: --config file1.yaml,file2.yaml
		// Cobra/Viper handles this automatically.
		flag := &StringSliceFlag{
			Name:        "config",
			Description: "Config files (comma-separated)",
		}

		assert.Equal(t, "config", flag.GetName())
		t.Log("Usage: --config file1.yaml,file2.yaml")
	})

	t.Run("mixed", func(t *testing.T) {
		// User provides: --config file1.yaml --config file2.yaml,file3.yaml
		// Cobra/Viper handles this automatically.
		flag := &StringSliceFlag{
			Name:        "config",
			Description: "Config files (both forms work)",
		}

		assert.Equal(t, "config", flag.GetName())
		t.Log("Usage: --config file1.yaml --config file2.yaml,file3.yaml")
		t.Log("Result: []string{\"file1.yaml\", \"file2.yaml\", \"file3.yaml\"}")
	})
}
