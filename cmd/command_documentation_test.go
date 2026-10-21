package cmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCommandDocumentationHelpTopics(t *testing.T) {
	_ = NewTestKit(t)
	root, leaf, _ := discoveryTestRoot(t)
	for _, command := range []*cobra.Command{root, leaf} {
		for _, topic := range append([]helpTopic{helpTopicDefault}, supportedHelpTopics...) {
			t.Run(command.Name()+"/"+string(topic), func(t *testing.T) {
				var out bytes.Buffer
				renderer := lipgloss.NewRenderer(&out)
				styles := createHelpStyles(renderer)
				ctx := &helpRenderContext{writer: &out, renderer: renderer, styles: &styles, atmosConfig: &schema.AtmosConfiguration{}}
				printHelpForTopic(ctx, command, helpTopicRequest{topic: topic, valid: true})
				assert.Contains(t, out.String(), "Documentation: "+internal.ResolveCommandMetadata(command).DocumentationURL)
				assert.Equal(t, 1, strings.Count(out.String(), "Documentation:"))
				if topic == helpTopicAll {
					assert.Contains(t, out.String(), "--docs")
					assert.Contains(t, out.String(), "--example")
				}
			})
		}
	}
}

// Match Docusaurus's local doc IDs and explicit slugs, rather than guessing a
// URL from the command name. This also catches links to removed example folders.
func TestCommandDocumentationCatalog(t *testing.T) {
	routes := map[string]string{"https://atmos.tools": "atmos"}
	expected := make(map[string]string)
	base := "../website/docs"
	require.NoError(t, filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (filepath.Ext(path) != ".md" && filepath.Ext(path) != ".mdx") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts := strings.SplitN(string(data), "---", 3)
		if len(parts) != 3 || parts[0] != "" {
			return nil
		}
		var metadata struct {
			Title string `yaml:"title"`
			ID    string `yaml:"id"`
			Slug  string `yaml:"slug"`
		}
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		rel = strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel))
		if metadata.ID != "" {
			rel = filepath.ToSlash(filepath.Join(filepath.Dir(rel), metadata.ID))
		}
		if metadata.Slug != "" {
			if strings.HasPrefix(metadata.Slug, "/") {
				rel = strings.TrimPrefix(metadata.Slug, "/")
			} else {
				rel = filepath.ToSlash(filepath.Join(filepath.Dir(rel), metadata.Slug))
			}
		}
		url := "https://atmos.tools/" + rel
		routes[url] = metadata.Title
		if strings.HasPrefix(metadata.Title, "atmos ") {
			expected[metadata.Title] = url
		}
		return nil
	}))
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		metadata := internal.GetCommandMetadata(command)
		if metadata.DocumentationURL != "" {
			_, exists := routes[metadata.DocumentationURL]
			assert.True(t, exists, "%s links to missing documentation %s", command.CommandPath(), metadata.DocumentationURL)
			if url := expected[command.CommandPath()]; url != "" {
				assert.Equal(t, url, metadata.DocumentationURL, "%s links to another command's page", command.CommandPath())
			}
		} else if url := expected[command.CommandPath()]; url != "" {
			t.Errorf("Dedicated documentation not registered: %s -> %s", command.CommandPath(), url)
		}
		for _, example := range metadata.Examples {
			assert.DirExists(t, filepath.Join("..", example.Source), command.CommandPath())
			assert.NotEmpty(t, example.Label)
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(RootCmd)
}
