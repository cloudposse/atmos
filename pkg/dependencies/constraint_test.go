package dependencies

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateConstraint(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		constraint string
		wantErr    bool
		errType    error
	}{
		{
			name:       "latest satisfies any constraint",
			version:    "latest",
			constraint: "~> 1.10.0",
			wantErr:    false,
		},
		{
			name:       "any version satisfies latest constraint",
			version:    "1.5.7",
			constraint: "latest",
			wantErr:    false,
		},
		{
			name:       "empty constraint allows any version",
			version:    "1.10.3",
			constraint: "",
			wantErr:    false,
		},
		{
			name:       "tilde constraint satisfied",
			version:    "1.10.3",
			constraint: "~> 1.10.0",
			wantErr:    false,
		},
		{
			name:       "tilde constraint not satisfied (minor version mismatch)",
			version:    "1.9.8",
			constraint: "~> 1.10.0",
			wantErr:    true,
			errType:    errUtils.ErrDependencyConstraint,
		},
		{
			name:       "caret constraint satisfied",
			version:    "0.54.2",
			constraint: "^0.54.0",
			wantErr:    false,
		},
		{
			name:       "caret constraint not satisfied",
			version:    "0.53.0",
			constraint: "^0.54.0",
			wantErr:    true,
			errType:    errUtils.ErrDependencyConstraint,
		},
		{
			name:       "greater than or equal satisfied",
			version:    "1.10.3",
			constraint: ">= 1.9.0",
			wantErr:    false,
		},
		{
			name:       "greater than or equal not satisfied",
			version:    "1.8.0",
			constraint: ">= 1.9.0",
			wantErr:    true,
			errType:    errUtils.ErrDependencyConstraint,
		},
		{
			name:       "version with v prefix",
			version:    "v1.10.3",
			constraint: "~> 1.10.0",
			wantErr:    false,
		},
		{
			name:       "invalid constraint",
			version:    "1.10.3",
			constraint: "invalid",
			wantErr:    true,
			errType:    errUtils.ErrDependencyConstraint,
		},
		{
			name:       "invalid version",
			version:    "invalid",
			constraint: "~> 1.10.0",
			wantErr:    true,
			errType:    errUtils.ErrDependencyConstraint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConstraint(tt.version, tt.constraint)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType), "expected error type %v, got %v", tt.errType, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMergeDependencies(t *testing.T) {
	tests := []struct {
		name    string
		parent  map[string]string
		child   map[string]string
		want    map[string]string
		wantErr bool
		errType error
	}{
		{
			name:   "empty parent and child",
			parent: map[string]string{},
			child:  map[string]string{},
			want:   map[string]string{},
		},
		{
			name: "child adds new tools",
			parent: map[string]string{
				"terraform": "~> 1.10.0",
			},
			child: map[string]string{
				"kubectl": "^1.32.0",
			},
			want: map[string]string{
				"terraform": "~> 1.10.0",
				"kubectl":   "^1.32.0",
			},
		},
		{
			name: "child overrides parent (satisfies constraint)",
			parent: map[string]string{
				"terraform": "~> 1.10.0",
				"helm":      "^3.17.0",
			},
			child: map[string]string{
				"terraform": "1.10.3", // Satisfies ~> 1.10.0
			},
			want: map[string]string{
				"terraform": "1.10.3",
				"helm":      "^3.17.0",
			},
		},
		{
			name: "child overrides with latest",
			parent: map[string]string{
				"terraform": "~> 1.10.0",
			},
			child: map[string]string{
				"terraform": "latest", // latest always satisfies
			},
			want: map[string]string{
				"terraform": "latest",
			},
		},
		{
			name: "child override violates parent constraint",
			parent: map[string]string{
				"terraform": "~> 1.10.0",
			},
			child: map[string]string{
				"terraform": "1.9.8", // Does not satisfy ~> 1.10.0
			},
			wantErr: true,
			errType: errUtils.ErrDependencyConstraint,
		},
		{
			name: "multiple tools mixed inheritance",
			parent: map[string]string{
				"terraform": "~> 1.10.0",
				"helm":      "^3.17.0",
				"tflint":    "^0.54.0",
			},
			child: map[string]string{
				"terraform": "1.10.3",  // Override (satisfies)
				"kubectl":   "^1.32.0", // Add new
			},
			want: map[string]string{
				"terraform": "1.10.3",
				"helm":      "^3.17.0",
				"tflint":    "^0.54.0",
				"kubectl":   "^1.32.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeDependencies(tt.parent, tt.child)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType), "expected error type %v, got %v", tt.errType, err)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
