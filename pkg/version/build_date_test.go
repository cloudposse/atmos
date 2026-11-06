package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// Exercise the actual release linker flags against a tiny binary importing the
// production version package, including stripped symbols and trimmed build paths.
func TestReleaseBuildDateSurvivesLinking(t *testing.T) {
	t.Parallel()
	const timestamp = "2026-06-27T12:00:00Z"
	for _, filename := range []string{".goreleaser.yml", ".goreleaser.draft.yml"} {
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("..", "..", filename))
			require.NoError(t, err)
			var config struct {
				Builds []struct {
					LDFlags []string `yaml:"ldflags"`
				} `yaml:"builds"`
			}
			require.NoError(t, yaml.Unmarshal(data, &config))
			require.NotEmpty(t, config.Builds)
			require.NotEmpty(t, config.Builds[0].LDFlags)
			flags := strings.NewReplacer(
				"{{.Tag}}", "v1.2.3", "{{.Version}}", "1.2.3",
				"{{.Commit}}", strings.Repeat("a", 40), "{{.CommitDate}}", timestamp,
			).Replace(strings.Join(config.Builds[0].LDFlags, " "))
			directory := t.TempDir()
			source := filepath.Join(directory, "main.go")
			binary := filepath.Join(directory, "version-fixture.exe")
			require.NoError(t, os.WriteFile(source, []byte(`package main
import (
 "fmt"
 "github.com/cloudposse/atmos/pkg/version"
)
func main() { fmt.Printf("%s\n%s", version.Version, version.BuildDate) }
`), 0o600))
			command := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags", flags, "-o", binary, source)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			output, err = exec.CommandContext(t.Context(), binary).CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, "1.2.3\n"+timestamp, string(output))
		})
	}
}
