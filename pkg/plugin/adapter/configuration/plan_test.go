package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// identifiedProvider adds the optional Identity capability to the generated provider mock.
type identifiedProvider struct {
	Provider
	identity func(context.Context) (string, error)
}

func (p *identifiedProvider) Identity(ctx context.Context) (string, error) { return p.identity(ctx) }

// planCase describes every input that a saved-plan binding is expected to cover.
type planCase struct {
	identity   string
	source     string
	component  string
	options    map[string]any
	args       []string
	subCommand string
	env        []string
}

func defaultPlanCase() *planCase {
	return &planCase{identity: "provider-v1", source: "./modules/unit", component: "app", options: map[string]any{"retries": 1}, args: []string{"-var=a=1"}, subCommand: "plan", env: []string{"TOKEN=one"}}
}

// planSession builds a discovered session whose provider reports the case's identity.
func planSession(t *testing.T, c *planCase) (*Session, *schema.ConfigAndStacksInfo) {
	t.Helper()
	config := enabledConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Options = c.options
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	mock := NewMockProvider(gomock.NewController(t))
	mock.EXPECT().ComponentType().Return("terraform").AnyTimes()
	component := unit(t, c.component)
	configuration, err := adapter.EncodeObject(map[string]any{"source": c.source})
	require.NoError(t, err)
	component.Configuration = configuration
	mock.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{component}}, nil)
	provider := &identifiedProvider{Provider: mock, identity: func(context.Context) (string, error) { return c.identity, nil }}
	session, err := New(t.Context(), config, "plan", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: c.component, SubCommand: c.subCommand, AdditionalArgsAndFlags: c.args, ComponentEnvList: c.env}
	return session, info
}

func planIdentity(t *testing.T, c *planCase) string {
	t.Helper()
	session, info := planSession(t, c)
	identity, err := session.PlanIdentity(t.Context(), info, nil)
	require.NoError(t, err)
	require.Len(t, identity, 64, "identity is a hex SHA-256 digest")
	return identity
}

func TestPlanIdentityDetectsEveryBoundInputAndIgnoresInvocationOnlyInputs(t *testing.T) {
	baseline := planIdentity(t, defaultPlanCase())

	for name, change := range map[string]func(*planCase){
		"provider identity": func(c *planCase) { c.identity = "provider-v2" },
		"module source":     func(c *planCase) { c.source = "./modules/other" },
		"component address": func(c *planCase) { c.component = "worker" },
		"provider options":  func(c *planCase) { c.options = map[string]any{"retries": 2} },
	} {
		t.Run("changed "+name, func(t *testing.T) {
			c := defaultPlanCase()
			change(c)
			require.NotEqual(t, baseline, planIdentity(t, c), "a saved plan must not be applied after this input changes")
		})
	}

	for name, change := range map[string]func(*planCase){
		"apply of the reviewed plan": func(c *planCase) { c.subCommand = "apply" },
		"ambient environment":        func(c *planCase) { c.env = []string{"TOKEN=two"} },
		"apply-only arguments":       func(c *planCase) { c.subCommand, c.args = "apply", []string{"-auto-approve"} },
	} {
		t.Run("unchanged "+name, func(t *testing.T) {
			c := defaultPlanCase()
			change(c)
			require.Equal(t, baseline, planIdentity(t, c), "apply must accept the plan reviewed with the same bound inputs")
		})
	}
}

func TestPlanIdentityIsStableAcrossProjectRoots(t *testing.T) {
	// Different checkouts of identical content must accept each other's saved plans.
	first, info := planSession(t, defaultPlanCase())
	second, secondInfo := planSession(t, defaultPlanCase())
	sources := []schema.ComponentConfigurationSource{{Path: "root.hcl"}, {Path: "modules", Directory: true}}
	for _, root := range []string{first.root, second.root} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "modules"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "root.hcl"), []byte("root"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "main.tf"), []byte("module"), 0o600))
	}
	one, err := first.PlanIdentity(t.Context(), info, sources)
	require.NoError(t, err)
	two, err := second.PlanIdentity(t.Context(), secondInfo, sources)
	require.NoError(t, err)
	require.Equal(t, one, two)
}

