package atmos

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// requireStringParam extracts a required string parameter, erroring with
// errUtils.ErrAIToolParameterRequired when the parameter is missing, not a
// string, or empty.
func requireStringParam(params map[string]interface{}, name string) (string, error) {
	value, ok := params[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%w: %s", errUtils.ErrAIToolParameterRequired, name)
	}
	return value, nil
}

// optionalStringParam extracts an optional string parameter, returning "" when
// the parameter is absent or not a string.
func optionalStringParam(params map[string]interface{}, name string) string {
	if value, ok := params[name].(string); ok {
		return value
	}
	return ""
}

// resolveConfigTargetFile resolves the atmos.yaml file an atmos_config_* tool
// should operate on. An optional "file" parameter mirrors the CLI's --config
// flag; when absent, resolution falls back to cfg.ResolveEditableConfigFile's
// current-directory / git-root discovery. The returned path is guaranteed to
// exist -- GetFile/SetFileWithType/DeleteFile/FormatFile all require the
// target file to already exist and do not auto-create it.
func resolveConfigTargetFile(atmosConfig *schema.AtmosConfiguration, params map[string]interface{}) (string, error) {
	override := optionalStringParam(params, "file")

	file, err := cfg.ResolveEditableConfigFile(atmosConfig, override)
	if err != nil {
		if errors.Is(err, cfg.ErrNoEditableConfig) {
			return "", fmt.Errorf("%w: %w", errUtils.ErrAIConfigFileNotFound, err)
		}
		return "", err
	}
	return file, nil
}

// filterConfigPathEntries filters entries to those whose Path matches a
// glob-style pattern, where '*' matches any sequence of characters and '?'
// matches a single character. An empty pattern matches everything. This
// mirrors the filtering pkg/list.RenderPathRowsWithPattern applies to
// PathRow.Path for `atmos config list`.
func filterConfigPathEntries(entries []atmosyaml.PathEntry, pattern string) []atmosyaml.PathEntry {
	if pattern == "" {
		return entries
	}

	re := configPathPatternRegexp(pattern)
	filtered := make([]atmosyaml.PathEntry, 0, len(entries))
	for _, entry := range entries {
		if re.MatchString(entry.Path) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// configPathPatternRegexp compiles a glob-style pattern ('*' and '?') into an
// anchored regexp matching the full path.
func configPathPatternRegexp(pattern string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	return regexp.MustCompile("^" + quoted + "$")
}
