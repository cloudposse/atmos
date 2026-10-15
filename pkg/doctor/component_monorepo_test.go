package doctor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/doctor/monorepo"
	"github.com/cloudposse/atmos/pkg/validation"
)

// monorepoAnswer decides the verification of one component at one ref.
type monorepoAnswer func(result monorepo.Result, ref string) monorepo.Verification

// recordingMonorepo returns a verifier that answers every question with answer and records
// each question as "<module>@<ref>".
func recordingMonorepo(t *testing.T, answer monorepoAnswer) (*monorepo.MockVerifier, *[]string) {
	t.Helper()
	var asked []string
	verifier := monorepo.NewMockVerifier(gomock.NewController(t))
	verifier.EXPECT().Verify(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, result monorepo.Result, ref string) monorepo.Verification {
			asked = append(asked, result.Module+"@"+ref)
			return answer(result, ref)
		},
	).AnyTimes()
	return verifier, &asked
}

func identical(monorepo.Result, string) monorepo.Verification {
	return monorepo.Verification{Verdict: monorepo.VerdictIdentical, Reason: "trees are equal"}
}

// identicalMonorepo returns a verifier that proves every move.
func identicalMonorepo(t *testing.T) *monorepo.MockVerifier {
	t.Helper()
	verifier, _ := recordingMonorepo(t, identical)
	return verifier
}

// runMonorepo runs the rule on a copy of the monorepo scenario with the given verifier.
func runMonorepo(t *testing.T, root string, verifier monorepo.Verifier, o *Options) Report {
	t.Helper()
	e := testEngine(t)
	e.Monorepo = verifier
	o.Root = root
	o.Only = []string{ruleComponentMonorepo}
	r, err := e.Run(context.Background(), *o)
	require.NoError(t, err)
	return r
}

// monorepoStatuses maps "<file>:<path>" to the status of the finding there.
func monorepoStatuses(r *Report) map[string]string {
	statuses := map[string]string{}
	findings := ecFindings(r, ruleComponentMonorepo)
	for i := range findings {
		f := &findings[i]
		statuses[f.File+":"+f.Path] = f.Status
	}
	return statuses
}

const (
	vpcSource      = "components/terraform/vpc/component.yaml:spec.source.uri"
	ssoSource      = "components/terraform/aws-sso/component.yaml:spec.source.uri"
	dnsSource      = "vendor.yaml:spec.sources[0].source"
	tgwSource      = "vendor.yaml:spec.sources[1].source"
	efsSource      = "vendor.yaml:spec.sources[2].source"
	customSource   = "vendor.yaml:spec.sources[3].source"
	importedSource = "vendor/networking.yaml:spec.sources[0].source"
	stackSource    = "stacks/deploy/dev.yaml:components.terraform.ecr.source.uri"
)

// movedSources are the sources of the monorepo scenario that doctor can prove and move.
var movedSources = []string{vpcSource, ssoSource, dnsSource, importedSource, stackSource}

func TestMonorepoScenarioDiagnosis(t *testing.T) {
	root := copyScenario(t, "monorepo")
	verifier, asked := recordingMonorepo(t, identical)

	r := runMonorepo(t, root, verifier, &Options{})

	statuses := monorepoStatuses(&r)
	require.Len(t, statuses, 8, "every monorepo source of the scenario is reported once, the untouched one is not")
	for _, location := range movedSources {
		assert.Equal(t, "available", statuses[location], location)
	}
	for _, location := range []string{tgwSource, efsSource, customSource} {
		assert.Equal(t, statusManual, statuses[location], location)
	}
	assert.ElementsMatch(t, []string{
		"vpc@1.495.0", "aws-sso@1.495.0", "dns-primary@1.495.0", "vpc-peering@1.495.0", "ecr@1.495.0",
	}, *asked, "components that cannot move are never looked up")
	assert.Empty(t, r.ChangedFiles, "a diagnosis writes nothing")
	assert.Equal(t, "findings", ecCheck(&r, ruleComponentMonorepo).Status)

	byLocation := map[string]Finding{}
	for _, f := range ecFindings(&r, ruleComponentMonorepo) {
		byLocation[f.File+":"+f.Path] = f
	}
	sso := byLocation[ssoSource]
	assert.True(t, sso.Fixable)
	assert.Equal(t, ruleModernization, sso.Category)
	assert.Equal(t, validation.SeverityWarning, sso.Severity)
	assert.Equal(t, "Moves to cloudposse-terraform-components/aws-identity-center at the same version (files verified identical)", sso.Remediation)
	assert.Equal(t, 8, sso.Line)

	tgw := byLocation[tgwSource]
	assert.False(t, tgw.Fixable)
	assert.Contains(t, tgw.Message, "modules/tgw was split into separate repositories")
	assert.Contains(t, tgw.Reason, "aws-tgw-hub, aws-tgw-hub-connector, aws-tgw-spoke")
	assert.Contains(t, tgw.Remediation, monorepo.MigrationGuideURL)

	efs := byLocation[efsSource]
	assert.Contains(t, efs.Message, "modules/eks/efs has no successor repository")
	assert.Contains(t, efs.Reason, "removed from the monorepo before the migration")

	custom := byLocation[customSource]
	assert.Contains(t, custom.Reason, `"CUSTOM, > 1.422" is not a single tag`)
	assert.Contains(t, custom.Remediation, "Pin the version to one tag")
}

