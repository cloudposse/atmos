package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
