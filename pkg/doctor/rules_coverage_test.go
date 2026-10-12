package doctor

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/validation"
)

func ecNode(t *testing.T, content string) *yaml.Node {
	t.Helper()
	node, err := parse([]byte(content))
	require.NoError(t, err)
	return node
}

func ecPaths(findings []Finding) []string {
	paths := []string{}
	for i := range findings {
		paths = append(paths, findings[i].Path)
	}
	return paths
}

func TestEcCommandFieldPathOnlyMatchesDeclaredCommandTrees(t *testing.T) {
	const simple = "commands:\n  - name: a\n    arguments:\n      - name: t\n    flags:\n      - name: f\n"
	const nested = "commands:\n  - name: a\n    commands:\n      - name: b\n        flags:\n          - name: f\n        steps: [echo]\n"
	for _, tt := range []struct {
		name    string
		content string
		path    []string
		want    bool
	}{
		{"argument", simple, []string{"commands", "0", "arguments", "0"}, true},
		{"flag", simple, []string{"commands", "0", "flags", "0"}, true},
		{"nested subcommand flag", nested, []string{"commands", "0", "commands", "0", "flags", "0"}, true},
		{"too short", simple, []string{"commands", "0", "arguments"}, false},
		{"odd length", simple, []string{"commands", "0", "arguments", "0", "name"}, false},
		{"not under commands", "vars:\n  - arguments:\n      - name: t\n", []string{"vars", "0", "arguments", "0"}, false},
		{"commands is a mapping", "commands:\n  a:\n    arguments:\n      - name: t\n", []string{"commands", "a", "arguments", "0"}, false},
		{"field is neither arguments nor flags", simple, []string{"commands", "0", "steps", "0"}, false},
		{"arguments is a mapping", "commands:\n  - name: a\n    arguments:\n      name: t\n", []string{"commands", "0", "arguments", "name"}, false},
		{"nested key is not commands", nested, []string{"commands", "0", "steps", "0", "flags", "0"}, false},
		{"nested commands is a mapping", "commands:\n  - name: a\n    commands:\n      b:\n        flags:\n          - name: f\n", []string{"commands", "0", "commands", "b", "flags", "0"}, false},
		{"path missing from the document", simple, []string{"commands", "5", "arguments", "0"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, commandFieldPath(ecNode(t, tt.content), tt.path))
		})
	}
}

func TestEcNestedCommandMigrationIsOffered(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "commands:\n  - name: parent\n    commands:\n      - name: child\n        arguments:\n          - name: target\n            type: stack\n        flags:\n          - name: which\n            semantic_type: component\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, DryRun: true, Only: []string{ruleCommandProvides}})
	found := ecFindings(&r, ruleCommandProvides)
	require.Equal(t, []string{"commands[0].commands[0].arguments[0]", "commands[0].commands[0].flags[0]"}, ecPaths(found))
	for i := range found {
		require.Equal(t, "available", found[i].Status)
		require.True(t, found[i].Fixable)
	}
}

func TestEcHookEventPathScopes(t *testing.T) {
	for _, tt := range []struct {
		name string
		path []string
		want bool
	}{
		{"global hooks", []string{"hooks", "notify", "events", "0"}, true},
		{"component-type hooks", []string{"terraform", "hooks", "notify", "events", "0"}, true},
		{"helmfile component-type hooks", []string{"helmfile", "hooks", "notify", "events", "0"}, true},
		{"component hooks", []string{"components", "terraform", "app", "hooks", "notify", "events", "0"}, true},
		{"non-component prefix", []string{"vars", "hooks", "notify", "events", "0"}, false},
		{"unknown component type", []string{"components", "bogus", "app", "hooks", "notify", "events", "0"}, false},
		{"unknown component-type scope", []string{"bogus", "hooks", "notify", "events", "0"}, false},
		{"prefix of two", []string{"components", "terraform", "hooks", "notify", "events", "0"}, false},
		{"wrong first segment", []string{"vars", "app", "hooks", "notify", "events", "0"}, false},
		{"not an events list", []string{"hooks", "notify", "commands", "0"}, false},
		{"not under hooks", []string{"other", "notify", "events", "0"}, false},
		{"too short", []string{"events", "0"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, hookEventPath(tt.path))
		})
	}

	t.Run("legacy events are found in every supported scope only", func(t *testing.T) {
		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		projectFile(t, root, "stacks/dev.yaml", "hooks:\n  g:\n    events: [before-terraform-plan]\nterraform:\n  hooks:\n    t:\n      events: [after-terraform-plan]\ncomponents:\n  terraform:\n    app:\n      hooks:\n        c:\n          events: [before-terraform-apply]\nvars:\n  hooks:\n    v:\n      events: [before-terraform-init]\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleHookEvent}})
		require.Equal(t, []string{"components.terraform.app.hooks.c.events[0]", "hooks.g.events[0]", "terraform.hooks.t.events[0]"}, ecSorted(ecPaths(ecFindings(&r, ruleHookEvent))))
	})
}