func TestMonorepoScenarioFixRewritesOnlyTheRepositoryAndPath(t *testing.T) {
	root := copyScenario(t, "monorepo")
	original := map[string]string{}
	for _, name := range []string{"components/terraform/vpc/component.yaml", "components/terraform/aws-sso/component.yaml", "vendor.yaml", "vendor/networking.yaml", "stacks/deploy/dev.yaml", "atmos.yaml"} {
		original[name] = readScenarioFile(t, root, name)
	}
	verifier, asked := recordingMonorepo(t, identical)

	r := runMonorepo(t, root, verifier, &Options{Fix: true})

	assert.ElementsMatch(t, []string{
		"components/terraform/vpc/component.yaml", "components/terraform/aws-sso/component.yaml",
		"vendor.yaml", "vendor/networking.yaml", "stacks/deploy/dev.yaml",
	}, r.ChangedFiles)
	for _, location := range movedSources {
		assert.Equal(t, statusFixed, monorepoStatuses(&r)[location], location)
	}
	assert.Len(t, *asked, 5)

	// Each file differs from its original in exactly the lines below: the comments, quoting,
	// versions, targets, and every other source stay as written.
	for name, replacements := range map[string][][2]string{
		"components/terraform/vpc/component.yaml": {{
			"github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{ .Version }} # keep this note",
			"github.com/cloudposse-terraform-components/aws-vpc.git//src?ref={{ .Version }} # keep this note",
		}},
		"components/terraform/aws-sso/component.yaml": {{
			`"github.com/cloudposse/terraform-aws-components.git//modules/aws-sso?ref={{.Version}}"`,
			`"github.com/cloudposse-terraform-components/aws-identity-center.git//src?ref={{.Version}}"`,
		}},
		"vendor.yaml": {{
			`"git::https://github.com/cloudposse/terraform-aws-components.git//modules/dns-primary?depth=1&ref=1.495.0"`,
			`"git::https://github.com/cloudposse-terraform-components/aws-dns-primary.git//src?depth=1&ref=1.495.0"`,
		}},
		"vendor/networking.yaml": {{
			`"github.com/cloudposse/terraform-aws-components.git//modules/vpc-peering?ref={{.Version}}"`,
			`"github.com/cloudposse-terraform-components/aws-vpc-peering.git//src?ref={{.Version}}"`,
		}},
		"stacks/deploy/dev.yaml": {{
			"github.com/cloudposse/terraform-aws-components.git//modules/ecr?ref={{.Version}}",
			"github.com/cloudposse-terraform-components/aws-ecr.git//src?ref={{.Version}}",
		}},
	} {
		want := original[name]
		for _, pair := range replacements {
			require.Contains(t, want, pair[0], name)
			want = strings.Replace(want, pair[0], pair[1], 1)
		}
		assert.Equal(t, want, readScenarioFile(t, root, name), name)
	}
	assert.Equal(t, original["atmos.yaml"], readScenarioFile(t, root, "atmos.yaml"))

	// The sources that cannot move are reported again, untouched, and the second run finds nothing to repair.
	again := runMonorepo(t, root, verifier, &Options{Fix: true})
	assert.Empty(t, again.ChangedFiles)
	assert.Len(t, *asked, 5, "nothing is verified again once the sources moved")
	statuses := monorepoStatuses(&again)
	assert.Len(t, statuses, 3)
	for _, location := range []string{tgwSource, efsSource, customSource} {
		assert.Equal(t, statusManual, statuses[location], location)
	}
}

