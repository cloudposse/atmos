package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestAtmosVendorTargets_UnmarshalYAML_SimpleStrings(t *testing.T) {
	input := `
- "components/terraform/vpc"
- "components/terraform/{{.Component}}/{{.Version}}"
- rds
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.NoError(t, err)

	assert.Len(t, targets, 3)

	assert.Equal(t, "components/terraform/vpc", targets[0].Path)
	assert.Empty(t, targets[0].Version)

	assert.Equal(t, "components/terraform/{{.Component}}/{{.Version}}", targets[1].Path)
	assert.Empty(t, targets[1].Version)

	assert.Equal(t, "rds", targets[2].Path)
	assert.Empty(t, targets[2].Version)
}

func TestAtmosVendorTargets_UnmarshalYAML_MapSyntax(t *testing.T) {
	input := `
- path: "vpc/{{.Version}}"
  version: "2.1.0"
- path: "vpc/latest"
  version: "2.2.0"
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.NoError(t, err)

	assert.Len(t, targets, 2)

	assert.Equal(t, "vpc/{{.Version}}", targets[0].Path)
	assert.Equal(t, "2.1.0", targets[0].Version)

	assert.Equal(t, "vpc/latest", targets[1].Path)
	assert.Equal(t, "2.2.0", targets[1].Version)
}

func TestAtmosVendorTargets_UnmarshalYAML_MixedSyntax(t *testing.T) {
	input := `
- "components/terraform/vpc"
- path: "vpc/{{.Version}}"
  version: "2.1.0"
- "rds"
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.NoError(t, err)

	assert.Len(t, targets, 3)

	assert.Equal(t, "components/terraform/vpc", targets[0].Path)
	assert.Empty(t, targets[0].Version)

	assert.Equal(t, "vpc/{{.Version}}", targets[1].Path)
	assert.Equal(t, "2.1.0", targets[1].Version)

	assert.Equal(t, "rds", targets[2].Path)
	assert.Empty(t, targets[2].Version)
}

func TestAtmosVendorTargets_UnmarshalYAML_MapPathOnly(t *testing.T) {
	input := `
- path: "vpc"
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.NoError(t, err)

	assert.Len(t, targets, 1)
	assert.Equal(t, "vpc", targets[0].Path)
	assert.Empty(t, targets[0].Version)
}

func TestAtmosVendorTargets_UnmarshalYAML_EmptyList(t *testing.T) {
	input := `[]`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.NoError(t, err)
	assert.Empty(t, targets)
}

func TestAtmosVendorTargets_UnmarshalYAML_MissingPath(t *testing.T) {
	input := `
- version: "2.1.0"
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetMissingPath)
}

func TestAtmosVendorTargets_UnmarshalYAML_EmptyScalarPath(t *testing.T) {
	input := `
- ""
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetMissingPath)
}

func TestAtmosVendorTargets_UnmarshalYAML_WhitespaceScalarPath(t *testing.T) {
	input := `
- "   "
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetMissingPath)
}

func TestAtmosVendorTargets_UnmarshalYAML_WhitespaceMapPath(t *testing.T) {
	input := `
- path: "   "
  version: "1.0.0"
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetMissingPath)
}

func TestAtmosVendorTargets_UnmarshalYAML_NotASequence(t *testing.T) {
	input := `"just a string"`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetInvalidFormat)
}

func TestAtmosVendorTargets_UnmarshalYAML_UnexpectedNodeKind(t *testing.T) {
	input := `
- []
`
	var targets AtmosVendorTargets
	err := yaml.Unmarshal([]byte(input), &targets)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVendorTargetUnexpectedNodeKind)
}

func TestAtmosVendorTargets_UnmarshalYAML_FullVendorSource(t *testing.T) {
	// Test that AtmosVendorSource correctly unmarshals with the new target types.
	input := `
component: vpc
source: "github.com/cloudposse/terraform-aws-vpc.git///?ref={{.Version}}"
version: "2.1.0"
targets:
  - "components/terraform/vpc"
  - path: "vpc/{{.Version}}"
    version: "3.0.0"
`
	var source AtmosVendorSource
	err := yaml.Unmarshal([]byte(input), &source)
	require.NoError(t, err)

	assert.Equal(t, "vpc", source.Component)
	assert.Equal(t, "2.1.0", source.Version)
	assert.Len(t, source.Targets, 2)

	assert.Equal(t, "components/terraform/vpc", source.Targets[0].Path)
	assert.Empty(t, source.Targets[0].Version)

	assert.Equal(t, "vpc/{{.Version}}", source.Targets[1].Path)
	assert.Equal(t, "3.0.0", source.Targets[1].Version)
}

func TestAtmosVendorTargets_UnmarshalYAML_FullVendorConfig(t *testing.T) {
	// Test that AtmosVendorConfig correctly unmarshals with mixed target types.
	input := `
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test
  description: test config
spec:
  sources:
    - component: vpc
      source: "github.com/cloudposse/terraform-aws-vpc.git///?ref={{.Version}}"
      targets:
        - path: "vpc/{{.Version}}"
          version: "2.1.0"
        - path: "vpc/latest"
          version: "2.2.0"
`
	var config AtmosVendorConfig
	err := yaml.Unmarshal([]byte(input), &config)
	require.NoError(t, err)

	require.Len(t, config.Spec.Sources, 1)
	source := config.Spec.Sources[0]
	assert.Equal(t, "vpc", source.Component)
	assert.Len(t, source.Targets, 2)

	assert.Equal(t, "vpc/{{.Version}}", source.Targets[0].Path)
	assert.Equal(t, "2.1.0", source.Targets[0].Version)

	assert.Equal(t, "vpc/latest", source.Targets[1].Path)
	assert.Equal(t, "2.2.0", source.Targets[1].Version)
}
