package adoption

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func guidanceFile(t *testing.T, root, ref, content string) string {
	t.Helper()
	name := filepath.Join(root, ref)
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
	require.NoError(t, os.WriteFile(name, []byte(content), 0o600))
	return name
}

type guidedAdapter string

func (a guidedAdapter) Kind() string                                       { return string(a) }
func (a guidedAdapter) Options() []string                                  { return nil }
func (a guidedAdapter) Adopt(context.Context, *Registration, string) error { return nil }
func (a guidedAdapter) Detect(s Source) (Match, error) {
	filename := map[string]string{"helm": "Chart.yaml", "wrapper": "wrap.yaml", "terraform": "main.tf"}[string(a)]
	if !s.IsDir && filepath.Base(s.Path) == filename {
		return Match{Path: s.Path, Wrapper: a == "wrapper"}, nil
	}
	for _, f := range s.Files {
		if f == filename && a != "wrapper" {
			return Match{Path: s.Path}, nil
		}
	}
	return Match{}, nil
}

func TestDiscoverLocalProjects(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"provider/main.tf", "chart/Chart.yaml", "chart/charts/child/Chart.yaml", "hooks/wrap.yaml", "provider/wrap.yaml", ".git/hide/main.tf", "vendor/a/main.tf", "one/two/three/four/five/six/main.tf", "unrelated"} {
		guidanceFile(t, root, f, "")
	}
	require.NoError(t, os.Symlink(root, filepath.Join(root, "cycle")))
	adapters := Adapters{guidedAdapter("terraform"), guidedAdapter("helm"), guidedAdapter("wrapper")}
	result, err := adapters.Discover(t.Context(), root)
	require.NoError(t, err)
	var rel []string
	for _, r := range result {
		p, e := filepath.Rel(root, r.Path)
		require.NoError(t, e)
		rel = append(rel, r.Kind+":"+filepath.ToSlash(p))
	}
	require.Equal(t, []string{"helm:chart", "wrapper:hooks/wrap.yaml", "wrapper:provider/wrap.yaml"}, rel)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = adapters.Discover(ctx, root)
	require.ErrorIs(t, err, context.Canceled)
	_, err = adapters.Discover(t.Context(), filepath.Join(root, "missing"))
	require.Error(t, err)
}

func TestVarFileDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"terraform.tfvars", "nested/extra.auto.tfvars.json", "sub/vars.tfvars", "sub/other.tofu.json", "vars/dev.tfvars", "file.pkr.hcl", "base.auto.pkrvars.json", "env/prod.pkrvars.hcl", "native/child.pkr.json", "native/foreign.pkrvars.hcl", ".terraform/no.tfvars", "node_modules/bad.tfvars", "unrelated", "not.auto.pkrvars.hcl~"} {
		guidanceFile(t, root, key, "")
	}
	require.NoError(t, os.Symlink(filepath.Join(root, "terraform.tfvars"), filepath.Join(root, "linked.tfvars")))
	for _, tc := range []struct {
		kind, source string
		auto, files  []string
	}{
		{"terraform", root, []string{"terraform.tfvars"}, []string{"nested/extra.auto.tfvars.json", "vars/dev.tfvars"}},
		{"packer", filepath.Join(root, "file.pkr.hcl"), []string{"base.auto.pkrvars.json"}, []string{"env/prod.pkrvars.hcl"}},
		{"other", root, nil, nil},
	} {
		r, err := DiscoverVarFiles(tc.source, tc.kind)
		require.NoError(t, err)
		require.Equal(t, tc.auto, r.Automatic)
		require.Equal(t, tc.files, r.Optional)
	}
	_, err := DiscoverVarFiles(filepath.Join(root, "missing"), "packer")
	require.Error(t, err)
}

func TestReplaceManagedInputsAndRecoverMissing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "source")
	b := guidanceFile(t, root, "source/b.tfvars", "x=2")
	guidanceFile(t, root, "source/team a.tfvars", "x=1")
	path := filepath.Join(root, "stacks", "dev.yaml")
	target := VarFileTarget{"components.terraform.web", path, dir, []string{".tfvars"}}
	r := Registration{Project: root, Stack: "dev", Dir: dir, Name: "web", Output: io.Discard, VarFiles: []string{"team a.tfvars", "b.tfvars"}}
	base := []byte("# keep this comment\ncomponents:\n  terraform:\n    web:\n      metadata: {component: source}\n")
	data, err := r.AddVarFiles(base, target, false)
	require.NoError(t, err)
	keep, err := r.AddVarFiles(data, target, true)
	require.NoError(t, err)
	require.Equal(t, data, keep)
	guidanceFile(t, root, "stacks/dev.yaml", string(data))
	r.Config = &schema.AtmosConfiguration{}
	r.Config.Stacks.IncludedPaths = []string{"*.yaml"}
	r.Config.StacksBaseAbsolutePath = filepath.Dir(path)
	require.NoError(t, os.Remove(b))
	files, err := r.SavedVarFiles("terraform", "web", dir)
	require.NoError(t, err)
	require.Equal(t, r.VarFiles, files)
	r.VarFiles = []string{"team a.tfvars"}
	_, err = r.AddVarFiles(data, target, true)
	require.ErrorContains(t, err, "--replace-var-files")
	require.ErrorContains(t, err, "--dry-run")
	r.ReplaceVarFiles = true
	replaced, err := r.AddVarFiles(data, target, true)
	require.NoError(t, err)
	require.NotContains(t, string(replaced), "b.tfvars")
	require.Contains(t, string(replaced), "# keep this comment")
	r.VarFiles = nil
	cleared, err := r.AddVarFiles(replaced, target, true)
	require.NoError(t, err)
	require.NotContains(t, string(cleared), "vars:")
	for _, v := range []string{"{custom: keep}", "[!include './x.tfvars | .extra']", "[!include './x.json']", "[!env FOO]", "[!include 'bad.tfvars']"} {
		_, err = r.AddVarFiles([]byte("components: {terraform: {web: {vars: "+v+"}}}"), target, true)
		require.ErrorContains(t, err, "hand-authored")
	}
}