func TestMonorepoDryRunLeavesSourcesUnchanged(t *testing.T) {
	root := copyScenario(t, "monorepo")
	before := readScenarioFile(t, root, "vendor.yaml")

	r := runMonorepo(t, root, identicalMonorepo(t), &Options{Fix: true, DryRun: true})

	assert.Empty(t, r.ChangedFiles)
	assert.Equal(t, before, readScenarioFile(t, root, "vendor.yaml"))
	for _, location := range movedSources {
		assert.Equal(t, "available", monorepoStatuses(&r)[location], location)
	}
	for _, f := range ecFindings(&r, ruleComponentMonorepo) {
		if f.Fixable {
			assert.Equal(t, "Candidate verified; dry run left the source unchanged", f.Reason)
		}
	}
}

func TestMonorepoVerificationFailureWritesNothing(t *testing.T) {
	root := copyScenario(t, "monorepo")
	before := readScenarioFile(t, root, "vendor.yaml")
	differs := func(result monorepo.Result, _ string) monorepo.Verification {
		if result.Module == "dns-primary" {
			return monorepo.Verification{Verdict: monorepo.VerdictDiffers, Reason: "trees differ"}
		}
		return identical(result, "")
	}
	verifier, _ := recordingMonorepo(t, differs)

	r := runMonorepo(t, root, verifier, &Options{Fix: true})

	statuses := monorepoStatuses(&r)
	assert.Equal(t, statusVerificationFailed, statuses[dnsSource])
	assert.Equal(t, statusFixed, statuses[vpcSource], "a failed component does not stop the proven ones")
	assert.Equal(t, before, readScenarioFile(t, root, "vendor.yaml"), "the file with the failed source is untouched")
	assert.NotContains(t, r.ChangedFiles, "vendor.yaml")
	for _, f := range ecFindings(&r, ruleComponentMonorepo) {
		if f.Path == "spec.sources[0].source" && f.File == "vendor.yaml" {
			assert.False(t, f.Fixable)
			assert.Equal(t, "trees differ", f.Reason)
			assert.Contains(t, f.Remediation, "github.com/cloudposse-terraform-components/aws-dns-primary.git//src?depth=1&ref=1.495.0")
		}
	}
	assert.Equal(t, 1, r.ExitCode("error"), "a repair that failed verification is not a clean run")
}

func TestMonorepoUnverifiedWritesNothing(t *testing.T) {
	unverified := func(monorepo.Result, string) monorepo.Verification {
		return monorepo.Verification{Verdict: monorepo.VerdictUnverified, Reason: "GitHub rate limit exceeded"}
	}
	tests := []struct {
		name    string
		options Options
		reason  string
		asks    bool
	}{
		{"rate limited", Options{Fix: true}, "GitHub rate limit exceeded", true},
		{"offline", Options{Fix: true, Offline: true}, monorepoOfflineNote, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := copyScenario(t, "monorepo")
			before := readScenarioFile(t, root, "components/terraform/vpc/component.yaml")
			verifier, asked := recordingMonorepo(t, unverified)

			r := runMonorepo(t, root, verifier, &tt.options)

			for _, location := range movedSources {
				assert.Equal(t, statusVerificationIncomplete, monorepoStatuses(&r)[location], location)
			}
			for _, f := range ecFindings(&r, ruleComponentMonorepo) {
				if f.Status == statusVerificationIncomplete {
					assert.Equal(t, tt.reason, f.Reason)
					assert.False(t, f.Fixable)
				}
			}
			assert.Empty(t, r.ChangedFiles)
			assert.Equal(t, before, readScenarioFile(t, root, "components/terraform/vpc/component.yaml"))
			assert.Equal(t, tt.asks, len(*asked) > 0)
			check := ecCheck(&r, ruleComponentMonorepo)
			assert.Equal(t, "incomplete", check.Status)
			assert.Equal(t, "Some component sources could not be verified", check.Reason)
		})
	}
}

func TestMonorepoVerifiesEachComponentAndRefOnce(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	source := "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"
	projectFile(t, root, "vendor.yaml", "spec:\n  sources:\n    - component: a\n      source: "+source+"\n      version: 1.0.0\n    - component: b\n      source: "+source+"\n      version: 1.0.0\n    - component: c\n      source: "+source+"\n      version: 2.0.0\n")
	verifier, asked := recordingMonorepo(t, identical)

	r := runMonorepo(t, root, verifier, &Options{})

	assert.Len(t, ecFindings(&r, ruleComponentMonorepo), 3)
	assert.Equal(t, []string{"vpc@1.0.0", "vpc@2.0.0"}, *asked)
}

