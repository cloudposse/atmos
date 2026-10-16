package monorepo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRewriteForms(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		want       string
		module     string
		repository string
		ref        string
		kind       RefKind
	}{
		{
			name:   "bare host with .git and the version template",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref={{.Version}}",
			module: "vpc", repository: "aws-vpc", ref: "{{.Version}}", kind: RefVersion,
		},
		{
			name:   "spaced version template",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{ .Version }}",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref={{ .Version }}",
			module: "vpc", repository: "aws-vpc", ref: "{{ .Version }}", kind: RefVersion,
		},
		{
			name:   "https scheme without .git and a literal ref",
			source: "https://github.com/cloudposse/terraform-aws-components//modules/vpc?ref=1.495.0",
			want:   "https://github.com/cloudposse-terraform-components/aws-vpc//src?ref=1.495.0",
			module: "vpc", repository: "aws-vpc", ref: "1.495.0", kind: RefLiteral,
		},
		{
			name:   "git forced getter keeps other query parameters in order",
			source: "git::https://github.com/cloudposse/terraform-aws-components.git//modules/vpc?depth=1&ref=1.495.0",
			want:   "git::https://github.com/cloudposse-terraform-components/aws-vpc.git//src?depth=1&ref=1.495.0",
			module: "vpc", repository: "aws-vpc", ref: "1.495.0", kind: RefLiteral,
		},
		{
			name:   "nested module path",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/eks/cluster?ref=1.2.3&depth=1",
			want:   "github.com/cloudposse-terraform-components/aws-eks-cluster.git//src?ref=1.2.3&depth=1",
			module: "eks/cluster", repository: "aws-eks-cluster", ref: "1.2.3", kind: RefLiteral,
		},
		{
			name:   "trailing slash on the module path",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc/?ref=1.2.3",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref=1.2.3",
			module: "vpc", repository: "aws-vpc", ref: "1.2.3", kind: RefLiteral,
		},
		{
			name:   "irregular name aws-sso",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/aws-sso?ref={{.Version}}",
			want:   "github.com/cloudposse-terraform-components/aws-identity-center.git//src?ref={{.Version}}",
			module: "aws-sso", repository: "aws-identity-center", ref: "{{.Version}}", kind: RefVersion,
		},
		{
			name:   "irregular name argocd-repo",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/argocd-repo?ref=1.0.0",
			want:   "github.com/cloudposse-terraform-components/aws-argocd-github-repo.git//src?ref=1.0.0",
			module: "argocd-repo", repository: "aws-argocd-github-repo", ref: "1.0.0", kind: RefLiteral,
		},
		{
			name:   "irregular name datadog-configuration",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/datadog-configuration?ref=1.0.0",
			want:   "github.com/cloudposse-terraform-components/aws-datadog-credentials.git//src?ref=1.0.0",
			module: "datadog-configuration", repository: "aws-datadog-credentials", ref: "1.0.0", kind: RefLiteral,
		},
		{
			name:   "irregular nested name tgw cross-region-hub-connector",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/tgw/cross-region-hub-connector?ref=1.0.0",
			want:   "github.com/cloudposse-terraform-components/aws-tgw-hub-connector.git//src?ref=1.0.0",
			module: "tgw/cross-region-hub-connector", repository: "aws-tgw-hub-connector", ref: "1.0.0", kind: RefLiteral,
		},
		{
			name:   "irregular nested name spacelift worker-pool",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/spacelift/worker-pool?ref=1.0.0",
			want:   "github.com/cloudposse-terraform-components/aws-spacelift-worker-pool-asg.git//src?ref=1.0.0",
			module: "spacelift/worker-pool", repository: "aws-spacelift-worker-pool-asg", ref: "1.0.0", kind: RefLiteral,
		},
		{
			name:   "no ref at all",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src",
			module: "vpc", repository: "aws-vpc", kind: RefNone,
		},
		{
			name:   "ref that is another template",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{ .vars.tag }}",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref={{ .vars.tag }}",
			module: "vpc", repository: "aws-vpc", ref: "{{ .vars.tag }}", kind: RefDynamic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Rewrite(tt.source)
			require.True(t, ok)
			assert.Equal(t, tt.want, got.Source)
			assert.Equal(t, tt.module, got.Module)
			assert.Equal(t, tt.repository, got.Repository)
			assert.Equal(t, "cloudposse-terraform-components/"+tt.repository, got.Slug())
			assert.Equal(t, tt.ref, got.Ref)
			assert.Equal(t, tt.kind, got.RefKind)

			again, ok := Rewrite(got.Source)
			assert.False(t, ok, "a rewritten source must not be rewritten again: %+v", again)
		})
	}
}