func ecSorted(values []string) []string {
	sorted := append([]string{}, values...)
	sort.Strings(sorted)
	return sorted
}

func TestEcMigrationConflictsStayManual(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		rule    string
	}{
		{"argument already provides", "commands:\n  - name: c\n    arguments:\n      - name: t\n        type: stack\n        provides: component\n", ruleCommandProvides},
		{"flag already provides", "commands:\n  - name: c\n    flags:\n      - name: f\n        semantic_type: stack\n        provides: component\n", ruleCommandProvides},
		{"terraform init already declared", "components:\n  terraform:\n    init_run_reconfigure: false\n    init:\n      reconfigure: always\n", ruleTerraformReconfigure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			path := projectFile(t, root, "atmos.yaml", tt.content)
			r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{tt.rule}})
			require.Empty(t, r.ChangedFiles)
			found := ecFindings(&r, tt.rule)
			require.Len(t, found, 1)
			require.Equal(t, statusManual, found[0].Status)
			require.False(t, found[0].Fixable)
			require.Contains(t, found[0].Reason, "already declared")
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.content, string(content))
		})
	}

	t.Run("only provides and init conflict", func(t *testing.T) {
		mapping := ecNode(t, "provides: x\ninit: {}\n").Content[0]
		require.True(t, hasMigrationConflict(mapping, &edit{kind: ruleCommandProvides}))
		require.True(t, hasMigrationConflict(mapping, &edit{kind: ruleTerraformReconfigure}))
		require.False(t, hasMigrationConflict(mapping, &edit{kind: ruleHookEvent}))
		empty := ecNode(t, "name: x\n").Content[0]
		require.False(t, hasMigrationConflict(empty, &edit{kind: ruleCommandProvides}))
		require.False(t, hasMigrationConflict(empty, &edit{kind: ruleTerraformReconfigure}))
	})
}

func TestEcApplyEditRejectsUnknownKinds(t *testing.T) {
	_, err := applyEdit([]byte("a: 1\n"), &edit{doc: &document{path: "x.yaml"}, path: []string{"a"}, kind: "unknown"})
	require.ErrorIs(t, err, ErrVerification)
}

