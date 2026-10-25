package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestCommandMarkdownURL(t *testing.T) {
	for source, expected := range map[string]string{
		"https://atmos.tools":                              "https://atmos.tools/index.md",
		"https://atmos.tools/":                             "https://atmos.tools/index.md",
		"https://atmos.tools/cli/commands/terraform/usage": "https://atmos.tools/cli/commands/terraform/usage.md",
		"https://atmos.tools/cli/commands/init/":           "https://atmos.tools/cli/commands/init.md",
		"https://atmos.tools/intro.md":                     "https://atmos.tools/intro.md",
		"https://atmos.tools/intro?lang=en#start":          "https://atmos.tools/intro.md?lang=en#start",
	} {
		t.Run(source, func(t *testing.T) {
			actual, err := commandMarkdownURL(source)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestDiscoveryMarkdown(t *testing.T) {
	_ = NewTestKit(t)
	mockDiscoveryDependencies(t)
	t.Setenv("NO_COLOR", "")
	previous := ui.GetColorProfile()
	previousArgs := os.Args
	t.Cleanup(func() {
		os.Args = previousArgs
		ui.SetColorProfile(previous)
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/plan.md", r.URL.Path)
		assert.Contains(t, r.Header.Get("Accept"), "text/markdown")
		w.Header().Set("Content-Type", "text/markdown")
		fmt.Fprintln(w, "# Planning infrastructure\n\nUse **atmos terraform plan** to preview changes.")
	}))
	t.Cleanup(server.Close)
	for _, forceColor := range []bool{false, true} {
		t.Run(fmt.Sprint(forceColor), func(t *testing.T) {
			root, leaf, output := discoveryTestRoot(t)
			internal.RegisterCommandMetadata(leaf, internal.WithDocumentationURL(server.URL+"/plan"))
			colorFlag := "--no-color"
			if forceColor {
				colorFlag = "--force-color"
			}
			args := []string{"terraform", "plan", "--docs=md", colorFlag}
			// Match cmd.init() in a fresh CLI: terminal capabilities are captured
			// from the invocation before early discovery, not inherited from
			// a previous test's formatter (which caches its color profile).
			os.Args = append([]string{"atmos"}, args...)
			ensureIOInitialized(t)
			handled, err := dispatchCommandDiscovery(root, args)
			require.NoError(t, err)
			assert.True(t, handled)
			plain := ansi.Strip(output.String())
			assert.Contains(t, plain, "Documentation: "+server.URL+"/plan.md")
			assert.Contains(t, plain, "Planning infrastructure")
			assert.Contains(t, plain, "atmos terraform plan")
			assert.Equal(t, forceColor, strings.Contains(output.String(), "\x1b["))
		})
	}
}

func TestDiscoveryMarkdownErrors(t *testing.T) {
	mockDiscoveryDependencies(t)
	for _, scenario := range []string{"missing", "html", "too large", "truncated", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch scenario {
				case "missing":
					http.NotFound(w, r)
				case "html":
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprint(w, "<html>Not documentation</html>")
				case "too large":
					fmt.Fprint(w, strings.Repeat("x", maxCommandDocumentationBytes+1))
				case "truncated":
					w.Header().Set("Content-Length", "100")
					fmt.Fprint(w, "short")
				}
			}))
			t.Cleanup(server.Close)
			root, leaf, output := discoveryTestRoot(t)
			internal.RegisterCommandMetadata(leaf, internal.WithDocumentationURL(server.URL+"/plan"))
			if scenario == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				root.SetContext(ctx)
			}
			handled, err := dispatchCommandDiscovery(root, []string{"terraform", "plan", "--docs=markdown"})
			assert.True(t, handled)
			require.Error(t, err)
			assert.Contains(t, output.String(), server.URL+"/plan.md")
			assert.NotContains(t, output.String(), "Not documentation")
		})
	}
}

func TestDiscoveryMarkdownFlagSemantics(t *testing.T) {
	for _, tc := range []struct {
		args                             string
		enabled, markdown, help, invalid bool
	}{
		{"--docs=md terraform plan", true, true, false, false},
		{"terraform plan --docs=md", true, true, false, false},
		{"--docs=markdown terraform plan", true, true, false, false},
		{"terraform plan --docs=md --docs=false", false, false, false, false},
		{"terraform plan --docs=false --docs=md", true, true, false, false},
		{"terraform plan --help --docs=md", true, true, true, false},
		{"terraform plan -- --docs=md", false, false, false, false},
		{"terraform plan --docs=markdown --docs=false", false, false, false, false},
		{"terraform plan --docs=markdown --docs", true, false, false, false},
		{"terraform plan --help --docs=markdown", true, true, true, false},
		{"terraform plan --docs=pdf", true, false, false, true},
		{"terraform plan -- --docs=markdown", false, false, false, false},
	} {
		t.Run(tc.args, func(t *testing.T) {
			root, _, _ := discoveryTestRoot(t)
			request, err := parseDiscoveryRequest(root, strings.Fields(tc.args))
			assert.Equal(t, tc.invalid, err != nil)
			assert.Equal(t, tc.enabled, request.docs)
			assert.Equal(t, tc.markdown, request.docsMarkdown)
			assert.Equal(t, tc.help, request.help)
		})
	}
}

func TestCustomDiscoveryFlagRegistration(t *testing.T) {
	for _, tc := range []struct{ name, kind, value string }{
		{"docs", "bool", "true"}, {"docs", "string", "markdown"}, {"example", "bool", "true"},
	} {
		t.Run(tc.name+"/"+tc.kind, func(t *testing.T) {
			mockDiscoveryDependencies(t)
			root, _, _ := discoveryTestRoot(t)
			config := schema.Command{Name: "custom", Flags: []schema.CommandFlag{{Name: tc.name, Type: tc.kind}}}
			require.NoError(t, validateCustomCommandFlags(&config, root))
			custom := &cobra.Command{Use: "custom"}
			root.AddCommand(custom)
			require.NoError(t, registerCustomCommandFlags(&config, custom, root))
			assert.NotSame(t, root.PersistentFlags().Lookup(tc.name), custom.PersistentFlags().Lookup(tc.name))
			root.PersistentPreRunE = nil
			ran := false
			custom.Run = func(cmd *cobra.Command, args []string) {
				ran = true
				assert.Equal(t, tc.value, cmd.Flags().Lookup(tc.name).Value.String())
			}
			args := []string{"--" + tc.name + "=" + tc.value, "custom"}
			handled, err := dispatchCommandDiscovery(root, args)
			require.NoError(t, err)
			require.False(t, handled)
			root.SetArgs(args)
			require.NoError(t, root.Execute())
			assert.True(t, ran)
		})
	}
}

func TestDiscoveryMarkdownResolvesLinksAfterRedirect(t *testing.T) {
	_ = NewTestKit(t)
	mockDiscoveryDependencies(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/original.md" {
			http.Redirect(w, r, "/moved/page.md", http.StatusFound)
			return
		}
		fmt.Fprint(w, "[Root](/intro.md)\n\n[Sibling](next.md)\n\n[Section](#examples)")
	}))
	t.Cleanup(server.Close)
	root, leaf, output := discoveryTestRoot(t)
	internal.RegisterCommandMetadata(leaf, internal.WithDocumentationURL(server.URL+"/original"))
	handled, err := dispatchCommandDiscovery(root, []string{"terraform", "plan", "--docs=md", "--no-color"})
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Contains(t, output.String(), server.URL+"/intro.md")
	assert.Contains(t, output.String(), server.URL+"/moved/next.md")
	assert.Contains(t, output.String(), server.URL+"/moved/page.md#examples")
}