func TestPlanIdentityBindsWatchedSources(t *testing.T) {
	session, info := planSession(t, defaultPlanCase())
	root := session.root
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	reset := func() {
		t.Helper()
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		for _, entry := range entries {
			require.NoError(t, os.RemoveAll(filepath.Join(root, entry.Name())))
		}
		write("root.hcl", "root")
		write("modules/main.tf", "main")
	}
	reset()
	sources := []schema.ComponentConfigurationSource{{Path: "root.hcl"}, {Path: "modules", Directory: true}}
	identity := func(sources []schema.ComponentConfigurationSource) string {
		t.Helper()
		result, err := session.PlanIdentity(t.Context(), info, sources)
		require.NoError(t, err)
		return result
	}
	baseline := identity(sources)
	require.Equal(t, baseline, identity(sources), "unchanged inputs are accepted")
	require.Equal(t, baseline, identity([]schema.ComponentConfigurationSource{sources[1], sources[0]}), "source order is not significant")
	require.NotEqual(t, baseline, identity(nil), "dropping a watched source changes the binding")

	// Runtime artifacts and state never invalidate a plan, at any depth.
	for _, ignored := range []string{
		"modules/.terraform/providers.json", "modules/.git/HEAD", "modules/terraform.tfstate", "modules/terraform.tfstate.backup",
		"modules/saved.planfile", "modules/unit.atmos.json", "modules/.terragrunt-cache/x/main.tf", "modules/.terragrunt-stack/stack.hcl",
		"modules/nested/.atmos/state", "modules/nested/.terraform/x",
	} {
		write(ignored, "noise")
	}
	require.Equal(t, baseline, identity(sources), "excluded artifacts must not affect the binding")
	reset()

	for _, change := range []struct {
		name string
		do   func()
	}{
		{"file edited", func() { write("root.hcl", "root changed") }},
		{"directory file edited", func() { write("modules/main.tf", "main changed") }},
		{"directory file added", func() { write("modules/extra.tf", "extra") }},
		{"nested directory file added", func() { write("modules/nested/deep.tf", "deep") }},
		{"watched file removed", func() { require.NoError(t, os.Remove(filepath.Join(root, "root.hcl"))) }},
		{"directory file removed", func() { require.NoError(t, os.Remove(filepath.Join(root, "modules", "main.tf"))) }},
		{"directory tree removed", func() { require.NoError(t, os.RemoveAll(filepath.Join(root, "modules"))) }},
	} {
		change.do()
		require.NotEqual(t, baseline, identity(sources), change.name)
		reset()
		require.Equal(t, baseline, identity(sources), "restoring the original bytes after %q is accepted again", change.name)
	}
}

func TestPlanIdentityRejectsUnboundableRequests(t *testing.T) {
	t.Run("provider without identity", func(t *testing.T) {
		session, info := planSession(t, defaultPlanCase())
		_, err := session.Discover(t.Context())
		require.NoError(t, err)
		session.registry["terragrunt"] = NewMockProvider(gomock.NewController(t))
		_, err = session.PlanIdentity(t.Context(), info, nil)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	})
	t.Run("provider identity failure", func(t *testing.T) {
		session, info := planSession(t, defaultPlanCase())
		_, err := session.Discover(t.Context())
		require.NoError(t, err)
		failure := errors.New("executable unreadable")
		session.registry["terragrunt"] = &identifiedProvider{Provider: NewMockProvider(gomock.NewController(t)), identity: func(context.Context) (string, error) { return "", failure }}
		_, err = session.PlanIdentity(t.Context(), info, nil)
		require.ErrorIs(t, err, failure)
	})
	t.Run("undiscovered component", func(t *testing.T) {
		session, info := planSession(t, defaultPlanCase())
		info.ComponentFromArg = "absent"
		_, err := session.PlanIdentity(t.Context(), info, nil)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	})
	t.Run("source escapes the project", func(t *testing.T) {
		session, info := planSession(t, defaultPlanCase())
		for _, path := range []string{"../outside", filepath.Join(session.root, "absolute")} {
			_, err := session.PlanIdentity(t.Context(), info, []schema.ComponentConfigurationSource{{Path: path}})
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, path)
		}
	})
}

