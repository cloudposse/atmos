package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This counterexample is a permanent guard against enabling a syntactic dependency fixer.
func TestDoctorDependencyContractRejectsMissingTargetMigration(t *testing.T) {
	legacy := map[string]any{"settings": map[string]any{"depends_on": []any{map[string]any{"component": "missing"}}}}
	modern := map[string]any{"dependencies": map[string]any{"components": []any{map[string]any{"component": "missing"}}}}
	check := func(component map[string]any) error {
		builder := dependency.NewBuilder()
		require.NoError(t, builder.AddNode(&dependency.Node{ID: "app-dev"}))
		return NewDependencyParser(builder, map[string]string{"app-dev": "app-dev"}).ParseComponentDependencies("dev", "app", component)
	}
	require.NoError(t, check(legacy))
	require.Error(t, check(modern), "A migration changes failure behavior even if existing-target graphs match")
}

func TestDoctorStaticStacksUsesProductionInheritance(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	valid := map[string]map[string]any{"dev.yaml": {"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"region": "us-east-1"}}}}}}
	require.NoError(t, ValidateStaticStackMaps(context.Background(), config, valid))
	invalid := map[string]map[string]any{"dev.yaml": {"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"metadata": map[string]any{"inherits": []any{"missing"}}}}}}}
	require.Error(t, ValidateStaticStackMaps(context.Background(), config, invalid))
}

func TestDoctorStaticStacksStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inputs := map[string]map[string]any{"dev.yaml": {"components": map[string]any{"terraform": map[string]any{"app": map[string]any{}}}}}
	err := ValidateStaticStackMaps(ctx, &schema.AtmosConfiguration{}, inputs)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDoctorStaticStacksReportsDuplicateComponentInstances(t *testing.T) {
	config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{stage}"}}
	component := map[string]any{"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"stage": "dev"}}}}}
	inputs := map[string]map[string]any{
		"dev-a.yaml": {"vars": map[string]any{"stage": "dev"}, "components": component["components"]},
		"dev-b.yaml": {"vars": map[string]any{"stage": "dev"}, "components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"stage": "dev", "size": "large"}}}}},
	}
	err := ValidateStaticStackMaps(context.Background(), config, inputs)
	require.ErrorIs(t, err, errUtils.ErrValidationFailed)
	require.Contains(t, err.Error(), "app")
}
