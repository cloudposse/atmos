package conformance

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
)

func TestRegisteredExtensionConformance(t *testing.T) {
	t.Parallel()
	for _, filter := range []string{"", "extension/test.example/first", "absent"} {
		t.Run(filter, func(t *testing.T) {
			t.Parallel()
			registry := extension.New()
			var tested []string
			require.NoError(t, registry.Register(extension.Descriptor{
				Capability: "test.example", Path: []string{"examples"}, Version: 1,
				Validate: func(extension.Binding) error { return nil },
				Test: func(ctx context.Context, binding extension.Binding) error {
					require.NoError(t, ctx.Err())
					require.Equal(t, "fixture-directory", binding.Directory)
					require.Equal(t, "value", binding.Metadata["setting"])
					tested = append(tested, binding.Name)
					return nil
				},
			}))
			bindings, err := registry.Validate("fixture-directory", map[string]any{
				"examples": map[string]any{"first": map[string]any{"setting": "value"}, "second": map[string]any{"setting": "value"}},
			})
			require.NoError(t, err)
			s := suite{bindings: bindings, registry: registry}
			report := &Report{Results: []Result{{Name: "manifest-handler-agreement", Passed: true}}}
			err = s.runFixtures(t.Context(), &Config{Version: 1}, &Options{Run: filter}, report)
			switch filter {
			case "":
				require.NoError(t, err)
				require.Equal(t, []string{"first", "second"}, tested)
				require.Equal(t, []Result{
					{Name: "manifest-handler-agreement", Passed: true},
					{Name: "extension/test.example/first", Passed: true},
					{Name: "extension/test.example/second", Passed: true},
				}, report.Results)
			case "absent":
				require.ErrorIs(t, err, errUtils.ErrPluginTestsNotMatched)
				require.Equal(t, []string{"Available tests: extension/test.example/first, extension/test.example/second."}, errUtils.AllHints(err))
				require.Empty(t, tested)
			default:
				require.NoError(t, err)
				require.Equal(t, []string{"first"}, tested)
				require.Equal(t, Result{Name: filter, Passed: true}, report.Results[1])
			}
		})
	}
}

func TestExtensionConformanceFailures(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{errors.New("credential-bearing failure"), context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			registry := extension.New()
			calls := 0
			require.NoError(t, registry.Register(extension.Descriptor{
				Capability: "test.example", Path: []string{"examples"}, Version: 1,
				Validate: func(extension.Binding) error { return nil },
				Test: func(context.Context, extension.Binding) error {
					calls++
					return failure
				},
			}))
			s := suite{registry: registry, bindings: []extension.Binding{
				{Capability: "test.example", Name: "first"},
				{Capability: "test.example", Name: "second"},
			}}
			report := &Report{}
			err := safeFailure(s.runFixtures(t.Context(), &Config{Version: 1}, &Options{}, report))
			require.ErrorIs(t, err, errUtils.ErrTestsFailed)
			require.NotContains(t, err.Error(), "credential-bearing")
			if errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
				require.ErrorIs(t, err, failure)
			}
			require.Equal(t, 1, calls, "a failing extension must stop the suite")
			require.Equal(t, []Result{{Name: "extension/test.example/first", Passed: false, Detail: failure.Error()}}, report.Results)
		})
	}
}

func TestCanceledExtensionConformanceDoesNotInvokeHook(t *testing.T) {
	t.Parallel()
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{
		Capability: "test.example", Path: []string{"examples"}, Version: 1,
		Validate: func(extension.Binding) error { return nil },
		Test: func(context.Context, extension.Binding) error {
			t.Fatal("canceled suite must not invoke another hook")
			return nil
		},
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := suite{registry: registry, bindings: []extension.Binding{{Capability: "test.example", Name: "first"}}}
	report := &Report{}
	require.ErrorIs(t, s.runFixtures(ctx, &Config{Version: 1}, &Options{}, report), context.Canceled)
	require.Empty(t, report.Results)
}

func TestExtensionWithoutHookDoesNotCountAsExecuted(t *testing.T) {
	t.Parallel()
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{
		Capability: "test.example", Path: []string{"examples"}, Version: 1,
		Validate: func(extension.Binding) error { return nil },
	}))
	s := suite{registry: registry, bindings: []extension.Binding{{Capability: "test.example", Name: "first"}}}
	report := &Report{Results: []Result{{Name: "manifest-handler-agreement", Passed: true}}}
	err := s.runFixtures(t.Context(), &Config{Version: 1}, &Options{}, report)
	require.ErrorIs(t, err, errUtils.ErrPluginTestsNotMatched)
	require.Equal(t, []Result{{Name: "manifest-handler-agreement", Passed: true}}, report.Results)
}
