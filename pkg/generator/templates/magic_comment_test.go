package templates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasTemplateMagicComment(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "shell style comment",
			content:  "# atmos:template\necho 'hello'",
			expected: true,
		},
		{
			name:     "go style comment",
			content:  "// atmos:template\npackage main",
			expected: true,
		},
		{
			name:     "c style block comment",
			content:  "/* atmos:template */\nint main() {}",
			expected: true,
		},
		{
			name:     "html/xml comment",
			content:  "<!-- atmos:template -->\n<html></html>",
			expected: true,
		},
		{
			name:     "case insensitive",
			content:  "# ATMOS:TEMPLATE\ntest",
			expected: true,
		},
		{
			name:     "case insensitive mixed",
			content:  "// Atmos:Template\ntest",
			expected: true,
		},
		{
			name:     "with extra whitespace",
			content:  "#   atmos:template   \ntest",
			expected: true,
		},
		{
			name:     "beyond max lines",
			content:  "\n\n\n\n\n\n\n\n\n\n\n# atmos:template\ntest",
			expected: false,
		},
		{
			name:     "no magic comment",
			content:  "# regular comment\ntest",
			expected: false,
		},
		{
			name:     "similar but not magic comment",
			content:  "# atmos-template\ntest",
			expected: false,
		},
		{
			name:     "magic comment in middle of line",
			content:  "some text // atmos:template\ntest",
			expected: true,
		},
		{
			name:     "python docstring with magic comment",
			content:  "#!/usr/bin/env python\n# atmos:template\n\"\"\"Module doc\"\"\"",
			expected: true,
		},
		{
			name:     "yaml with magic comment",
			content:  "---\n# atmos:template\nkey: value",
			expected: true,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "only magic comment",
			content:  "# atmos:template",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasTemplateMagicComment(tt.content, 10)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStripTemplateMagicComment(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "shell style comment alone on line",
			content:  "# atmos:template\necho 'hello'\ntest",
			expected: "echo 'hello'\ntest",
		},
		{
			name:     "go style comment alone on line",
			content:  "// atmos:template\npackage main\nfunc main() {}",
			expected: "package main\nfunc main() {}",
		},
		{
			name:     "c style block comment alone on line",
			content:  "/* atmos:template */\nint main() {}\nreturn 0;",
			expected: "int main() {}\nreturn 0;",
		},
		{
			name:     "html comment alone on line",
			content:  "<!-- atmos:template -->\n<html>\n</html>",
			expected: "<html>\n</html>",
		},
		{
			name:     "with leading whitespace",
			content:  "   # atmos:template\ntest\nmore",
			expected: "test\nmore",
		},
		{
			name:     "multiple lines preserved",
			content:  "// atmos:template\nline1\nline2\nline3",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "no magic comment",
			content:  "# regular comment\ntest\nmore",
			expected: "# regular comment\ntest\nmore",
		},
		{
			name:     "magic comment not alone on line (stripped)",
			content:  "some text // atmos:template\ntest",
			expected: "some text\ntest",
		},
		{
			name:     "empty content",
			content:  "",
			expected: "",
		},
		{
			name:     "only magic comment",
			content:  "# atmos:template",
			expected: "",
		},
		{
			name:     "shebang and magic comment",
			content:  "#!/bin/bash\n# atmos:template\necho 'test'",
			expected: "#!/bin/bash\necho 'test'",
		},
		{
			name:     "yaml with magic comment",
			content:  "---\n# atmos:template\nkey: value\nkey2: value2",
			expected: "---\nkey: value\nkey2: value2",
		},
		{
			name:     "yaml with inline magic comment",
			content:  "---\nkey: value # atmos:template\nkey2: value2",
			expected: "---\nkey: value\nkey2: value2",
		},
		{
			name:     "multiple inline magic comments",
			content:  "line1 // atmos:template\nline2\nline3 # atmos:template",
			expected: "line1\nline2\nline3",
		},
		{
			name:     "inline magic comment with only comment marker remaining",
			content:  "// atmos:template\ntest",
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTemplateMagicComment(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTemplateMagicCommentPattern(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "shell comment",
			line:     "# atmos:template",
			expected: true,
		},
		{
			name:     "go comment",
			line:     "// atmos:template",
			expected: true,
		},
		{
			name:     "c block comment",
			line:     "/* atmos:template */",
			expected: true,
		},
		{
			name:     "html comment",
			line:     "<!-- atmos:template -->",
			expected: true,
		},
		{
			name:     "case insensitive uppercase",
			line:     "# ATMOS:TEMPLATE",
			expected: true,
		},
		{
			name:     "case insensitive mixed",
			line:     "// Atmos:Template",
			expected: true,
		},
		{
			name:     "with spaces",
			line:     "#   atmos:template   ",
			expected: true,
		},
		{
			name:     "no comment marker",
			line:     "atmos:template",
			expected: true,
		},
		{
			name:     "wrong separator",
			line:     "# atmos-template",
			expected: false,
		},
		{
			name:     "wrong prefix",
			line:     "# atm:template",
			expected: false,
		},
		{
			name:     "partial match",
			line:     "# atmostemplate",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := templateMagicCommentPattern.MatchString(tt.line)
			assert.Equal(t, tt.expected, result, "Pattern matching failed for: %s", tt.line)
		})
	}
}

func TestMagicCommentIntegration(t *testing.T) {
	t.Run("File with magic comment should be detected as template", func(t *testing.T) {
		content := `# atmos:template
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.name}}
data:
  key: {{.value}}
`
		isTemplate := hasTemplateMagicComment(content, 10)
		assert.True(t, isTemplate)

		stripped := stripTemplateMagicComment(content)
		assert.NotContains(t, stripped, "atmos:template")
		assert.Contains(t, stripped, "apiVersion: v1")
		assert.Contains(t, stripped, "{{.name}}")
	})

	t.Run("File without magic comment should not be detected", func(t *testing.T) {
		content := `{
  "config": {
    "template": "{{value}}"
  }
}`
		isTemplate := hasTemplateMagicComment(content, 10)
		assert.False(t, isTemplate)
	})

	t.Run("Multiple comment styles", func(t *testing.T) {
		testCases := []string{
			"#!/bin/bash\n# atmos:template\necho 'test'",
			"// atmos:template\npackage main",
			"/* atmos:template */\nint main() {}",
			"<!-- atmos:template -->\n<html></html>",
		}

		for _, content := range testCases {
			isTemplate := hasTemplateMagicComment(content, 10)
			assert.True(t, isTemplate, "Should detect template in: %s", content)

			stripped := stripTemplateMagicComment(content)
			assert.NotContains(t, stripped, "atmos:template", "Should strip magic comment from: %s", content)
		}
	})
}