func TestEcVerifyDocumentRefusesUnsafeTransformations(t *testing.T) {
	hook := edit{path: []string{"hooks", "h", "events", "0"}, kind: ruleHookEvent}
	const before = "# keep\nhooks:\n  h:\n    events: [before-terraform-plan]\n"

	t.Run("normalized hook events compare equal", func(t *testing.T) {
		require.NoError(t, verifyDocument([]byte(before), []byte("# keep\nhooks:\n  h:\n    events: [before.terraform.plan]\n"), []edit{hook}))
	})

	for _, tt := range []struct {
		name          string
		before, after string
		edits         []edit
		wantErr       error
		message       string
	}{
		{"lost comment", before, "hooks:\n  h:\n    events: [before.terraform.plan]\n", []edit{hook}, ErrVerification, "comments changed"},
		{"changed comment", before, "# other\nhooks:\n  h:\n    events: [before.terraform.plan]\n", []edit{hook}, ErrVerification, "comments changed"},
		{"edit path missing", before, before, []edit{{path: []string{"missing"}, kind: ruleHookEvent}}, ErrVerification, ""},
		{"edit path missing after", "hooks: {}\n", "hooks: {}\n", []edit{hook}, ErrVerification, ""},
		{"edit target removed by the candidate", before, "# keep\nhooks:\n  h:\n    events: []\n", []edit{hook}, ErrVerification, ""},
		{"unrelated value changed", before, "# keep\nhooks:\n  h:\n    events: [before.terraform.plan]\n    command: added\n", []edit{hook}, ErrVerification, "effective or unrelated YAML values changed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyDocument([]byte(tt.before), []byte(tt.after), tt.edits)
			require.ErrorIs(t, err, tt.wantErr)
			require.Contains(t, err.Error(), tt.message)
		})
	}

	t.Run("fields the production consumer cannot decode are errors", func(t *testing.T) {
		content := []byte("commands:\n  - name: c\n    arguments:\n      - name: [1]\n        type: stack\n")
		err := verifyDocument(content, content, []edit{{path: []string{"commands", "0", "arguments", "0"}, kind: ruleCommandProvides, old: "type"}})
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrVerification)
	})

	t.Run("malformed documents are errors", func(t *testing.T) {
		require.Error(t, verifyDocument([]byte(ecBroken), []byte("a: 1\n"), nil))
		require.Error(t, verifyDocument([]byte("a: 1\n"), []byte(ecBroken), nil))
	})
}

func TestEcNormalizeNodeComputesEffectiveValues(t *testing.T) {
	scalar := func(node *yaml.Node, key string) string {
		if v := mappingValue(node, key); v != nil {
			return v.Value
		}
		return "<absent>"
	}

	t.Run("argument type becomes provides", func(t *testing.T) {
		node := ecNode(t, "name: t\ntype: component\n").Content[0]
		require.NoError(t, normalizeNode(node, &edit{kind: ruleCommandProvides, old: "type"}))
		require.Equal(t, "component", scalar(node, "provides"))
		require.Equal(t, "<absent>", scalar(node, "type"))
		require.Equal(t, "t", scalar(node, "name"))
	})

	t.Run("flag semantic_type becomes provides", func(t *testing.T) {
		node := ecNode(t, "name: f\nsemantic_type: stack\n").Content[0]
		require.NoError(t, normalizeNode(node, &edit{kind: ruleCommandProvides, old: "semantic_type"}))
		require.Equal(t, "stack", scalar(node, "provides"))
		require.Equal(t, "<absent>", scalar(node, "semantic_type"))
	})

	t.Run("an existing provides wins", func(t *testing.T) {
		node := ecNode(t, "name: f\nsemantic_type: stack\nprovides: component\n").Content[0]
		require.NoError(t, normalizeNode(node, &edit{kind: ruleCommandProvides, old: "semantic_type"}))
		require.Equal(t, "component", scalar(node, "provides"))
	})

	t.Run("undecodable command fields are errors", func(t *testing.T) {
		for _, old := range []string{"type", "semantic_type"} {
			node := ecNode(t, "name: [1, 2]\n"+old+": stack\n").Content[0]
			require.Error(t, normalizeNode(node, &edit{kind: ruleCommandProvides, old: old}))
		}
	})

	t.Run("hook events use dotted names", func(t *testing.T) {
		node := ecNode(t, "before-terraform-apply\n").Content[0]
		require.NoError(t, normalizeNode(node, &edit{kind: ruleHookEvent}))
		require.Equal(t, "before.terraform.apply", node.Value)
	})

	for _, tt := range []struct {
		name, content, reconfigure, init string
	}{
		{"legacy false", "init_run_reconfigure: false\n", "never", "<absent>"},
		{"legacy true", "init_run_reconfigure: true\n", "auto", "<absent>"},
		{"explicit policy wins over the legacy flag", "init_run_reconfigure: false\ninit:\n  reconfigure: always\n", "always", "<absent>"},
		{"other init keys are preserved", "init_run_reconfigure: false\ninit:\n  reconfigure: always\n  mode: auto\n", "always", "present"},
	} {
		t.Run("reconfigure "+tt.name, func(t *testing.T) {
			node := ecNode(t, tt.content).Content[0]
			require.NoError(t, normalizeNode(node, &edit{kind: ruleTerraformReconfigure}))
			require.Equal(t, tt.reconfigure, scalar(node, "init_run_reconfigure"))
			init := mappingValue(node, "init")
			if tt.init == "present" {
				require.NotNil(t, init)
				require.Nil(t, mappingValue(init, "reconfigure"))
				require.Equal(t, "auto", scalar(init, "mode"))
			} else {
				require.Nil(t, init)
			}
		})
	}

	t.Run("undecodable terraform section is an error", func(t *testing.T) {
		node := ecNode(t, "init_run_reconfigure: [1]\n").Content[0]
		require.Error(t, normalizeNode(node, &edit{kind: ruleTerraformReconfigure}))
	})

	t.Run("unknown kinds leave the node untouched", func(t *testing.T) {
		node := ecNode(t, "a: 1\n").Content[0]
		require.NoError(t, normalizeNode(node, &edit{kind: "unknown"}))
		require.Equal(t, "1", scalar(node, "a"))
	})
}

