package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

const cfnSelector = "!aws.cloudformation.output producer dev SecretStoreName"

var errProducerMissing = fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationStackNotFound, "producer-dev")

// selectorSection declares one secret whose store is a selector and one with a literal store.
func selectorSection() map[string]any {
	return map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"API_KEY":     map[string]any{"store": cfnSelector},
				"LITERAL_KEY": map[string]any{"store": "app-secrets"},
			},
		},
	}
}

func selectorConfig(s store.Store) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app-secrets": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
		},
		Stores: store.StoreRegistry{"app-secrets": s},
	}
}

func TestIsSelector(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"app-secrets", false},
		{"", false},
		{cfnSelector, true},
		{"  !aws.cloudformation.output a b c", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, IsSelector(tt.value), tt.value)
	}
}

// TestResolve_SelectorStore proves `!secret` resolves a declaration whose store is a selector, using
// the supplied evaluator for that declaration's own path only.
func TestResolve_SelectorStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get("dev", "consumer", "API_KEY").Return("s3cret", nil)
	require.NoError(t, iolib.Initialize())

	var gotPath []string
	evaluator := func(path []string, raw any) (any, error) {
		gotPath = path
		assert.Equal(t, cfnSelector, raw)
		return "app-secrets", nil
	}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "consumer", ComponentSection: selectorSection()}

	got, err := Resolve(selectorConfig(mockStore), "!secret API_KEY", "dev", info, WithSelectorEvaluator(evaluator))
	require.NoError(t, err)
	assert.Equal(t, "s3cret", got)
	assert.Equal(t, []string{"secrets", "vars", "API_KEY", "store"}, gotPath)
}

