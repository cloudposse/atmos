package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// TestAgentSkillCommandExamples checks published examples against registered commands without
// invoking authentication, downloading tools, or executing the examples' external operations.
func TestAgentSkillCommandExamples(t *testing.T) {
	_ = NewTestKit(t)
	tests := []struct {
		file, example, command string
	}{
		{"atmos-auth/references/commands-reference.md", "atmos aws ecr login dev/ecr/primary", "atmos aws ecr login"},
		{"atmos-auth/references/commands-reference.md", "atmos aws ecr login --identity dev-admin", "atmos aws ecr login"},
		{"atmos-auth/references/commands-reference.md", "atmos aws ecr login --registry 123456789012.dkr.ecr.us-east-1.amazonaws.com", "atmos aws ecr login"},
		{"atmos-ansible/SKILL.md", "atmos describe component hello-world -s dev", "atmos describe component"},
		{"atmos-ansible/references/commands-reference.md", "atmos describe component webserver -s prod", "atmos describe component"},
		{"atmos-design-patterns/references/version-management.md", "atmos vendor pull --component vpc", "atmos vendor pull"},
		{"atmos-toolchain/references/commands-reference.md", "atmos toolchain aliases", "atmos toolchain aliases"},
		{"atmos-toolchain/references/commands-reference.md", "atmos toolchain aliases --format=json", "atmos toolchain aliases"},
		{"atmos-toolchain/references/commands-reference.md", "atmos toolchain aliases --format=yaml", "atmos toolchain aliases"},
		{"atmos-settings/SKILL.md", "atmos theme list", "atmos theme list"},
		{"atmos-settings/SKILL.md", "atmos theme list --recommended", "atmos theme list"},
		{"atmos-settings/SKILL.md", "atmos theme show <theme-name>", "atmos theme show"},
	}
	for _, tt := range tests {
		t.Run(tt.example, func(t *testing.T) {
			_ = NewTestKit(t)
			content := readAgentSkill(t, tt.file)
			require.True(t, hasSkillCommand(content, tt.example), "published example missing from %s", tt.file)
			args := strings.Fields(tt.example)[1:]
			command, remaining, err := RootCmd.Find(args)
			require.NoError(t, err)
			require.Equal(t, tt.command, command.CommandPath())
			// Unknown flags must fail even for commands whose runtime parser allows passthrough.
			for _, arg := range remaining {
				if strings.HasPrefix(arg, "--") {
					name, _, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
					require.NotNil(t, command.Flag(name), "unknown documented flag %s", arg)
				} else if strings.HasPrefix(arg, "-") {
					require.NotNil(t, command.Flags().ShorthandLookup(strings.TrimPrefix(arg, "-")), "unknown documented shorthand %s", arg)
				}
			}
		})
	}
}

// TestAgentSkillWorkflowExample decodes the complete published workflow through the real
// workflow loader, catching unsupported retry keys and preserving its stated ten-second delay.
func TestAgentSkillWorkflowExample(t *testing.T) {
	_ = NewTestKit(t)
	content := readAgentSkill(t, "atmos-workflows/references/workflow-syntax.md")
	_, example, found := strings.Cut(content, "# stacks/workflows/networking.yaml\n")
	require.True(t, found)
	example, _, found = strings.Cut(example, "```")
	require.True(t, found)
	root := t.TempDir()
	manifest, err := workflow.LoadManifest(&schema.AtmosConfiguration{BasePathAbsolute: root}, filepath.Join(root, "networking.yaml"), []byte(example))
	require.NoError(t, err)
	definition, found := manifest.Workflows["deploy-networking"]
	require.True(t, found)
	require.NotEmpty(t, definition.Steps)
	retry := definition.Steps[0].Retry
	require.NotNil(t, retry)
	require.NotNil(t, retry.InitialDelay)
	require.Equal(t, 10*time.Second, *retry.InitialDelay)
	require.Equal(t, schema.BackoffConstant, retry.BackoffStrategy)
}

// TestAgentSkillToolchainAliasExample verifies the documented alias configuration.
func TestAgentSkillToolchainAliasExample(t *testing.T) {
	_ = NewTestKit(t)
	content := readAgentSkill(t, "atmos-toolchain/references/commands-reference.md")
	_, section, found := strings.Cut(content, "## Tool aliases (configuration)")
	require.True(t, found)
	_, example, found := strings.Cut(section, "```yaml\n")
	require.True(t, found)
	example, _, found = strings.Cut(example, "```")
	require.True(t, found)
	var config schema.AtmosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(example), &config))
	require.Equal(t, "hashicorp/terraform", config.Toolchain.Aliases["tf"])
}

// TestAgentSkillHTTPRetryExample checks that nested HTTP checks use the shared workflow retry schema.
func TestAgentSkillHTTPRetryExample(t *testing.T) {
	content := readAgentSkill(t, "atmos-tests/references/patterns.md")
	_, example, found := strings.Cut(content, "```yaml\n")
	require.True(t, found)
	example, _, found = strings.Cut(example, "```")
	require.True(t, found)
	var document struct {
		Commands []struct {
			Steps []schema.WorkflowStep `yaml:"steps"`
		} `yaml:"commands"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(example), &document))
	require.Len(t, document.Commands, 1)
	require.Len(t, document.Commands[0].Steps, 1)
	checks := document.Commands[0].Steps[0].Steps
	require.Len(t, checks, 2)
	require.NotNil(t, checks[0].Retry)
	require.NotNil(t, checks[0].Retry.InitialDelay)
	require.Equal(t, 2*time.Second, *checks[0].Retry.InitialDelay)
}

// TestAgentSkillCustomCommandIntro validates each published introductory step against its registered handler.
func TestAgentSkillCustomCommandIntro(t *testing.T) {
	content := readAgentSkill(t, "atmos-custom-commands/SKILL.md")
	_, example, found := strings.Cut(content, "```yaml\n")
	require.True(t, found)
	example, _, found = strings.Cut(example, "```")
	require.True(t, found)
	var document schema.AtmosConfiguration
	require.NoError(t, yaml.Unmarshal([]byte(example), &document))
	require.Len(t, document.Commands, 1)
	require.NotEmpty(t, document.Commands[0].Steps)
	for _, task := range document.Commands[0].Steps {
		definition := task.ToWorkflowStep()
		handler, registered := step.Get(definition.Type)
		require.True(t, registered)
		require.NoError(t, handler.Validate(&definition))
	}
}

// readAgentSkill loads a bundled skill or reference relative to the skills directory.
func readAgentSkill(t *testing.T, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "agent-skills", "skills", filepath.FromSlash(relative)))
	require.NoError(t, err)
	return string(content)
}

// hasSkillCommand matches a complete example line, ignoring an optional trailing shell comment.
func hasSkillCommand(content, example string) bool {
	for _, line := range strings.Split(content, "\n") {
		command, _, _ := strings.Cut(line, " #")
		if strings.TrimSpace(command) == example {
			return true
		}
	}
	return false
}