func TestEcModernizationFindings(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, ".github/workflows/ci.yaml", legacyActionWorkflow+"      - uses: actions/checkout@v4\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    legacy:\n      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=0.1\n    current:\n      source: github.com/cloudposse-terraform-components/aws-vpc.git?ref=1.0\n    vars:\n      settings:\n        source: github.com/cloudposse/terraform-aws-components.git\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleModernization}})
	found := ecFindings(&r, ruleModernization)
	byMessage := map[string][]Finding{}
	for _, f := range found {
		byMessage[f.Message] = append(byMessage[f.Message], f)
		require.Equal(t, validation.SeverityWarning, f.Severity)
		require.Equal(t, statusManual, f.Status)
	}
	actions := byMessage["Legacy Atmos GitHub Action"]
	require.Len(t, actions, 1)
	require.Equal(t, ".github/workflows/ci.yaml", actions[0].File)
	require.Equal(t, 6, actions[0].Line)
	require.Equal(t, "https://atmos.tools/ci", actions[0].Documentation)

	sources := byMessage["Legacy component monorepo source"]
	require.Len(t, sources, 2, "every scalar source naming the monorepo is reported, including in arbitrary variables")
	require.Equal(t, "stacks/dev.yaml", sources[0].File)
	require.Equal(t, 4, sources[0].Line)
	require.Equal(t, 9, sources[1].Line)
	require.Len(t, found, 3)
}

func TestEcDeprecatedNamePatternRequiresEquivalenceProof(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "stacks:\n  name_pattern: \"{tenant}-{stage}\"\n")
	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{ruleDeprecatedField}})
	found := ecFindings(&r, ruleDeprecatedField)
	require.Len(t, found, 1)
	require.Equal(t, "stacks.name_pattern", found[0].Path)
	require.Equal(t, "stacks.name_pattern is deprecated", found[0].Message)
	require.Equal(t, statusVerificationIncomplete, found[0].Status)
	require.Contains(t, found[0].Reason, "workspace equivalence")
	require.Equal(t, 2, found[0].Line)
	require.Empty(t, r.ChangedFiles)
}

