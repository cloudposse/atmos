package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringMapFlag_GetName(t *testing.T) {
	flag := &StringMapFlag{Name: "set"}
	assert.Equal(t, "set", flag.GetName())
}

func TestStringMapFlag_GetShorthand(t *testing.T) {
	tests := []struct {
		name      string
		shorthand string
		want      string
	}{
		{
			name:      "with shorthand",
			shorthand: "s",
			want:      "s",
		},
		{
			name:      "without shorthand",
			shorthand: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringMapFlag{Shorthand: tt.shorthand}
			assert.Equal(t, tt.want, flag.GetShorthand())
		})
	}
}

func TestStringMapFlag_GetDescription(t *testing.T) {
	flag := &StringMapFlag{Description: "Set template values"}
	assert.Equal(t, "Set template values", flag.GetDescription())
}

func TestStringMapFlag_GetDefault(t *testing.T) {
	tests := []struct {
		name        string
		default_val map[string]string
		want        interface{}
	}{
		{
			name:        "empty map",
			default_val: map[string]string{},
			want:        map[string]string{},
		},
		{
			name:        "nil map",
			default_val: nil,
			want:        map[string]string(nil),
		},
		{
			name: "single value",
			default_val: map[string]string{
				"key": "value",
			},
			want: map[string]string{
				"key": "value",
			},
		},
		{
			name: "multiple values",
			default_val: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			want: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringMapFlag{Default: tt.default_val}
			assert.Equal(t, tt.want, flag.GetDefault())
		})
	}
}

func TestStringMapFlag_IsRequired(t *testing.T) {
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
			flag := &StringMapFlag{Required: tt.required}
			assert.Equal(t, tt.want, flag.IsRequired())
		})
	}
}

func TestStringMapFlag_GetNoOptDefVal(t *testing.T) {
	// StringMap flags don't support NoOptDefVal.
	flag := &StringMapFlag{}
	assert.Empty(t, flag.GetNoOptDefVal(), "StringMap flags should not use NoOptDefVal")
}

func TestStringMapFlag_GetEnvVars(t *testing.T) {
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
			envVars: []string{"ATMOS_SET"},
			want:    []string{"ATMOS_SET"},
		},
		{
			name:    "multiple env vars",
			envVars: []string{"ATMOS_SET", "SET"},
			want:    []string{"ATMOS_SET", "SET"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := &StringMapFlag{EnvVars: tt.envVars}
			assert.Equal(t, tt.want, flag.GetEnvVars())
		})
	}
}

func TestStringMapFlag_Interface(t *testing.T) {
	// Test that StringMapFlag implements Flag interface.
	var _ Flag = &StringMapFlag{}

	flag := &StringMapFlag{
		Name:        "set",
		Shorthand:   "s",
		Default:     map[string]string{"default_key": "default_value"},
		Description: "Set template values",
		Required:    false,
		EnvVars:     []string{"ATMOS_SET"},
	}

	// Test all interface methods.
	assert.Equal(t, "set", flag.GetName())
	assert.Equal(t, "s", flag.GetShorthand())
	assert.Equal(t, "Set template values", flag.GetDescription())
	assert.Equal(t, map[string]string{"default_key": "default_value"}, flag.GetDefault())
	assert.False(t, flag.IsRequired())
	assert.Empty(t, flag.GetNoOptDefVal())
	assert.Equal(t, []string{"ATMOS_SET"}, flag.GetEnvVars())
}

func TestStringMapFlag_UsageScenarios(t *testing.T) {
	tests := []struct {
		name        string
		flag        *StringMapFlag
		description string
	}{
		{
			name: "template values flag",
			flag: &StringMapFlag{
				Name:        "set",
				Shorthand:   "",
				Default:     map[string]string{},
				Description: "Set template values (key=value)",
				EnvVars:     []string{"ATMOS_SET"},
			},
			description: "User can provide multiple values: --set foo=bar --set baz=qux",
		},
		{
			name: "config overrides flag",
			flag: &StringMapFlag{
				Name:        "override",
				Shorthand:   "o",
				Default:     map[string]string{},
				Description: "Override configuration values",
				EnvVars:     []string{"ATMOS_OVERRIDE"},
			},
			description: "User can override config: --override key1=val1 --override key2=val2",
		},
		{
			name: "required map flag",
			flag: &StringMapFlag{
				Name:        "vars",
				Shorthand:   "v",
				Default:     map[string]string{},
				Description: "Required variables",
				Required:    true,
			},
			description: "Required flag that must have at least one value",
		},
		{
			name: "map flag with defaults",
			flag: &StringMapFlag{
				Name:        "defaults",
				Shorthand:   "",
				Default:     map[string]string{"env": "dev", "region": "us-east-1"},
				Description: "Values with defaults",
			},
			description: "Map flag with default values",
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

func TestStringMapFlag_EmptyDefault(t *testing.T) {
	// Test zero value behavior.
	var flag StringMapFlag

	assert.Empty(t, flag.Name)
	assert.Empty(t, flag.Shorthand)
	assert.Nil(t, flag.Default)
	assert.Empty(t, flag.Description)
	assert.False(t, flag.Required)
	assert.Nil(t, flag.EnvVars)
}
