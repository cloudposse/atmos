package dependencies

import (
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// toolIdentity maps tool names to their owner/repo identity and answers
// "is this tool installed" questions. Resolution is offline-first: configured
// aliases, built-in aliases, and the owner/repo form never touch the registry.
// The registry-backed resolver is created lazily, only for bare names that the
// offline rules cannot place.
type toolIdentity struct {
	aliases map[string]string
	resolve func(tool string) (owner, repo string, err error)
	find    func(owner, repo, version string, binaryName ...string) (string, error)
	list    func(owner, repo string) ([]string, error)
	locator *toolchain.Installer
}

func newToolIdentity(atmosConfig *schema.AtmosConfiguration, cfg *envConfig) *toolIdentity {
	ids := &toolIdentity{resolve: cfg.resolveFunc, find: cfg.findBinaryPath, list: cfg.listInstalled}
	if atmosConfig != nil {
		ids.aliases = atmosConfig.Toolchain.Aliases
	}
	return ids
}

func (t *toolIdentity) installer() *toolchain.Installer {
	if t.locator == nil {
		t.locator = toolchain.NewInstaller()
	}
	return t.locator
}

// splitOwnerRepo parses "owner/repo" exactly.
func splitOwnerRepo(name string) (owner, repo string, ok bool) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// offline resolves a tool name to "owner/repo" without registry access, using
// configured aliases, then built-in aliases, then the owner/repo form.
func (t *toolIdentity) offline(tool string) (string, bool) {
	name := tool
	if target, ok := t.aliases[name]; ok {
		name = target
	}
	if target, ok := toolchain.BuiltinAliases[name]; ok {
		name = target
	}
	owner, repo, ok := splitOwnerRepo(name)
	if !ok {
		return "", false
	}
	return owner + "/" + repo, true
}

// identity resolves a tool name to "owner/repo", trying the offline rules first
// and the registry-backed resolver second.
func (t *toolIdentity) identity(tool string) (string, error) {
	if id, ok := t.offline(tool); ok {
		return id, nil
	}
	if t.resolve == nil {
		t.resolve = t.installer().GetResolver().Resolve
	}
	owner, repo, err := t.resolve(tool)
	if err != nil {
		return "", err
	}
	return owner + "/" + repo, nil
}

// installedVersion reports the installed version of the tool that satisfies the
// requested version, and whether there is one. A concrete version must be on
// disk exactly. A constraint (for example `~>1.9.0`) resolves to the highest
// installed version that satisfies it. Nothing is ever downloaded.
func (t *toolIdentity) installedVersion(identity, version string) (string, bool) {
	owner, repo, ok := splitOwnerRepo(identity)
	if !ok {
		return "", false
	}
	if isConstraint(version) {
		version, ok = t.highestInstalledMatch(owner, repo, version)
		if !ok {
			return "", false
		}
	}
	if t.find == nil {
		t.find = t.installer().FindBinaryPath
	}
	if _, err := t.find(owner, repo, version); err != nil {
		return "", false
	}
	return version, true
}

// highestInstalledMatch returns the highest installed version of owner/repo that
// satisfies the constraint.
func (t *toolIdentity) highestInstalledMatch(owner, repo, constraint string) (string, bool) {
	parsed, err := semver.NewConstraint(constraint)
	if err != nil {
		return "", false
	}
	if t.list == nil {
		t.list = t.installer().ListInstalledVersions
	}
	installed, err := t.list(owner, repo)
	if err != nil {
		return "", false
	}
	match, err := highestMatch(installed, parsed)
	if err != nil {
		return "", false
	}
	return match, true
}

// groupSelected reports whether any manifest key in the group matches any of the
// selected executables.
func (t *toolIdentity) groupSelected(group *defaultGroup, selected []string) bool {
	for _, exe := range selected {
		for _, key := range group.keys {
			if t.keyMatchesExecutable(key, exe) {
				return true
			}
		}
	}
	return false
}

// keyMatchesExecutable decides whether manifest key k provides executable c.
// Rules, in order, none of which need the network:
//  1. k == c.
//  2. c maps through configured or built-in aliases to an owner/repo equal to k
//     or to k's own alias target (tofu -> opentofu/opentofu).
//  3. k resolves to owner/repo and repo == c (hashicorp/terraform provides terraform).
func (t *toolIdentity) keyMatchesExecutable(key, exe string) bool {
	if key == exe {
		return true
	}
	keyID, keyOK := t.offline(key)
	if exeID, exeOK := t.offline(exe); exeOK && (exeID == key || (keyOK && exeID == keyID)) {
		return true
	}
	if keyOK {
		_, repo, _ := splitOwnerRepo(keyID)
		return repo == exe
	}
	return false
}

// isPathLike reports whether an executable names a file location rather than a
// bare command.
func isPathLike(executable string) bool {
	return filepath.IsAbs(executable) || strings.ContainsAny(executable, `/\`)
}

// normalizeExecutable strips a Windows ".exe" suffix so "tofu.exe" matches "tofu".
func normalizeExecutable(executable string) string {
	if strings.EqualFold(filepath.Ext(executable), ".exe") {
		return strings.TrimSuffix(executable, filepath.Ext(executable))
	}
	return executable
}
