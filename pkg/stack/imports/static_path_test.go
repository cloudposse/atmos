package imports

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateStaticImportPath(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		delimiters []string
		wantErr    error
	}{
		{"local", "catalog/auth", nil, nil},
		{"default expression", "{{ .vars.path }}", nil, errUtils.ErrInvalidImport},
		{"custom expression", "[[ .vars.path ]]", []string{"[[", "]]"}, errUtils.ErrInvalidImport},
		{"invalid settings", "catalog/auth", []string{"[["}, errUtils.ErrInvalidTemplateSettings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Templates.Settings.Delimiters = tc.delimiters
			err := ValidateStaticImportPath(config, tc.path, "dev.yaml")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorContains(t, err, "dev.yaml")
			} else {
				require.NoError(t, err)
			}
		})
	}
	require.NoError(t, ValidateStaticImportPath(nil, "catalog/auth", "dev.yaml"))
}