func TestRewriteRefusals(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		parses  bool
		wantErr error
	}{
		{"parent that was split into subcomponents", "github.com/cloudposse/terraform-aws-components.git//modules/tgw?ref=1.0.0", true, ErrSplitComponent},
		{"other split parent", "github.com/cloudposse/terraform-aws-components.git//modules/spacelift?ref=1.0.0", true, ErrSplitComponent},
		{"module removed before the migration", "github.com/cloudposse/terraform-aws-components.git//modules/eks/efs?ref=1.0.0", true, ErrNoSuccessor},
		{"module without a mapping", "github.com/cloudposse/terraform-aws-components.git//modules/eks/efs-controller?ref=1.0.0", true, ErrNoSuccessor},
		{"templated module path", "github.com/cloudposse/terraform-aws-components.git//modules/{{ .component }}?ref=1.0.0", true, ErrTemplatedModule},
		{"whole repository", "github.com/cloudposse/terraform-aws-components.git?ref=1.0.0", true, ErrNoModule},
		{"subdirectory outside modules", "github.com/cloudposse/terraform-aws-components.git//mixins?ref=1.0.0", true, ErrNoModule},
		{"unrelated repository", "github.com/cloudposse/terraform-null-label.git?ref=0.25.0", false, nil},
		{"successor repository", "github.com/cloudposse-terraform-components/aws-vpc.git//src?ref=1.0.0", false, nil},
		{"repository that only shares the prefix", "github.com/cloudposse/terraform-aws-components-fork.git//modules/vpc?ref=1.0.0", false, nil},
		{"templated repository", "github.com/cloudposse/{{ .repo }}.git//modules/vpc?ref=1.0.0", false, nil},
		{"other host", "gitlab.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0", false, nil},
		{"raw content URL", "https://raw.githubusercontent.com/cloudposse/terraform-aws-components/1.0.0/mixins/provider.tf", false, nil},
		{"single slash subdirectory", "github.com/cloudposse/terraform-aws-components.git/modules/vpc?ref=1.0.0", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := Rewrite(tt.source)
			assert.False(t, ok)
			reference, parsed := Parse(tt.source)
			assert.Equal(t, tt.parses, parsed)
			if !parsed {
				return
			}
			_, err := reference.Rewrite()
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestMentions(t *testing.T) {
	assert.True(t, Mentions("https://raw.githubusercontent.com/cloudposse/terraform-aws-components/1.0.0/mixins/provider.tf"))
	assert.True(t, Mentions("git@github.com:cloudposse/terraform-aws-components.git"))
	assert.False(t, Mentions("github.com/cloudposse/terraform-null-label.git"))
}

func TestSuccessors(t *testing.T) {
	assert.Equal(t, []string{"aws-tgw-hub", "aws-tgw-hub-connector", "aws-tgw-spoke"}, Successors("tgw"))
	assert.Equal(t, []string{"aws-spacelift-admin-stack", "aws-spacelift-spaces", "aws-spacelift-worker-pool-asg"}, Successors("spacelift"))
	assert.Empty(t, Successors("vpc"))
}

// TestEmbeddedMap pins the shape of the shipped map so a bad edit fails the build, not a user's run.
func TestEmbeddedMap(t *testing.T) {
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(componentsYAML, &raw))
	m := components()
	assert.Equal(t, "aws", m.RepoSettings.Prefix)
	require.Len(t, m.Components, 158, "the published map has 158 components")

	for module, suffix := range m.Components {
		repository, ok := Repository(module)
		require.True(t, ok, module)
		assert.NotContains(t, repository, "/", module)
		assert.Equal(t, "aws-", repository[:4], module)
		assert.NotEmpty(t, suffix, module)
	}
	_, ok := Repository("eks/efs")
	assert.False(t, ok)
}