func TestSourceDigests(t *testing.T) {
	t.Run("symlink leaving the project is rejected", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.hcl"), []byte("secret"), 0o600))
		if err := os.Symlink(filepath.Join(outside, "secret.hcl"), filepath.Join(root, "link.hcl")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := sourceDigests(t.Context(), root, []schema.ComponentConfigurationSource{{Path: "link.hcl"}})
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	})
	t.Run("symlink inside the project digests its target under the link path", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "shared.hcl")
		require.NoError(t, os.WriteFile(target, []byte("one"), 0o600))
		if err := os.Symlink(target, filepath.Join(root, "link.hcl")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		sources := []schema.ComponentConfigurationSource{{Path: "link.hcl"}}
		before, err := sourceDigests(t.Context(), root, sources)
		require.NoError(t, err)
		require.Contains(t, before, "link.hcl")
		require.NoError(t, os.WriteFile(target, []byte("two"), 0o600))
		after, err := sourceDigests(t.Context(), root, sources)
		require.NoError(t, err)
		require.NotEqual(t, before["link.hcl"], after["link.hcl"], "edits behind a symlink must change the digest")
	})
	t.Run("missing sources are recorded as absent", func(t *testing.T) {
		got, err := sourceDigests(t.Context(), t.TempDir(), []schema.ComponentConfigurationSource{{Path: "missing.hcl"}, {Path: "nodir", Directory: true}})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"missing.hcl": "absent", "nodir": "absent"}, got)
	})
	t.Run("missing project root fails", func(t *testing.T) {
		_, err := sourceDigests(t.Context(), filepath.Join(t.TempDir(), "gone"), nil)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("directory watched as a file fails", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "modules"), 0o700))
		_, err := sourceDigests(t.Context(), root, []schema.ComponentConfigurationSource{{Path: "modules"}})
		require.Error(t, err)
	})
	t.Run("canceled context stops digesting", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.hcl"), []byte("a"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(root, "dir"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, "dir", "b.tf"), []byte("b"), 0o600))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		for _, source := range []schema.ComponentConfigurationSource{{Path: "a.hcl"}, {Path: "dir", Directory: true}} {
			_, err := sourceDigests(ctx, root, []schema.ComponentConfigurationSource{source})
			require.ErrorIs(t, err, context.Canceled, source.Path)
		}
	})
}

func TestExcludedPlanSource(t *testing.T) {
	for name, excluded := range map[string]bool{
		".atmos": true, ".git": true, ".terraform": true, ".terragrunt-cache": true, ".terragrunt-stack": true,
		"terraform.tfstate": true, "terraform.tfstate.backup": true, "a.planfile": true, "b.atmos.json": true,
		"main.tf": false, "terragrunt.hcl": false, ".terraform.lock.hcl": false, "state.json": false,
	} {
		require.Equal(t, excluded, excludedPlanSource(name), name)
	}
}

func TestSourceThatLeavesTheProjectIsNamedBeforeAnythingRuns(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "app", "main.tf"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "victim.tf"), []byte("x"), 0o644))
	sources := []schema.ComponentConfigurationSource{{Path: "modules/app", Directory: true}, {Path: "absent", Directory: true}}
	require.NoError(t, checkSourcesInRoot(t.Context(), root, sources))
	if err := os.Symlink(filepath.Join(outside, "victim.tf"), filepath.Join(root, "modules", "app", "link.tf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	err := checkSourcesInRoot(t.Context(), root, sources)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "modules/app/link.tf")
	require.ErrorContains(t, err, "leaves the project root")
	// The plan fingerprint reports the same file instead of an anonymous failure.
	_, err = sourceDigests(t.Context(), root, sources)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "modules/app/link.tf")
	// A symlink inside the project is fine, and ignored artifacts are not inspected.
	require.NoError(t, os.Remove(filepath.Join(root, "modules", "app", "link.tf")))
	require.NoError(t, os.Symlink(filepath.Join(root, "modules", "app", "main.tf"), filepath.Join(root, "modules", "app", "inside.tf")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "app", ".terraform"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "victim.tf"), filepath.Join(root, "modules", "app", ".terraform", "provider")))
	require.NoError(t, checkSourcesInRoot(t.Context(), root, sources))
}
