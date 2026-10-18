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
		subpath    string
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
			name:   "mixed-case host, owner, and repository keep the host spelling",
			source: "GitHub.com/CloudPosse/terraform-aws-components.git//modules/acm?ref=1.495.0",
			want:   "GitHub.com/cloudposse-terraform-components/aws-acm.git//src?ref=1.495.0",
			module: "acm", repository: "aws-acm", ref: "1.495.0", kind: RefLiteral,
		},
		{
			name:   "mixed-case scheme and upper-case .git keep their spelling",
			source: "Git::HTTPS://GITHUB.COM/CLOUDPOSSE/Terraform-AWS-Components.GIT//modules/vpc?ref=1.0.0",
			want:   "Git::HTTPS://GITHUB.COM/cloudposse-terraform-components/aws-vpc.GIT//src?ref=1.0.0",
			module: "vpc", repository: "aws-vpc", ref: "1.0.0", kind: RefLiteral,
		},
		{
			name:   "directory inside a mapped module moves below src",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/eks/cluster/modules/foo?ref=1.495.0",
			want:   "github.com/cloudposse-terraform-components/aws-eks-cluster.git//src/modules/foo?ref=1.495.0",
			module: "eks/cluster", repository: "aws-eks-cluster", ref: "1.495.0", kind: RefLiteral, subpath: "modules/foo",
		},
		{
			name:   "directory inside a module with a trailing slash and the version placeholder",
			source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc/modules/a/b/?ref={{.Version}}",
			want:   "github.com/cloudposse-terraform-components/aws-vpc.git//src/modules/a/b?ref={{.Version}}",
			module: "vpc", repository: "aws-vpc", ref: "{{.Version}}", kind: RefVersion, subpath: "modules/a/b",
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
			assert.Equal(t, tt.subpath, got.Subpath)

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
		{"subdirectory inside the query", "github.com/cloudposse/terraform-aws-components.git?ref=1.495.0//modules/alb", true, ErrSubdirInQuery},
		{"subdirectory inside the query with other parameters", "git::https://github.com/cloudposse/terraform-aws-components.git?depth=1&ref=1.495.0//modules/eks/cluster", true, ErrSubdirInQuery},
		{"unmapped parent keeps a deep path without a successor", "github.com/cloudposse/terraform-aws-components.git//modules/eks/efs/modules/x?ref=1.0.0", true, ErrNoSuccessor},
		{"parent traversal is never a module", "github.com/cloudposse/terraform-aws-components.git//modules/vpc/../eks/efs?ref=1.0.0", true, ErrNoSuccessor},
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

func TestQuerySubdirCorrection(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{"successor exists", "github.com/cloudposse/terraform-aws-components.git?ref=1.495.0//modules/alb", "github.com/cloudposse-terraform-components/aws-alb.git//src?ref=1.495.0"},
		{"other parameters are kept in order", "git::https://github.com/cloudposse/terraform-aws-components.git?depth=1&ref=1.495.0//modules/alb&sshkey=x", "git::https://github.com/cloudposse-terraform-components/aws-alb.git//src?depth=1&ref=1.495.0&sshkey=x"},
		{"trailing slash", "github.com/cloudposse/terraform-aws-components.git?ref=1.495.0//modules/alb/", "github.com/cloudposse-terraform-components/aws-alb.git//src?ref=1.495.0"},
		{"no successor keeps the monorepo source", "github.com/cloudposse/terraform-aws-components.git?ref=1.0.0//modules/eks/efs", "github.com/cloudposse/terraform-aws-components.git//modules/eks/efs?ref=1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reference, ok := Parse(tt.source)
			require.True(t, ok)
			got, ok := reference.QuerySubdirCorrection()
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	reference, ok := Parse("github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0")
	require.True(t, ok)
	_, ok = reference.QuerySubdirCorrection()
	assert.False(t, ok, "a source with its subdirectory in the right place needs no correction")
}

func TestParentModule(t *testing.T) {
	tests := []struct {
		module, parent, subpath string
		ok                      bool
	}{
		{"eks/cluster/modules/foo", "eks/cluster", "modules/foo", true},
		{"vpc/modules/a/b", "vpc", "modules/a/b", true},
		{"vpc", "", "", false},
		{"eks/efs", "", "", false},
		{"eks/karpenter-node-pool/modules/x", "eks/karpenter-node-pool", "modules/x", true},
		{"eks/karpenter/modules/x", "eks/karpenter", "modules/x", true},
		{"eks/karpenter-extra/x", "", "", false},
		{"vpc/../x", "", "", false},
		{"vpc//x", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			parent, subpath, ok := ParentModule(tt.module)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.parent, parent)
			assert.Equal(t, tt.subpath, subpath)
		})
	}
}

func TestMentions(t *testing.T) {
	assert.True(t, Mentions("GitHub.com/CloudPosse/Terraform-AWS-Components.git//modules/acm?ref=1.495.0"))
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
