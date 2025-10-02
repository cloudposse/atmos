package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetStringFlagIfChanged(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		setValue string
		setFlag  bool
		expected string
		wantErr  bool
	}{
		{
			name:     "flag changed - updates target",
			flagName: "test-flag",
			setValue: "new-value",
			setFlag:  true,
			expected: "new-value",
			wantErr:  false,
		},
		{
			name:     "flag not changed - keeps original",
			flagName: "test-flag",
			setValue: "new-value",
			setFlag:  false,
			expected: "original",
			wantErr:  false,
		},
		{
			name:     "non-existent flag",
			flagName: "non-existent",
			setValue: "",
			setFlag:  false,
			expected: "original",
			wantErr:  false, // Changed() returns false for non-existent flags, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("test-flag", "default", "test flag")

			target := "original"

			if tt.setFlag && tt.flagName == "test-flag" {
				err := flags.Set(tt.flagName, tt.setValue)
				require.NoError(t, err)
			}

			err := setStringFlagIfChanged(flags, tt.flagName, &target)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, target)
			}
		})
	}
}

func TestSetBoolFlagIfChanged(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		setValue bool
		setFlag  bool
		expected bool
		wantErr  bool
	}{
		{
			name:     "flag changed to true - updates target",
			flagName: "test-flag",
			setValue: true,
			setFlag:  true,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "flag changed to false - updates target",
			flagName: "test-flag",
			setValue: false,
			setFlag:  true,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "flag not changed - keeps original",
			flagName: "test-flag",
			setValue: true,
			setFlag:  false,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "non-existent flag",
			flagName: "non-existent",
			setValue: false,
			setFlag:  false,
			expected: false,
			wantErr:  false, // Changed() returns false for non-existent flags, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.Bool("test-flag", false, "test flag")

			target := false

			if tt.setFlag && tt.flagName == "test-flag" {
				if tt.setValue {
					err := flags.Set(tt.flagName, "true")
					require.NoError(t, err)
				} else {
					err := flags.Set(tt.flagName, "false")
					require.NoError(t, err)
				}
			}

			err := setBoolFlagIfChanged(flags, tt.flagName, &target)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, target)
			}
		})
	}
}

func TestSetSliceOfStringsFlagIfChanged(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		setValue []string
		setFlag  bool
		expected []string
		wantErr  bool
	}{
		{
			name:     "flag changed - updates target",
			flagName: "test-flag",
			setValue: []string{"val1", "val2"},
			setFlag:  true,
			expected: []string{"val1", "val2"},
			wantErr:  false,
		},
		{
			name:     "flag not changed - keeps original",
			flagName: "test-flag",
			setValue: []string{"val1", "val2"},
			setFlag:  false,
			expected: []string{"original"},
			wantErr:  false,
		},
		{
			name:     "flag changed to empty slice",
			flagName: "test-flag",
			setValue: []string{},
			setFlag:  true,
			expected: []string{},
			wantErr:  false,
		},
		{
			name:     "non-existent flag",
			flagName: "non-existent",
			setValue: []string{},
			setFlag:  false,
			expected: []string{"original"},
			wantErr:  false, // Changed() returns false for non-existent flags, so no error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.StringSlice("test-flag", []string{}, "test flag")

			target := []string{"original"}

			if tt.setFlag && tt.flagName == "test-flag" {
				for _, val := range tt.setValue {
					err := flags.Set(tt.flagName, val)
					require.NoError(t, err)
				}
				// For empty slice, we need to explicitly set it
				if len(tt.setValue) == 0 {
					flags.Lookup(tt.flagName).Changed = true
				}
			}

			err := setSliceOfStringsFlagIfChanged(flags, tt.flagName, &target)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, target)
			}
		})
	}
}