func TestEcRepositoryURINormalization(t *testing.T) {
	for _, tt := range []struct {
		name, value, want string
	}{
		{"https with module directory and ref", "https://github.com/o/r.git//modules/vpc?ref=v1", "https://github.com/o/r"},
		{"getter prefix", "git::https://github.com/o/r.git?ref=v1", "https://github.com/o/r"},
		{"scp-style ssh", "git@github.com:o/r.git", "https://github.com/o/r"},
		{"ssh url", "ssh://git@github.com/o/r.git", "https://github.com/o/r"},
		{"bare host path", "github.com/o/r//modules/vpc", "https://github.com/o/r"},
		{"embedded credentials are dropped", "https://user:secret@github.com/o/r.git", "https://github.com/o/r"},
		{"unsupported host is returned unchanged", "https://gitlab.com/o/r.git", "https://gitlab.com/o/r.git"},
		{"local path is returned unchanged", "components/terraform/vpc", "components/terraform/vpc"},
		{"scp-style without a path is returned unchanged", "git@github.com", "git@github.com"},
		{"templated value", "github.com/o/{{ .vars.repo }}", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, repositoryURI(tt.value, []string{"{{"}))
		})
	}

	t.Run("custom delimiters make default markers literal", func(t *testing.T) {
		require.Equal(t, "", repositoryURI("github.com/o/[[ .repo ]]", []string{"[["}))
		require.Equal(t, "https://github.com/o/r", repositoryURI("github.com/o/r?x={{y}}", []string{"[["}))
	})

	t.Run("enterprise hosts keep their port", func(t *testing.T) {
		t.Setenv("GITHUB_SERVER_URL", "https://ghe.example.invalid:8443")
		t.Setenv("GITHUB_API_URL", "https://ghe.example.invalid:8443/api/v3")
		require.Equal(t, "https://ghe.example.invalid:8443/o/r", repositoryURI("https://ghe.example.invalid:8443/o/r.git//mod", []string{"{{"}))
	})
}

func TestEcOfflineRepositoryChecksAreInconclusive(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    app:\n      source: github.com/example/repo.git//modules/app?ref=v1\n    local:\n      source: ../components/local\n")
	stub := &repositoryStub{health: RepositoryHealth{"healthy", "healthy"}}
	e := testEngine(t)
	e.Repositories = stub
	r, err := e.Run(context.Background(), Options{Root: root, Offline: true, Only: []string{"sources"}})
	require.NoError(t, err)
	require.Empty(t, stub.calls, "--offline must never contact the repository host")
	require.Len(t, r.Findings, 1)
	require.Equal(t, "Repository checks were disabled with --offline", r.Findings[0].Message)
	require.Equal(t, statusVerificationIncomplete, r.Findings[0].Status)
	require.Equal(t, repositoryInconclusive, r.Findings[0].Reason)
	require.Equal(t, "components.terraform.app.source", r.Findings[0].Path)
	check := ecCheck(&r, ruleSourceRepository)
	require.NotNil(t, check)
	require.Equal(t, "incomplete", check.Status)
	require.Equal(t, 1, check.Files, "the local path is not a repository reference")
}

func TestEcRepositoryHealthStatusesDriveCheckOutcome(t *testing.T) {
	for _, tt := range []struct {
		health      RepositoryHealth
		wantStatus  string
		wantCheck   string
		wantFinding bool
	}{
		{RepositoryHealth{"healthy", "ok"}, "", statusPassed, false},
		{RepositoryHealth{"archived", "Upstream repository is archived"}, statusManual, statusFindings, true},
		{RepositoryHealth{"missing", "gone"}, statusManual, statusFindings, true},
		{RepositoryHealth{"unavailable", "private"}, statusManual, "incomplete", true},
		{RepositoryHealth{"unsupported", "other host"}, statusVerificationIncomplete, "incomplete", true},
		{RepositoryHealth{repositoryInconclusive, "rate limited"}, statusVerificationIncomplete, "incomplete", true},
	} {
		t.Run(tt.health.Status, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			projectFile(t, root, "vendor.yaml", "spec:\n  sources:\n    - source: https://gitlab.com/example/repo.git\n")
			stub := &repositoryStub{health: tt.health}
			e := testEngine(t)
			e.Repositories = stub
			r, err := e.Run(context.Background(), Options{Root: root, Only: []string{"sources"}})
			require.NoError(t, err)
			require.Equal(t, []string{"https://gitlab.com/example/repo.git"}, stub.calls, "unrecognized hosts are passed through unchanged")
			check := ecCheck(&r, ruleSourceRepository)
			require.NotNil(t, check)
			require.Equal(t, tt.wantCheck, check.Status)
			if !tt.wantFinding {
				require.Empty(t, r.Findings)
				return
			}
			require.Len(t, r.Findings, 1)
			require.Equal(t, tt.wantStatus, r.Findings[0].Status)
			require.Equal(t, tt.health.Status, r.Findings[0].Reason)
			require.Equal(t, tt.health.Message, r.Findings[0].Message)
			require.Equal(t, "vendor.yaml", filepath.ToSlash(r.Findings[0].File))
		})
	}
}

