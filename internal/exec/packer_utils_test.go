package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckPackerConfig(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		wantErr     bool
		errType     error
	}{
		{
			name: "valid packer config with base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing packer base path",
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "",
					},
				},
			},
			wantErr: true,
			errType: errUtils.ErrMissingPackerBasePath,
		},
		{
			name:        "empty atmos config",
			atmosConfig: &schema.AtmosConfiguration{},
			wantErr:     true,
			errType:     errUtils.ErrMissingPackerBasePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkPackerConfig(tt.atmosConfig)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType), "expected error type %v, got %v", tt.errType, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPackerTemplateFromSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings *schema.AtmosSectionMapType
		want     string
		wantErr  bool
	}{
		{
			name: "valid packer template setting",
			settings: &schema.AtmosSectionMapType{
				"packer": map[string]any{
					"template": "example.pkr.hcl",
				},
			},
			want:    "example.pkr.hcl",
			wantErr: false,
		},
		{
			name:     "nil settings",
			settings: nil,
			want:     "",
			wantErr:  false,
		},
		{
			name: "empty packer section",
			settings: &schema.AtmosSectionMapType{
				"packer": map[string]any{},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "packer section without template",
			settings: &schema.AtmosSectionMapType{
				"packer": map[string]any{
					"other_setting": "value",
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "invalid template type",
			settings: &schema.AtmosSectionMapType{
				"packer": map[string]any{
					"template": 123, // invalid type
				},
			},
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetPackerTemplateFromSettings(tt.settings)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPackerManifestFromVars(t *testing.T) {
	tests := []struct {
		name    string
		vars    *schema.AtmosSectionMapType
		want    string
		wantErr bool
	}{
		{
			name: "valid manifest file name",
			vars: &schema.AtmosSectionMapType{
				"manifest_file_name": "manifest.json",
			},
			want:    "manifest.json",
			wantErr: false,
		},
		{
			name:    "nil vars",
			vars:    nil,
			want:    "",
			wantErr: false,
		},
		{
			name:    "empty vars map",
			vars:    &schema.AtmosSectionMapType{},
			want:    "",
			wantErr: false,
		},
		{
			name: "manifest file name not string",
			vars: &schema.AtmosSectionMapType{
				"manifest_file_name": 123,
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "different key present",
			vars: &schema.AtmosSectionMapType{
				"other_key": "manifest.json",
			},
			want:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetPackerManifestFromVars(tt.vars)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
