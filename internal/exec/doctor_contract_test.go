package exec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mapStackReader serves manifests from memory.
type mapStackReader map[string][]byte

func (r mapStackReader) ReadFile(path string) ([]byte, error) {
	if content, ok := r[filepath.ToSlash(path)]; ok {
		return content, nil
	}
	return nil, fmt.Errorf("%w: %s", os.ErrNotExist, path)
}

// staticInputsFromMaps renders decoded manifests as in-memory files below an empty stacks base path.
func staticInputsFromMaps(t *testing.T, manifests map[string]map[string]any) StaticStackInputs {
	t.Helper()
	reader := mapStackReader{}
	var files []string
	for path, values := range manifests {
		content, err := yaml.Marshal(values)
		require.NoError(t, err)
		reader[filepath.ToSlash(filepath.FromSlash(path))] = content
		files = append(files, path)
	}
	return StaticStackInputs{Files: files, Reader: reader}
}

// This counterexample documents the intentional semantic change the doctor migration must report.
func TestDoctorDependencyContractMissingTargetChangesFailureBehavior(t *testing.T) {
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
	require.NoError(t, ValidateStaticStacks(context.Background(), config, staticInputsFromMaps(t, valid)))
	invalid := map[string]map[string]any{"dev.yaml": {"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"metadata": map[string]any{"inherits": []any{"missing"}}}}}}}
	require.Error(t, ValidateStaticStacks(context.Background(), config, staticInputsFromMaps(t, invalid)))
}

func TestDoctorStaticStacksStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inputs := map[string]map[string]any{"dev.yaml": {"components": map[string]any{"terraform": map[string]any{"app": map[string]any{}}}}}
	err := ValidateStaticStacks(ctx, &schema.AtmosConfiguration{}, staticInputsFromMaps(t, inputs))
	require.ErrorIs(t, err, context.Canceled)
}

func TestDoctorStaticStacksReportsDuplicateComponentInstances(t *testing.T) {
	config := &schema.AtmosConfiguration{Stacks: schema.Stacks{NamePattern: "{stage}"}}
	component := map[string]any{"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"stage": "dev"}}}}}
	inputs := map[string]map[string]any{
		"dev-a.yaml": {"vars": map[string]any{"stage": "dev"}, "components": component["components"]},
		"dev-b.yaml": {"vars": map[string]any{"stage": "dev"}, "components": map[string]any{"terraform": map[string]any{"app": map[string]any{"vars": map[string]any{"stage": "dev", "size": "large"}}}}},
	}
	err := ValidateStaticStacks(context.Background(), config, staticInputsFromMaps(t, inputs))
	require.ErrorIs(t, err, errUtils.ErrValidationFailed)
	require.Contains(t, err.Error(), "app")
}

// staticProjectConfig returns a configuration whose stacks live in dir/stacks and are named by stage.
func staticProjectConfig(dir string) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		BasePath:               dir,
		StacksBaseAbsolutePath: filepath.Join(dir, "stacks"),
		Stacks:                 schema.Stacks{NamePattern: "{stage}"},
	}
}

const staticCatalogBase = "components:\n  terraform:\n    base:\n      metadata:\n        type: abstract\n      vars:\n        size: small\n"

// Regular imports are merged by the production loader, so a component can inherit from an imported base.
func TestDoctorStaticStacksResolveImports(t *testing.T) {
	dir := writeStackFixture(t, map[string]string{
		"stacks/catalog/base.yaml": staticCatalogBase,
		"stacks/dev.yaml":          "import:\n  - catalog/*\nvars:\n  stage: dev\ncomponents:\n  terraform:\n    app:\n      metadata:\n        inherits: [base]\n",
	})
	config := staticProjectConfig(dir)
	inputs := StaticStackInputs{Files: []string{"dev.yaml"}}

	require.NoError(t, ValidateStaticStacks(context.Background(), config, inputs))

	t.Run("a missing import is an error", func(t *testing.T) {
		missing := writeStackFixture(t, map[string]string{"stacks/dev.yaml": "import:\n  - catalog/absent\nvars:\n  stage: dev\n"})
		err := ValidateStaticStacks(context.Background(), staticProjectConfig(missing), inputs)
		require.ErrorIs(t, err, errUtils.ErrStackImportNotFound)
	})

	t.Run("the reader replaces the file content for the manifest and its imports", func(t *testing.T) {
		reader := mapStackReader{filepath.ToSlash(filepath.Join(dir, "stacks", "catalog", "base.yaml")): []byte("vars: {}\n")}
		overlay := fallbackStackReader{primary: reader}

		err := ValidateStaticStacks(context.Background(), config, StaticStackInputs{Files: []string{"dev.yaml"}, Reader: overlay})

		require.Error(t, err, "the candidate import no longer defines the base component app inherits")
		require.Contains(t, err.Error(), "base")
	})
}