func TestEcDynamicRepositorySourcesAreNotProbed(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor.yaml", "spec:\n  sources:\n    - source: !include other.yaml\n    - source: github.com/example/{{ .Component }}.git\n    - source: !!int 5\n")
	stub := &repositoryStub{health: RepositoryHealth{"healthy", "ok"}}
	e := testEngine(t)
	e.Repositories = stub
	r, err := e.Run(context.Background(), Options{Root: root, Only: []string{"sources"}})
	require.NoError(t, err)
	require.Empty(t, stub.calls)
	require.Len(t, r.Findings, 3)
	for i := range r.Findings {
		require.Equal(t, "Dynamic repository source was not evaluated", r.Findings[i].Message)
		require.Equal(t, statusVerificationIncomplete, r.Findings[i].Status)
	}
	require.True(t, strings.HasSuffix(r.Findings[0].Path, "source"), r.Findings[0].Path)
}

type ecCancelingChecker struct {
	cancel context.CancelFunc
	calls  int
}

func (c *ecCancelingChecker) Check(_ context.Context, _ string) RepositoryHealth {
	c.calls++
	c.cancel()
	return RepositoryHealth{"healthy", "ok"}
}

func TestEcRepositoryChecksStopOnCancellation(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor.yaml", "spec:\n  sources:\n    - source: github.com/example/one.git\n    - source: github.com/example/two.git\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker := &ecCancelingChecker{cancel: cancel}
	e := testEngine(t)
	e.Repositories = checker
	r, err := e.Run(ctx, Options{Root: root, Only: []string{"sources"}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, checker.calls, "no repository is probed after cancellation")
	require.Equal(t, 2, r.ExitCode("none"))
}

func TestEcSupportingPredicates(t *testing.T) {
	t.Run("contains matches whole path segments", func(t *testing.T) {
		require.True(t, contains([]string{"spec", "sources", "uri"}, "sources"))
		require.False(t, contains([]string{"spec", "source", "uri"}, "sources"))
		require.False(t, contains(nil, "sources"))
	})

	t.Run("vendor uri keys only count under sources", func(t *testing.T) {
		require.True(t, sourcePath("vendor", []string{"spec", "sources", "0", "uri"}))
		require.True(t, sourcePath("vendor", []string{"spec", "sources", "0", "source"}))
		require.False(t, sourcePath("vendor", []string{"spec", "targets", "0", "uri"}))
	})

	t.Run("hook names outside the supported catalog are never rewritten", func(t *testing.T) {
		require.True(t, knownHook("before-terraform-apply"))
		require.True(t, knownHook("after.helmfile.apply"))
		require.False(t, knownHook("before-terraform-unknown"))
		require.False(t, knownHook("custom-event"))

		root := newProject(t)
		projectFile(t, root, "atmos.yaml", "{}\n")
		projectFile(t, root, "stacks/dev.yaml", "hooks:\n  h:\n    events: [custom-event, after-terraform-apply]\n")
		r := ecRun(t, &Options{Root: root, Offline: true, Only: []string{ruleHookEvent}})
		require.Equal(t, []string{"hooks.h.events[1]"}, ecPaths(ecFindings(&r, ruleHookEvent)))
	})

	t.Run("non-string template delimiters are ignored", func(t *testing.T) {
		config := map[string]any{"templates": map[string]any{"settings": map[string]any{"delimiters": []any{"[[", 5}}}}
		require.Nil(t, stringList(config, "templates", "settings", "delimiters"))
		config["templates"].(map[string]any)["settings"].(map[string]any)["delimiters"] = []any{"[[", "]]"}
		require.Equal(t, []string{"[[", "]]"}, stringList(config, "templates", "settings", "delimiters"))
		require.Nil(t, stringList(config, "templates", "missing", "delimiters"))
	})
}