func TestSaveCommandAndPreviewBaseline(t *testing.T) {
	out := new(bytes.Buffer)
	r := Registration{SourcePath: "folder/that's fine", Dir: "other", Stack: "dev", Name: "one", ReleaseName: "canary", Namespace: "preview", Workspace: "blue", Explicit: "tofu", ShareState: true, ReplaceVarFiles: true, VarFiles: []string{"a b", "c"}, Values: []string{"values"}, Output: out}
	var snapshot Registration
	r.OnPreview = func(rr Registration) { snapshot = rr }
	r.PreviewSave()
	for _, piece := range []string{"'folder/that", "-s dev --name one --command tofu", "--release-name canary --namespace preview --workspace blue", "--var-file 'a b' --var-file c --values values --share-state --replace-var-files --interactive=false"} {
		require.Contains(t, out.String(), piece)
	}
	require.Equal(t, r.SaveCommand(), snapshot.SaveCommand())
	r.AtlantisProject = "from-atlantis"
	require.Contains(t, r.SaveCommand(), "--project from-atlantis")
	require.NotContains(t, r.SaveCommand(), "--var-file")
	r.reviewed = map[string]reviewedFile{}
	r.DryRun = true
	require.NoError(t, r.reviewFile("dev", nil))
	r.DryRun = false
	require.NoError(t, r.reviewFile("dev", nil))
	require.ErrorContains(t, r.reviewFile("dev", []byte{}), "changed after preview")
	require.Error(t, r.reviewFile("staging", nil))
}

func TestLiteralIncludeSafety(t *testing.T) {
	for _, v := range []string{`"unterminated`, `"interior"quote"`, "{{ env }}", "x?optional", "x#fragments", "x y", "x|y", `x'y`} {
		require.Empty(t, literalInclude(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!include", Value: v}), v)
	}
	for _, v := range []string{`"./team a.tfvars"`, `"./team ""x"".tfvars"`, "../ok.tfvars"} {
		require.NotEmpty(t, literalInclude(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!include", Value: v}), v)
	}
	r := Registration{Dir: "source", Stack: "dev"}
	require.True(t, strings.HasPrefix(r.SaveCommand(), "atmos stack add source"))
}

func TestSavedInputsWithUnmanagedOrInvalidManifest(t *testing.T) {
	root := t.TempDir()
	r := Registration{Project: root, Stack: "dev", Output: io.Discard, Dir: root}
	check := func(kind string) {
		t.Helper()
		v, err := r.SavedVarFiles(kind, "image", root)
		require.NoError(t, err)
		require.Empty(t, v)
	}
	check("packer")
	r.Config = &schema.AtmosConfiguration{}
	check("packer")
	r.Config.Stacks.IncludedPaths = []string{"*.yaml"}
	r.Config.StacksBaseAbsolutePath = filepath.Join(root, "stacks")
	check("packer")
	for _, manifest := range []string{"components: {}", "components: {packer: {image: {vars: {local: yes}}}}"} {
		guidanceFile(t, root, "stacks/dev.yaml", manifest)
		check("packer")
	}
	guidanceFile(t, root, "stacks/dev.yaml", "name: [invalid")
	_, err := r.SavedVarFiles("packer", "image", root)
	require.Error(t, err)
	_, err = varNode([]byte("components: [bad"), "components")
	require.Error(t, err)
	files, err := r.AddVarFiles([]byte("components: [bad"), VarFileTarget{Dest: "components.packer.image"}, true)
	require.Error(t, err)
	require.Nil(t, files)
	_, err = r.AddVarFiles(nil, VarFileTarget{Dest: "x", Dir: root}, true)
	require.NoError(t, err)
}

func TestPreviewRequiresUnchangedConfig(t *testing.T) {
	root := t.TempDir()
	r := Registration{Project: root, Dir: root, Stack: "dev", Adapters: Adapters{fixtureAdapter{kind: "x", match: Match{Path: root}}}, DryRun: true, Output: io.Discard, reviewed: make(map[string]reviewedFile)}
	require.NoError(t, r.adoptReviewed(t.Context()))
	r.DryRun = false
	guidanceFile(t, root, "atmos.yaml", "base_path: .")
	require.ErrorContains(t, r.adoptReviewed(t.Context()), "changed after preview")
	r.reviewed = nil
	require.NoError(t, r.adoptReviewed(t.Context()))
}