// fallbackStackReader serves overridden files from memory and everything else from disk.
type fallbackStackReader struct{ primary mapStackReader }

func (r fallbackStackReader) ReadFile(path string) ([]byte, error) {
	if content, err := r.primary.ReadFile(path); err == nil {
		return content, nil
	}
	return os.ReadFile(path)
}

func TestDoctorStaticStacksNeverEvaluateTemplatesInImports(t *testing.T) {
	dir := writeStackFixture(t, map[string]string{
		"stacks/catalog/base.yaml": "vars:\n  name: \"{{ undefined_function .x }}\"\n",
		"stacks/dev.yaml":          "import:\n  - catalog/base\nvars:\n  stage: dev\n",
	})

	require.NoError(t, ValidateStaticStacks(context.Background(), staticProjectConfig(dir), StaticStackInputs{Files: []string{"dev.yaml"}}))
}

func TestDoctorStaticDependenciesReportEveryMissingTarget(t *testing.T) {
	dir := writeStackFixture(t, map[string]string{
		"stacks/dev.yaml":  "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    app:\n      dependencies:\n        components:\n          - component: vpc\n",
		"stacks/prod.yaml": "vars:\n  stage: prod\ncomponents:\n  terraform:\n    app:\n      dependencies:\n        components:\n          - component: missing-a\n          - component: missing-b\n    web:\n      dependencies:\n        components:\n          - component: missing-c\n",
	})

	err := ValidateStaticDependencies(context.Background(), staticProjectConfig(dir), StaticStackInputs{Files: []string{"dev.yaml", "prod.yaml"}})

	require.ErrorIs(t, err, errUtils.ErrDependencyTargetNotFound)
	for _, target := range []string{"missing-a", "missing-b", "missing-c"} {
		require.Contains(t, err.Error(), "target_component="+target)
	}
}

// The mixed-inheritance scenario: migrating only the abstract base changes what app depends on.
func TestStaticDependencyTargetsFollowRuntimeSelectionAcrossInheritance(t *testing.T) {
	const manifest = "vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    db: {}\n    base:\n      metadata:\n        type: abstract\n%s\n    app:\n      metadata:\n        component: app\n        inherits: [base]\n      settings:\n        depends_on:\n          1:\n            component: db\n"
	legacyBase := "      settings:\n        depends_on:\n          - component: vpc"
	modernBase := "      dependencies:\n        components:\n          - component: vpc"
	targets := func(base string) map[dependency.ComponentRef][]dependency.ComponentRef {
		dir := writeStackFixture(t, map[string]string{"stacks/dev.yaml": fmt.Sprintf(manifest, base)})
		result, err := StaticDependencyTargets(context.Background(), staticProjectConfig(dir), StaticStackInputs{Files: []string{"dev.yaml"}})
		require.NoError(t, err)
		return result
	}

	before := targets(legacyBase)
	after := targets(modernBase)

	app := dependency.ComponentRef{Component: "app", Stack: "dev"}
	require.Equal(t, []dependency.ComponentRef{{Component: "db", Stack: "dev"}}, before[app])
	require.Equal(t, []dependency.ComponentRef{{Component: "vpc", Stack: "dev"}}, after[app])
}

// Loading the same stack twice with different content, as a repair verification does, must not reuse
// the inheritance results of the first load.
func TestStaticDependencyTargetsDoNotReuseInheritanceFromAnEarlierLoad(t *testing.T) {
	dir := writeStackFixture(t, map[string]string{"stacks/dev.yaml": "vars:\n  stage: dev\n"})
	config := staticProjectConfig(dir)
	file := filepath.ToSlash(filepath.Join(dir, "stacks", "dev.yaml"))
	manifest := func(base string) mapStackReader {
		return mapStackReader{file: []byte("vars:\n  stage: dev\ncomponents:\n  terraform:\n    vpc: {}\n    db: {}\n    base:\n      metadata:\n        type: abstract\n" + base + "    app:\n      metadata:\n        inherits: [base]\n")}
	}
	inputs := func(reader mapStackReader) StaticStackInputs {
		return StaticStackInputs{Files: []string{"dev.yaml"}, Reader: reader}
	}
	app := dependency.ComponentRef{Component: "app", Stack: "dev"}

	first, err := StaticDependencyTargets(context.Background(), config, inputs(manifest("      dependencies:\n        components:\n          - component: vpc\n")))
	require.NoError(t, err)
	second, err := StaticDependencyTargets(context.Background(), config, inputs(manifest("      dependencies:\n        components:\n          - component: db\n")))
	require.NoError(t, err)

	require.Equal(t, []dependency.ComponentRef{{Component: "vpc", Stack: "dev"}}, first[app])
	require.Equal(t, []dependency.ComponentRef{{Component: "db", Stack: "dev"}}, second[app])
}
