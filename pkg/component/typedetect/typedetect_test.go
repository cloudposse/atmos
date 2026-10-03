package typedetect

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

var errProbe = errors.New("probe failed")

// probeFrom builds a probe from a set of types that define the component, optionally failing
// for one type. It records every type it was asked about.
func probeFrom(defined map[string]bool, failOn string, asked *[]string) Probe {
	return func(componentType string) (bool, error) {
		*asked = append(*asked, componentType)
		if componentType == failOn {
			return false, errProbe
		}
		return defined[componentType], nil
	}
}

func TestResolve(t *testing.T) {
	types := []string{"terraform", "helmfile", "aws/cloudformation"}

	tests := []struct {
		name       string
		defined    map[string]bool
		failOn     string
		wantType   string
		wantErr    error
		wantAsked  []string
		wantDetail string
	}{
		{
			name:      "first type defines it",
			defined:   map[string]bool{"terraform": true},
			wantType:  "terraform",
			wantAsked: types,
		},
		{
			name:      "last type defines it",
			defined:   map[string]bool{"aws/cloudformation": true},
			wantType:  "aws/cloudformation",
			wantAsked: types,
		},
		{
			name:      "no type defines it",
			defined:   map[string]bool{},
			wantType:  "",
			wantAsked: types,
		},
		{
			name:       "two types define it",
			defined:    map[string]bool{"terraform": true, "aws/cloudformation": true},
			wantErr:    errUtils.ErrDuplicateComponentConfig,
			wantAsked:  types,
			wantDetail: "terraform, aws/cloudformation",
		},
		{
			name:      "probe error is returned and stops detection",
			defined:   map[string]bool{"aws/cloudformation": true},
			failOn:    "helmfile",
			wantErr:   errProbe,
			wantAsked: []string{"terraform", "helmfile"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked []string
			got, err := Resolve("app", "dev", types, probeFrom(tt.defined, tt.failOn, &asked))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				if tt.wantDetail != "" {
					assert.Contains(t, err.Error(), tt.wantDetail)
					assert.Contains(t, err.Error(), "`app`")
					assert.Contains(t, err.Error(), "`dev`")
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantType, got)
			}
			assert.Equal(t, tt.wantAsked, asked)
		})
	}
}

// A duplicate must not be mistaken for a missing component by callers that test for
// ErrInvalidComponent.
func TestResolve_DuplicateIsNotInvalidComponent(t *testing.T) {
	var asked []string
	_, err := Resolve("app", "dev", []string{"a", "b"}, probeFrom(map[string]bool{"a": true, "b": true}, "", &asked))
	require.ErrorIs(t, err, errUtils.ErrDuplicateComponentConfig)
	assert.NotErrorIs(t, err, errUtils.ErrInvalidComponent)
}