// TestResolve_SelectorStore_Unresolvable covers the failure modes, which must all name the
// declaration, component, stack and selector and never fall back to treating the selector as a store.
func TestResolve_SelectorStore_Unresolvable(t *testing.T) {
	tests := []struct {
		name      string
		options   []Option
		wantIs    error
		wantHint  string
		wantInMsg []string
	}{
		{
			name:      "no evaluator",
			wantIs:    ErrSelectorEvaluatorUnavailable,
			wantInMsg: []string{`"API_KEY"`, `"consumer"`, `"dev"`, cfnSelector},
		},
		{
			name: "producer not deployed",
			options: []Option{WithSelectorEvaluator(func([]string, any) (any, error) {
				return nil, errProducerMissing
			})},
			wantIs:    errUtils.ErrAwsCloudFormationStackNotFound,
			wantHint:  "Deploy the producer component first: `atmos aws cloudformation deploy producer --stack dev`",
			wantInMsg: []string{`"API_KEY"`, `"consumer"`, `"dev"`, cfnSelector},
		},
		{
			name: "evaluates to non-name",
			options: []Option{WithSelectorEvaluator(func([]string, any) (any, error) {
				return 42, nil
			})},
			wantIs:    ErrSelectorResult,
			wantInMsg: []string{`"API_KEY"`},
		},
		{
			name: "evaluates to another selector",
			options: []Option{WithSelectorEvaluator(func([]string, any) (any, error) {
				return "!terraform.output a b c", nil
			})},
			wantIs:    ErrSelectorResult,
			wantInMsg: []string{`"API_KEY"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl) // No Get expected: the backend must never be contacted.
			info := &schema.ConfigAndStacksInfo{Stack: "dev", Component: "consumer", ComponentSection: selectorSection()}

			_, err := Resolve(selectorConfig(mockStore), "!secret API_KEY", "dev", info, tt.options...)
			require.ErrorIs(t, err, ErrSelectorUnresolved)
			require.ErrorIs(t, err, tt.wantIs)
			for _, want := range tt.wantInMsg {
				assert.Contains(t, err.Error(), want)
			}
			if tt.wantHint != "" {
				assert.Contains(t, errorHints(err), tt.wantHint)
			}
		})
	}
}

// TestService_SelectorIsLazyPerDeclaration is the contract behind the CLI fix: a sibling declaration
// whose selector cannot be resolved never affects a literal declaration, and the evaluator runs only
// for the declaration a command uses.
func TestService_SelectorIsLazyPerDeclaration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockDeletableStore(ctrl)
	mockStore.EXPECT().Get("dev", "consumer", "LITERAL_KEY").Return("literal", nil)
	mockStore.EXPECT().Delete("dev", "consumer", "LITERAL_KEY").Return(nil)

	calls := 0
	svc := NewService(selectorConfig(mockStore), "dev", "consumer", selectorSection(),
		WithSelectorEvaluator(func([]string, any) (any, error) {
			calls++
			return nil, errProducerMissing
		}))

	got, err := svc.Get("LITERAL_KEY", ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "literal", got)
	require.NoError(t, svc.Delete("LITERAL_KEY"))
	assert.Zero(t, calls, "literal declarations must not evaluate any selector")

	_, err = svc.Get("API_KEY", ResolveOptions{})
	require.ErrorIs(t, err, ErrSelectorUnresolved)
	assert.Contains(t, err.Error(), "API_KEY")
	assert.Equal(t, 1, calls)
}

// TestService_SelectorEvaluationIsMemoized proves a successful evaluation is reused.
func TestService_SelectorEvaluationIsMemoized(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get("dev", "consumer", "API_KEY").Return("v", nil).Times(2)

	calls := 0
	svc := NewService(selectorConfig(mockStore), "dev", "consumer", selectorSection(),
		WithSelectorEvaluator(func([]string, any) (any, error) {
			calls++
			return "app-secrets", nil
		}))
	for range 2 {
		_, err := svc.Get("API_KEY", ResolveOptions{})
		require.NoError(t, err)
	}
	assert.Equal(t, 1, calls)
}

// TestService_DeleteAll_ContinuesPastUnresolvableSibling proves one unresolvable selector does not
// strand the values of declarations that can still be deleted.
func TestService_DeleteAll_ContinuesPastUnresolvableSibling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockDeletableStore(ctrl)
	mockStore.EXPECT().Delete("dev", "consumer", "LITERAL_KEY").Return(nil)

	svc := NewService(selectorConfig(mockStore), "dev", "consumer", selectorSection(),
		WithSelectorEvaluator(func([]string, any) (any, error) { return nil, errProducerMissing }))

	n, err := svc.DeleteAll()
	require.ErrorIs(t, err, ErrSelectorUnresolved)
	assert.Equal(t, 1, n, "the literal declaration is still deleted")
}

// TestService_Status_Selector covers listing: credential-free status reports an explicit unresolved
// state with a reason (not a bare error), while verification evaluates the selector and reports a
// failure as an error that names the declaration.
func TestService_Status_Selector(t *testing.T) {
	t.Run("credential-free is unresolved with a reason", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		svc := NewService(selectorConfig(store.NewMockStatusStore(ctrl)), "dev", "consumer", selectorSection())

		byName := statusesByName(svc.Status(false))
		api := byName["API_KEY"]
		require.NoError(t, api.Err)
		assert.True(t, api.Unknown)
		assert.True(t, api.Unresolved)
		assert.Contains(t, api.Reason, cfnSelector)
		assert.Contains(t, api.Reason, "--verify")
		assert.False(t, byName["LITERAL_KEY"].Unresolved)
	})

	t.Run("verify surfaces the failure per declaration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStore := store.NewMockStatusStore(ctrl)
		mockStore.EXPECT().Has("dev", "consumer", "LITERAL_KEY").Return(true, nil)
		svc := NewService(selectorConfig(mockStore), "dev", "consumer", selectorSection(),
			WithSelectorEvaluator(func([]string, any) (any, error) { return nil, errProducerMissing }))

		byName := statusesByName(svc.Status(true))
		require.ErrorIs(t, byName["API_KEY"].Err, ErrSelectorUnresolved)
		assert.NotEmpty(t, byName["API_KEY"].Reason)
		require.NoError(t, byName["LITERAL_KEY"].Err)
		assert.True(t, byName["LITERAL_KEY"].Initialized)
	})
}

func statusesByName(statuses []Status) map[string]Status {
	out := make(map[string]Status, len(statuses))
	for i := range statuses {
		out[statuses[i].Declaration.Name] = statuses[i]
	}
	return out
}

// sopsSelectorSection declares two instances' worth of SOPS secrets sharing one file; the file name
// comes from the provider, which the selector chooses.
func sopsSelectorSection(t *testing.T, sopsValue string) (*schema.AtmosConfiguration, map[string]any, string) {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)
	file := filepath.Join(dir, "shared.enc.yaml")
	section := map[string]any{
		"secrets": map[string]any{
			"providers": map[string]any{
				"sops-a": map[string]any{"kind": "sops/age", "spec": map[string]any{"file": file, "age_recipients": identity.Recipient().String()}},
			},
			"vars": map[string]any{"DB_PASS": map[string]any{"sops": sopsValue}},
		},
	}
	return &schema.AtmosConfiguration{BasePath: dir}, section, file
}

// TestService_SopsPlacements_Selector is the collision-guard contract: a selector-backed SOPS
// declaration is resolved (so its file takes part in collision detection) or the check fails closed
// naming the declaration — it is never silently skipped.
func TestService_SopsPlacements_Selector(t *testing.T) {
	t.Run("resolved selector produces a placement that collides", func(t *testing.T) {
		cfg, sectionY, file := sopsSelectorSection(t, "!aws.cloudformation.output producer dev SopsProviderName")
		_, sectionX, _ := sopsSelectorSection(t, "sops-a")
		sectionX["secrets"].(map[string]any)["providers"] = sectionY["secrets"].(map[string]any)["providers"]

		evaluator := WithSelectorEvaluator(func([]string, any) (any, error) { return "sops-a", nil })
		x, err := NewService(cfg, "dev", "x", sectionX, evaluator).SopsPlacements()
		require.NoError(t, err)
		y, err := NewService(cfg, "dev", "y", sectionY, evaluator).SopsPlacements()
		require.NoError(t, err)
		require.Len(t, y, 1)
		assert.Equal(t, file, y[0].File)

		require.ErrorIs(t, DetectSopsCollisions(append(x, y...)), ErrSopsCollision)
	})

	t.Run("unresolvable selector fails closed", func(t *testing.T) {
		cfg, section, _ := sopsSelectorSection(t, "!aws.cloudformation.output producer dev SopsProviderName")
		svc := NewService(cfg, "dev", "y", section,
			WithSelectorEvaluator(func([]string, any) (any, error) { return nil, errProducerMissing }))

		placements, err := svc.SopsPlacements()
		require.ErrorIs(t, err, ErrSelectorUnresolved)
		assert.Empty(t, placements)
		for _, want := range []string{`"DB_PASS"`, `"y"`, `"dev"`} {
			assert.Contains(t, err.Error(), want)
		}
	})

	t.Run("no evaluator fails closed", func(t *testing.T) {
		cfg, section, _ := sopsSelectorSection(t, "!aws.cloudformation.output producer dev SopsProviderName")
		_, err := NewService(cfg, "dev", "y", section).SopsPlacements()
		require.ErrorIs(t, err, ErrSelectorUnresolved)
		require.ErrorIs(t, err, ErrSelectorEvaluatorUnavailable)
	})
}

// TestProviderDefinitionSelector proves selectors inside a SOPS provider definition are evaluated
// for the provider the declaration uses, without mutating the shared section.
func TestProviderDefinitionSelector(t *testing.T) {
	cfg, section, file := sopsSelectorSection(t, "sops-a")
	spec := section["secrets"].(map[string]any)["providers"].(map[string]any)["sops-a"].(map[string]any)["spec"].(map[string]any)
	spec["file"] = "!aws.cloudformation.output producer dev SopsFile"

	var gotPath []string
	svc := NewService(cfg, "dev", "x", section, WithSelectorEvaluator(func(path []string, raw any) (any, error) {
		gotPath = path
		def, ok := raw.(map[string]any)
		require.True(t, ok)
		resolved := map[string]any{"kind": def["kind"], "spec": map[string]any{
			"file":           file,
			"age_recipients": def["spec"].(map[string]any)["age_recipients"],
		}}
		return resolved, nil
	}))

	placements, err := svc.SopsPlacements()
	require.NoError(t, err)
	require.Len(t, placements, 1)
	assert.Equal(t, file, placements[0].File)
	assert.Equal(t, []string{"secrets", "providers", "sops-a"}, gotPath)
	assert.Equal(t, "!aws.cloudformation.output producer dev SopsFile", spec["file"], "the input section must not be mutated")
}

func TestCloudFormationProducer(t *testing.T) {
	tests := []struct {
		selector      string
		wantComponent string
		wantStack     string
		wantOK        bool
	}{
		{"!aws.cloudformation.output producer dev Out", "producer", "dev", true},
		{"!aws.cloudformation.output producer Out", "producer", "fallback", true},
		{"!terraform.output vpc dev id", "", "", false},
		{"literal", "", "", false},
	}
	for _, tt := range tests {
		component, stack, ok := cloudFormationProducer(tt.selector, "fallback")
		assert.Equal(t, tt.wantOK, ok, tt.selector)
		assert.Equal(t, tt.wantComponent, component, tt.selector)
		assert.Equal(t, tt.wantStack, stack, tt.selector)
	}
}

// TestDeclaration_ScopeConflict covers the post-render one-way scope rule for templated scopes.
func TestDeclaration_ScopeConflict(t *testing.T) {
	tests := []struct {
		name string
		decl Declaration
		want bool
	}{
		{"validated at stamp time", Declaration{Name: "X", Scope: ScopeStack}, false},
		{"rendered matches position", Declaration{Name: "X", Scope: ScopeInstance, PositionScope: ScopeInstance}, false},
		{"rendered global is exempt", Declaration{Name: "X", Scope: ScopeGlobal, PositionScope: ScopeInstance}, false},
		{"instance declaration rendered stack", Declaration{Name: "X", Scope: ScopeStack, PositionScope: ScopeInstance}, true},
		{"stack declaration rendered instance", Declaration{Name: "X", Scope: ScopeInstance, PositionScope: ScopeStack}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.decl.ScopeConflict()
			if tt.want {
				require.ErrorIs(t, err, ErrScopeConflict)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTagScope_TemplatedScopeIsDeferred proves a templated/YAML-function scope is not rejected
// before it renders, records its position, and is validated after rendering.
func TestTagScope_TemplatedScopeIsDeferred(t *testing.T) {
	section := map[string]any{"vars": map[string]any{
		"TPL":  map[string]any{"store": "s", "scope": "{{ .vars.scope }}"},
		"FUNC": map[string]any{"store": "s", "scope": "!env SCOPE"},
		"BAD":  map[string]any{"store": "s", "scope": "stack"},
	}}
	_, err := TagScope(section, ScopeInstance)
	require.ErrorIs(t, err, ErrScopeConflict, "a literal conflicting scope is still rejected immediately")

	delete(section["vars"].(map[string]any), "BAD")
	tagged, err := TagScope(section, ScopeInstance)
	require.NoError(t, err)
	vars := tagged["vars"].(map[string]any)
	for _, name := range []string{"TPL", "FUNC"} {
		spec := vars[name].(map[string]any)
		assert.Equal(t, "instance", spec["scope_position"], name)
	}
	assert.Equal(t, "{{ .vars.scope }}", vars["TPL"].(map[string]any)["scope"], "the template is preserved for rendering")

	// After rendering, the rendered scope is validated against the recorded position.
	rendered := map[string]any{"secrets": map[string]any{"vars": map[string]any{
		"TPL": map[string]any{"store": "s", "scope": "stack", "scope_position": "instance"},
	}}}
	decl := ExtractDeclarations(rendered)["TPL"]
	assert.Equal(t, ScopeStack, decl.Scope)
	require.ErrorIs(t, decl.ScopeConflict(), ErrScopeConflict)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	_, err = Resolve(selectorConfig(store.NewMockStore(ctrl)), "!secret TPL", "dev",
		&schema.ConfigAndStacksInfo{Stack: "dev", Component: "c", ComponentSection: rendered})
	require.ErrorIs(t, err, ErrScopeConflict)
}

// errorHints returns the hints attached to an error built with the error builder.
func errorHints(err error) []string {
	return cockroach.GetAllHints(err)
}
