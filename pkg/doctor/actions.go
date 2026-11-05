package doctor

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/validation"
)

// actionlintConfigNames are the project-level actionlint configuration files, in precedence order.
var actionlintConfigNames = []string{"actionlint.yaml", "actionlint.yml"}

const (
	unknownLabelSeparator = ". available labels are "
	unknownLabelRemedy    = "If it is a custom self-hosted runner label, list it under self-hosted-runner.labels in .github/actionlint.yaml"
)

// actionlintConfig returns the repository's actionlint configuration, or "" when there is none.
// The linter only discovers its project configuration inside a Git repository, so doctor passes
// the file explicitly to honor it in source archives and nested projects too.
func actionlintConfig(root string) string {
	for _, name := range actionlintConfigNames {
		path := filepath.Join(root, ".github", name)
		if exists, err := regularFile(path); err == nil && exists {
			return path
		}
	}
	return ""
}

// appendActionDiagnostics converts workflow diagnostics into findings, shortening unknown
// runner-label messages and pointing at the configuration that resolves them.
func appendActionDiagnostics(r *Report, report validation.Report) {
	start := len(r.Findings)
	appendDiagnostics(r, report)
	for i := start; i < len(r.Findings); i++ {
		f := &r.Findings[i]
		if head, _, ok := strings.Cut(f.Message, unknownLabelSeparator); ok {
			f.Message = head
			f.Remediation = unknownLabelRemedy
		}
	}
}