func TestMonorepoSelection(t *testing.T) {
	for _, tt := range []struct {
		only   []string
		checks bool
	}{
		{[]string{ruleComponentMonorepo}, true},
		{[]string{ruleModernization}, true},
		{[]string{categoryDeprecations}, false},
		{[]string{ruleSourceRepository}, false},
	} {
		t.Run(strings.Join(tt.only, ","), func(t *testing.T) {
			root := copyScenario(t, "monorepo")
			e := testEngine(t)
			if tt.checks {
				e.Monorepo = identicalMonorepo(t)
			} else {
				e.Monorepo = monorepo.NewMockVerifier(gomock.NewController(t))
			}
			e.Repositories = &repositoryStub{health: RepositoryHealth{"healthy", "ok"}}
			r, err := e.Run(context.Background(), Options{Root: root, Only: tt.only})
			require.NoError(t, err)
			assert.Equal(t, tt.checks, len(ecFindings(&r, ruleComponentMonorepo)) > 0)
			assert.Equal(t, tt.checks, ecCheck(&r, ruleComponentMonorepo) != nil)
		})
	}

	t.Run("the rule is listed under the modernization category", func(t *testing.T) {
		var listed *Rule
		for _, rule := range Rules() {
			if rule.ID == ruleComponentMonorepo {
				listed = &rule
			}
		}
		require.NotNil(t, listed)
		assert.Equal(t, ruleModernization, listed.Category)
		assert.Contains(t, listed.Description, "same version")
	})
}

func TestMonorepoMixinsAreReportedNotRewritten(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	content := "kind: ComponentVendorConfig\nspec:\n  source:\n    uri: github.com/cloudposse-terraform-components/aws-vpc.git//src?ref=1.0.0\n  mixins:\n    - uri: https://raw.githubusercontent.com/cloudposse/terraform-aws-components/1.0.0/mixins/provider-aws.tf\n      filename: providers.tf\n"
	projectFile(t, root, "components/terraform/vpc/component.yaml", content)

	r := runMonorepo(t, root, monorepo.NewMockVerifier(gomock.NewController(t)), &Options{Fix: true})

	found := ecFindings(&r, ruleComponentMonorepo)
	require.Len(t, found, 1)
	assert.Equal(t, statusManual, found[0].Status)
	assert.Equal(t, "spec.mixins[0].uri", found[0].Path)
	assert.Equal(t, 6, found[0].Line)
	assert.Contains(t, found[0].Message, "Mixin")
	assert.Equal(t, content, readScenarioFile(t, root, "components/terraform/vpc/component.yaml"))
}

func TestMonorepoSourceFormsDoctorReportsByHand(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor.yaml", strings.Join([]string{
		"spec:",
		"  sources:",
		"    - component: ssh",
		"      source: git@github.com:cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
		"    - component: whole",
		"      source: github.com/cloudposse/terraform-aws-components.git?ref=1.0.0",
		"    - component: dynamic-path",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/{{ .name }}?ref=1.0.0",
		"    - component: dynamic-ref",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{ .tag }}",
		"    - component: no-ref",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc",
		"    - component: no-version",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
		"    - component: templated-version",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
		"      version: '{{ .tag }}'",
		"    - component: range",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
		"      version: '^1.495.0'",
		"",
	}, "\n"))

	r := runMonorepo(t, root, monorepo.NewMockVerifier(gomock.NewController(t)), &Options{Fix: true})

	found := ecFindings(&r, ruleComponentMonorepo)
	require.Len(t, found, 8)
	wants := []string{
		"written in a form doctor cannot rewrite",
		"selects no module",
		"module path is a template",
		"template that doctor cannot evaluate",
		"pins no ref",
		"declares no version next to it",
		"version is dynamic",
		`"^1.495.0" is not a single tag`,
	}
	for i, f := range found {
		assert.Equal(t, statusManual, f.Status, "%s: %s", f.Path, f.Reason)
		assert.False(t, f.Fixable, f.Path)
		assert.Contains(t, f.Message+f.Reason, wants[i], f.Path)
	}
	assert.Empty(t, r.ChangedFiles)
}

