package installer

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/schema"
)

var errToolNotFoundInMock = errors.New("tool not found in mock resolver")

type mockToolResolver struct {
	mapping map[string][2]string // toolName -> [owner, repo]
}

func (m *mockToolResolver) Resolve(toolName string) (string, string, error) {
	if val, ok := m.mapping[toolName]; ok {
		return val[0], val[1], nil
	}
	return "", "", errToolNotFoundInMock
}

// SetAtmosConfig is a test helper - in production this is set by the toolchain package.
func SetAtmosConfig(_ *schema.AtmosConfiguration) {
	// No-op for tests - the installer doesn't use this directly.
}
