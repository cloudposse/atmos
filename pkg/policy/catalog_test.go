package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type catalogCase struct {
	Name    string            `json:"name"`
	Request Request           `json:"request"`
	Status  DecisionStatus    `json:"status"`
	Error   string            `json:"error,omitempty"`
	Hooks   []HookRequirement `json:"hooks,omitempty"`
}

// TestPolicyCatalog evaluates each distributed policy against its colocated fixtures.
// A new entry must contain exactly one rule and matching/nonmatching test cases.
func TestPolicyCatalog(t *testing.T) {
	root, entries := policyCatalogEntries(t)
	engine := NewEngine()
	for _, entry := range entries {
		t.Run(entry, func(t *testing.T) {
			directory := filepath.Join(root, entry)
			bundle, err := LoadDirectory(directory)
			require.NoError(t, err)
			require.Len(t, bundle.Rules, 1, "each catalog entry must define one policy")
			rule := bundle.Rules[0]
			variants := catalogVariants(t, directory, bundle)
			require.NotEmpty(t, rule.Description, "catalog policies need a plain-English description")
			require.Contains(t, rule.ID, entry+"/")
			data, err := os.ReadFile(filepath.Join(directory, "tests", "cases.json"))
			require.NoError(t, err)
			var cases []catalogCase
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			require.NoError(t, decoder.Decode(&cases))
			require.ErrorIs(t, decoder.Decode(new(any)), io.EOF)
			require.NotEmpty(t, cases)
			names := map[string]bool{}
			var matching, nonmatching bool
			for _, tc := range cases {
				require.NotEmpty(t, tc.Name)
				require.False(t, names[tc.Name], "duplicate case %s", tc.Name)
				names[tc.Name] = true
				if tc.Request.Kind == rule.On {
					matching = matching || tc.Status == Allow || tc.Status == Deny || tc.Status == AwaitingApproval
					nonmatching = nonmatching || tc.Status == NotApplicable
				}
				t.Run(tc.Name, func(t *testing.T) {
					expected := checkCatalogCase(t, engine, bundle, &tc)
					for _, language := range []string{CELEngine, StarlarkEngine} {
						t.Run(language, func(t *testing.T) {
							actual := checkCatalogCase(t, engine, variants[language], &tc)
							require.Equal(t, expected, actual, "language variants must return identical findings and obligations")
						})
					}
				})
			}
			require.True(t, matching, "missing matching case")
			require.True(t, nonmatching, "missing nonmatching case for the policy's input kind")
		})
	}
}

// checkCatalogCase verifies the fixture's status, error, and hook obligations and returns the full decision for comparison.
func checkCatalogCase(t *testing.T, engine *Engine, bundle *Bundle, tc *catalogCase) Decision {
	t.Helper()
	decision, err := engine.Evaluate(context.Background(), &tc.Request, bundle.Rules)
	if tc.Error == "input_unavailable" {
		require.Equal(t, EvaluationError, tc.Status)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	} else {
		require.Empty(t, tc.Error, "unknown expected error")
		require.NoError(t, err)
	}
	require.Equal(t, tc.Status, decision.Status)
	var hooks []HookRequirement
	for _, finding := range decision.Findings {
		hooks = append(hooks, finding.Effects.Hooks...)
	}
	require.Equal(t, tc.Hooks, hooks, "hook selection, event, and required flags must match")
	for _, hook := range hooks {
		require.Contains(t, bundle.Hooks, hook.Ref)
	}
	return decision
}

// catalogVariants loads both engines and requires their configuration to differ only in engine and expression.
func catalogVariants(t *testing.T, directory string, baseline *Bundle) map[string]*Bundle {
	t.Helper()
	variants := map[string]*Bundle{baseline.Rules[0].Engine: baseline}
	entries, err := os.ReadDir(filepath.Join(directory, "variants"))
	require.NoError(t, err)
	for _, entry := range entries {
		require.True(t, entry.IsDir(), "variants must be isolated engine directories")
		require.Contains(t, []string{CELEngine, StarlarkEngine}, entry.Name())
		variant, err := LoadDirectory(filepath.Join(directory, "variants", entry.Name()))
		require.NoError(t, err)
		require.Len(t, variant.Rules, 1)
		rule := variant.Rules[0]
		require.Equal(t, entry.Name(), rule.Engine)
		require.NotContains(t, variants, rule.Engine, "one implementation per engine")
		variants[rule.Engine] = variant
		expected := baseline.Rules[0]
		expected.Engine, expected.Expression = "", ""
		rule.Engine, rule.Expression = "", ""
		require.Equal(t, expected, rule, "only the engine and expression may differ")
		require.Equal(t, baseline.Hooks, variant.Hooks)
		require.Equal(t, baseline.Tools, variant.Tools)
	}
	require.Len(t, variants, 2, "every library policy needs CEL and Starlark implementations")
	return variants
}

// TestPolicyCatalogCombined verifies selected files can share one governing directory.
func TestPolicyCatalogCombined(t *testing.T) {
	root, entries := policyCatalogEntries(t)
	for _, language := range []string{CELEngine, StarlarkEngine} {
		t.Run(language, func(t *testing.T) {
			testPolicyCatalogCombined(t, root, entries, language)
		})
	}
}

// testPolicyCatalogCombined verifies that one engine's catalog can share a directory without rule or hook conflicts.
func testPolicyCatalogCombined(t *testing.T, root string, entries []string, language string) {
	t.Helper()
	directory := t.TempDir()
	for _, entry := range entries {
		entryDirectory := filepath.Join(root, entry)
		baseline, err := LoadDirectory(entryDirectory)
		require.NoError(t, err)
		if baseline.Rules[0].Engine != language {
			entryDirectory = filepath.Join(entryDirectory, "variants", language)
		}
		data, err := os.ReadFile(filepath.Join(entryDirectory, "policy.yaml"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(directory, entry+".yaml"), data, 0o600))
	}
	bundle, err := LoadDirectory(directory)
	require.NoError(t, err, "catalog entries must not collide when combined")
	require.Len(t, bundle.Rules, len(entries))
	for i := range bundle.Rules {
		for _, hook := range bundle.Rules[i].Effects.Hooks {
			require.Contains(t, bundle.Hooks, hook.Ref)
		}
	}
}

// policyCatalogEntries returns the absolute library root and entry directories, failing if the catalog is empty.
func policyCatalogEntries(t *testing.T) (string, []string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "policy-library"))
	require.NoError(t, err)
	files, err := os.ReadDir(root)
	require.NoError(t, err)
	var entries []string
	for _, file := range files {
		if file.IsDir() {
			entries = append(entries, file.Name())
		}
	}
	require.NotEmpty(t, entries, "policy library must not be empty")
	return root, entries
}
