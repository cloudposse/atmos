package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCLIHelmAdoption(t *testing.T) {
	ensureAtmosRunner(t)
	helm := requireExecutablePath(t, "helm", "comparing native and adopted chart renders")
	project := t.TempDir()
	chart := "./charts/web"
	require.NoError(t, os.CopyFS(filepath.Join(project, chart), os.DirFS("../examples/helm/components/helm/demo")))
	writeValues := func(name, contents string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(project, name), []byte(contents), 0o600))
	}
	writeValues("base.yaml", "service: {port: 81}\nimage: {tag: '1.24'}\n")
	const serviceDocument = "service: {port: 82}\n"
	const imageDocument = "image: {tag: '1.27'}\n"
	writeValues("env.yaml", serviceDocument+"---\n"+imageDocument)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("KUBECONFIG", filepath.Join(project, "unused-kubeconfig"))
	t.Setenv("ATMOS_EXPERIMENTAL", "silence")

	run := func(cmd *exec.Cmd) []byte {
		t.Helper()
		cmd.Dir = project
		var diagnostics bytes.Buffer
		cmd.Stderr = &diagnostics
		output, err := cmd.Output()
		require.NoError(t, err, "%s\n%s", cmd.Args, diagnostics.String())
		return output
	}
	atmos := func(args ...string) []byte {
		t.Helper()
		return run(atmosRunner.CommandContext(t.Context(), args...))
	}
	// The component embeds Helm 4. On a Helm 3 test host, supply each
	// document as an individual native overlay because Helm 3 ignores later
	// documents in a single file. Atmos always receives the original file.
	helm3 := strings.HasPrefix(string(run(exec.CommandContext(t.Context(), helm, "version", "--short"))), "v3.")
	if helm3 {
		writeValues("service.yaml", serviceDocument)
		writeValues("image.yaml", imageDocument)
	}
	manifest := filepath.Join(project, "stacks", "dev.yaml")
	for _, release := range []struct {
		name, namespace, first, last, image string
		port                                int
	}{
		{"web", "preview", "base.yaml", "env.yaml", "nginx:1.27", 82},
		{"web-canary", "canary", "env.yaml", "base.yaml", "nginx:1.24", 81},
	} {
		add := []string{"stack", "add", chart, "-s", "dev", "--release-name", release.name, "--namespace", release.namespace, "-f", release.first, "-f", release.last}
		if release.name == "web" {
			atmos(append(add, "--dry-run")...)
			require.NoFileExists(t, manifest)
			require.NoFileExists(t, filepath.Join(project, "atmos.yaml"))
		}
		atmos(add...)
		before, err := os.ReadFile(manifest)
		require.NoError(t, err)
		atmos(add...)
		after, err := os.ReadFile(manifest)
		require.NoError(t, err)
		require.Equal(t, before, after)

		nativeArgs := []string{"template", release.name, chart, "--namespace", release.namespace}
		for _, file := range []string{release.first, release.last} {
			if helm3 && file == "env.yaml" {
				nativeArgs = append(nativeArgs, "-f", "service.yaml", "-f", "image.yaml")
			} else {
				nativeArgs = append(nativeArgs, "-f", file)
			}
		}
		native := run(exec.CommandContext(t.Context(), helm, nativeArgs...))
		actual := atmos("helm", "template", release.name, "-s", "dev")
		expected := helmAdoptionResources(t, native)
		require.Len(t, expected, 2)
		require.Contains(t, expected, "Service/"+release.namespace+"/"+release.name)
		require.Contains(t, expected, "Deployment/"+release.namespace+"/"+release.name)
		require.Contains(t, string(native), release.image)
		require.Contains(t, string(native), fmt.Sprintf("port: %d", release.port))
		require.Equal(t, expected, helmAdoptionResources(t, actual))
	}
	// The first instance must still render its own values after saving the sibling.
	require.Contains(t, string(atmos("helm", "template", "web", "-s", "dev")), "nginx:1.27")
}

// Compare complete resources while ignoring document order and Helm source comments.
func helmAdoptionResources(t *testing.T, manifest []byte) map[string]map[string]any {
	t.Helper()
	resources := make(map[string]map[string]any)
	decoder := yaml.NewDecoder(bytes.NewReader(manifest))
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return resources
		}
		require.NoError(t, err)
		if len(document) == 0 {
			continue
		}
		metadata, ok := document["metadata"].(map[string]any)
		require.True(t, ok)
		key := fmt.Sprintf("%v/%v/%v", document["kind"], metadata["namespace"], metadata["name"])
		require.NotContains(t, resources, key)
		resources[key] = document
	}
}
