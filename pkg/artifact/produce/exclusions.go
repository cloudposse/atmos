package produce

import (
	"path"
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
	"github.com/cloudposse/atmos/pkg/perf"
)

// dirRuleSuffix marks a rule that excludes a whole directory subtree, for example ".terraform/**".
const dirRuleSuffix = "/**"

// terraformExclusions are the visible default exclusions of the component/terraform producer:
// Terraform working directories, state files, and saved plans. `.terraform.lock.hcl` is a different
// name and stays in the payload, because the dependency lock file belongs to the component.
var terraformExclusions = []string{
	".terraform" + dirRuleSuffix,
	"*.tfstate",
	"*.tfstate.*",
	"*.tfplan",
	"*.tfplan.*",
}

// TerraformExclusions returns the default exclusion rules of the component/terraform producer, in the
// form dry-run output reports them. Each rule matches at any depth below the source root: a rule
// ending in "/**" excludes every directory with that name and its contents, and any other rule is a
// glob on the entry name. The generic files producer and backups do not apply them.
func TerraformExclusions() []string {
	defer perf.Track(nil, "produce.TerraformExclusions")()

	return append([]string(nil), terraformExclusions...)
}

// excluderFor builds the snapshot excluder for rules. The rule text returned for a match is the
// rule exactly as listed, so reports name the rule a user can read in the documentation.
func excluderFor(rules []string) snapshot.Excluder {
	return func(entryPath string, isDir bool) (string, bool) {
		name := path.Base(entryPath)
		for _, rule := range rules {
			if matchRule(rule, name, isDir) {
				return rule, true
			}
		}
		return "", false
	}
}

// matchRule reports whether rule excludes the entry named name. Directory rules (a "/**" suffix) match
// directories by name. Name globs match files, links, and directories alike, so the workspace state
// directory `terraform.tfstate.d` is excluded with the state it holds.
func matchRule(rule, name string, isDir bool) bool {
	if dir, isDirRule := strings.CutSuffix(rule, dirRuleSuffix); isDirRule {
		return isDir && name == dir
	}
	matched, err := path.Match(rule, name)
	return err == nil && matched
}
