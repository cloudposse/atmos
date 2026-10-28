package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LooksLikePath reports whether a release source is a filesystem path. It never touches the
// network: a leading dot, slash, tilde or backslash, a Windows drive letter, or an existing
// directory holding plugin.yaml all mean the user meant a local plugin.
func LooksLikePath(source string) bool {
	defer perf.Track(nil, "plugininstall.LooksLikePath")()

	if source == "" || strings.HasPrefix(source, "oci://") {
		return false
	}
	switch source[0] {
	case '.', '/', '~', '\\':
		return true
	}
	if len(source) >= 2 && source[1] == ':' && unicode.IsLetter(rune(source[0])) {
		return true
	}
	if strings.Contains(source, "\\") {
		return true
	}
	info, err := os.Stat(filepath.Join(source, "plugin.yaml"))
	return err == nil && info.Mode().IsRegular()
}

// CheckReleaseSource rejects path-like sources with guidance, before any registry is queried.
func CheckReleaseSource(source string) error {
	defer perf.Track(nil, "plugininstall.CheckReleaseSource")()

	if !LooksLikePath(source) {
		return nil
	}
	builder := errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrPluginSourceIsPath, source)).
		WithHintf("To run a plugin you built locally, declare `path: %s` for the package under `plugins.packages.<alias>` in a profile (or in `atmos.yaml`); a path package is never fetched or locked.", source)
	if isOCILayout(source) {
		builder = builder.WithHintf("`%s` is an OCI layout. To install it offline, pass `--from-layout %s` together with the release source, for example `atmos plugin install oci://registry.example.com/acme/plugin --from-layout %s`.", source, source, source)
	}
	return builder.Err()
}

func isOCILayout(directory string) bool {
	_, err := os.Stat(filepath.Join(directory, "oci-layout"))
	return err == nil
}

// ParseSelector splits `<registry>/<package>`. Every segment must be non-empty, so a malformed
// selector is rejected locally instead of being searched across every configured registry.
func ParseSelector(source string) (registry, name string, err error) {
	defer perf.Track(nil, "plugininstall.ParseSelector")()

	registry, name, found := strings.Cut(source, "/")
	if found && registry != "" && name != "" {
		valid := true
		for _, segment := range strings.Split(name, "/") {
			valid = valid && segment != ""
		}
		if valid {
			return registry, name, nil
		}
	}
	return "", "", errUtils.Build(fmt.Errorf("%w: %q is not a valid plugin selector", errUtils.ErrPluginInvalidConfig, source)).
		WithHint("Use `<registry>/<package>` with a registry alias from `atmos plugin registry list`, or a full `oci://<host>/<repository>` source.").
		Err()
}