func TestMonorepoStackSourceWithoutVersionIsReportedByHand(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "stacks/dev.yaml", "components:\n  terraform:\n    vpc:\n      source: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}\n")

	r := runMonorepo(t, root, monorepo.NewMockVerifier(gomock.NewController(t)), &Options{})

	found := ecFindings(&r, ruleComponentMonorepo)
	require.Len(t, found, 1)
	assert.Equal(t, statusManual, found[0].Status)
	assert.Contains(t, found[0].Reason, "declares no version")
}

func TestMonorepoDynamicEnclosingNodeBlocksTheMove(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	content := "components:\n  terraform:\n    ecr:\n      source: !merge\n        uri: github.com/cloudposse/terraform-aws-components.git//modules/ecr?ref=1.0.0\n"
	projectFile(t, root, "stacks/dev.yaml", content)

	r := runMonorepo(t, root, monorepo.NewMockVerifier(gomock.NewController(t)), &Options{Fix: true})

	found := ecFindings(&r, ruleComponentMonorepo)
	require.Len(t, found, 1)
	assert.Equal(t, statusVerificationIncomplete, found[0].Status)
	assert.Contains(t, found[0].Reason, "YAML function !merge")
	assert.False(t, found[0].Fixable)
	assert.Equal(t, content, readScenarioFile(t, root, "stacks/dev.yaml"))
}

func TestMonorepoSemanticVerificationAcceptsOnlyTheRepositoryMove(t *testing.T) {
	const from = "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0"
	const to = "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref=1.0.0"
	change := edit{kind: ruleComponentMonorepo, path: []string{"spec", "source", "uri"}, from: from, value: to}
	document := func(uri string) []byte {
		return []byte("spec:\n  source:\n    uri: " + uri + "\n    version: 1.0.0\n")
	}

	tests := []struct {
		name  string
		after []byte
		edit  edit
		ok    bool
	}{
		{"the exact move", document(to), change, true},
		{"another repository", document(strings.Replace(to, "aws-vpc", "aws-ecr", 1)), change, false},
		{"another version", document(strings.Replace(to, "1.0.0", "2.0.0", 1)), change, false},
		{"another path", document(strings.Replace(to, "//src", "//other", 1)), change, false},
		{"a changed sibling", []byte("spec:\n  source:\n    uri: " + to + "\n    version: 2.0.0\n"), change, false},
		{"a recorded change that is not the move", document(to), edit{kind: ruleComponentMonorepo, path: change.path, from: from, value: "github.com/example/x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyDocument(document(from), tt.after, []edit{tt.edit})
			if tt.ok {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrVerification)
		})
	}

	t.Run("comments on the edited line are carried", func(t *testing.T) {
		before := []byte("spec:\n  source:\n    uri: " + from + " # pinned\n")
		require.NoError(t, verifyDocument(before, []byte("spec:\n  source:\n    uri: "+to+" # pinned\n"), []edit{change}))
		require.ErrorIs(t, verifyDocument(before, []byte("spec:\n  source:\n    uri: "+to+"\n"), []edit{change}), ErrVerification)
		require.ErrorIs(t, verifyDocument(before, []byte("spec:\n  source:\n    uri: "+to+" # changed\n"), []edit{change}), ErrVerification)
	})
}

func TestMonorepoRefResolution(t *testing.T) {
	vendor := func(content string) *document {
		node, err := parse([]byte(content))
		require.NoError(t, err)
		return &document{kind: documentVendor, node: node}
	}
	versioned := vendor("spec:\n  sources:\n    - source: x\n      version: 1.2.3\n")
	result := func(source string) *monorepo.Result {
		moved, ok := monorepo.Rewrite(source)
		require.True(t, ok)
		return &moved
	}

	t.Run("a literal ref is used as written", func(t *testing.T) {
		ref, err := monorepoRef(versioned, []string{"spec", "sources", "0", "source"}, result("github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=v1.0.0"))
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", ref)
	})
	t.Run("the placeholder takes the version beside the source", func(t *testing.T) {
		ref, err := monorepoRef(versioned, []string{"spec", "sources", "0", "source"}, result("github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"))
		require.NoError(t, err)
		assert.Equal(t, "1.2.3", ref)
	})
	t.Run("an unusable ref is refused", func(t *testing.T) {
		for _, source := range []string{
			"github.com/cloudposse/terraform-aws-components.git//modules/vpc",
			"github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{ .x }}",
			"github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=a%20b",
		} {
			_, err := monorepoRef(versioned, []string{"spec", "sources", "0", "source"}, result(source))
			require.ErrorIs(t, err, ErrMonorepoVersion, source)
		}
	})
}

