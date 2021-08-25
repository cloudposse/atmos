package config

import (
	g "atmos/internal/globals"
	u "atmos/internal/utils"
	"github.com/bmatcuk/doublestar"
	"github.com/fatih/color"
	"path/filepath"
	"strings"
)

// findAllStackConfigsInPaths finds all stack config files in the paths specified by globs
func findAllStackConfigsInPaths(
	stack string,
	includeStackPaths []string,
	excludeStackPaths []string,
) ([]string, []string, bool, error) {

	var absolutePaths []string
	var relativePaths []string

	for _, p := range includeStackPaths {
		pathWithExt := p

		ext := filepath.Ext(p)
		if ext == "" {
			ext = g.DefaultStackConfigFileExtension
			pathWithExt = p + ext
		}

		// Find all matches in the glob
		matches, err := doublestar.Glob(pathWithExt)
		if err != nil {
			return nil, nil, false, err
		}

		// Exclude files that match any of the excludePaths
		if matches != nil && len(matches) > 0 {
			for _, matchedFileAbsolutePath := range matches {
				matchedFileRelativePath := u.TrimBasePathFromPath(ProcessedConfig.StacksBaseAbsolutePath+"/", matchedFileAbsolutePath)
				stackMatch := strings.HasSuffix(matchedFileAbsolutePath, stack+g.DefaultStackConfigFileExtension)
				if stackMatch == true {
					return []string{matchedFileAbsolutePath}, []string{matchedFileRelativePath}, true, nil
				}

				include := true

				for _, excludePath := range excludeStackPaths {
					match, err := doublestar.PathMatch(excludePath, matchedFileAbsolutePath)
					if err != nil {
						color.Red("%s", err)
						include = false
						continue
					}
					if match {
						include = false
						continue
					}
				}

				if include == true {
					absolutePaths = append(absolutePaths, matchedFileAbsolutePath)
					relativePaths = append(relativePaths, matchedFileRelativePath)
				}
			}
		}
	}

	return absolutePaths, relativePaths, false, nil
}