func TestMonorepoSiteClassification(t *testing.T) {
	for _, tt := range []struct {
		kind     string
		path     []string
		want     siteKind
		relevant bool
	}{
		{documentVendor, []string{"spec", "source", "uri"}, siteSource, true},
		{documentVendor, []string{"spec", "sources", "3", "source"}, siteSource, true},
		{documentVendor, []string{"spec", "mixins", "0", "uri"}, siteMixin, true},
		{documentVendor, []string{"spec", "other", "source"}, siteOther, true},
		{"stack", []string{"components", "terraform", "vpc", "source"}, siteSource, true},
		{"stack", []string{"components", "terraform", "vpc", "source", "uri"}, siteSource, true},
		{"stack", []string{"components", "terraform", "vars", "settings", "source"}, siteOther, true},
		{"stack", []string{"components", "terraform", "vpc", "vars", "repo"}, siteOther, false},
		{"stack", []string{"spec", "source", "uri"}, siteOther, true},
	} {
		got, relevant := classifySite(tt.kind, tt.path)
		assert.Equal(t, tt.relevant, relevant, "%s %v", tt.kind, tt.path)
		if tt.relevant {
			assert.Equal(t, tt.want, got, "%s %v", tt.kind, tt.path)
		}
	}
}

func TestMonorepoSourceSpellingAndDeepPaths(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	projectFile(t, root, "vendor.yaml", strings.Join([]string{
		"spec:",
		"  sources:",
		"    - component: acm",
		"      source: GitHub.com/CloudPosse/terraform-aws-components.git//modules/acm?ref=1.495.0",
		"    - component: foo",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/eks/cluster/modules/foo?ref=1.495.0",
		"    - component: bar",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/eks/cluster/modules/bar?ref=1.495.0",
		"    - component: alb",
		"      source: git::https://github.com/cloudposse/terraform-aws-components.git?ref=1.495.0//modules/alb",
		"    - component: removed",
		"      source: github.com/cloudposse/terraform-aws-components.git//modules/eks/efs/modules/x?ref=1.495.0",
		"",
	}, "\n"))
	verifier, asked := recordingMonorepo(t, identical)

	r := runMonorepo(t, root, verifier, &Options{Fix: true})

	statuses := monorepoStatuses(&r)
	assert.Equal(t, statusFixed, statuses["vendor.yaml:spec.sources[0].source"], "mixed-case host and owner are detected")
	assert.Equal(t, statusFixed, statuses["vendor.yaml:spec.sources[1].source"])
	assert.Equal(t, statusFixed, statuses["vendor.yaml:spec.sources[2].source"])
	assert.Equal(t, statusManual, statuses["vendor.yaml:spec.sources[3].source"])
	assert.Equal(t, statusManual, statuses["vendor.yaml:spec.sources[4].source"])
	assert.Equal(t, []string{"acm@1.495.0", "eks/cluster@1.495.0"}, *asked,
		"the parent module is verified once for every directory inside it")

	content := readScenarioFile(t, root, "vendor.yaml")
	assert.Contains(t, content, "GitHub.com/cloudposse-terraform-components/aws-acm.git//src?ref=1.495.0")
	assert.Contains(t, content, "github.com/cloudposse-terraform-components/aws-eks-cluster.git//src/modules/foo?ref=1.495.0")
	assert.Contains(t, content, "github.com/cloudposse-terraform-components/aws-eks-cluster.git//src/modules/bar?ref=1.495.0")
	assert.Contains(t, content, "git::https://github.com/cloudposse/terraform-aws-components.git?ref=1.495.0//modules/alb",
		"a subdirectory inside the query is guidance only and is never rewritten")

	found := ecFindings(&r, ruleComponentMonorepo)
	var alb, removed *Finding
	for i := range found {
		switch found[i].Path {
		case "spec.sources[3].source":
			alb = &found[i]
		case "spec.sources[4].source":
			removed = &found[i]
		}
	}
	require.NotNil(t, alb)
	assert.False(t, alb.Fixable)
	assert.Contains(t, alb.Reason, "subdirectory is inside the query string")
	assert.Contains(t, alb.Reason, "before the ?")
	assert.Contains(t, alb.Remediation, "git::https://github.com/cloudposse-terraform-components/aws-alb.git//src?ref=1.495.0")
	require.NotNil(t, removed)
	assert.Contains(t, removed.Message, "has no successor repository")
}
