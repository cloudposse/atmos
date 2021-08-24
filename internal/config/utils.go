package config

import (
	g "atmos/internal/globals"
	"fmt"
	"github.com/bmatcuk/doublestar"
	"path/filepath"
	"strings"
)

// findAllStackConfigsInPaths finds all stack config files in the paths specified by globs
func findAllStackConfigsInPaths(stack string, includeStackPaths []string, excludeStackPaths []string) ([]string, bool, string, error) {
	var res []string

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
			return nil, false, "", err
		}

		// Exclude files that match any of the excludePaths
		if matches != nil && len(matches) > 0 {
			for _, matchedFile := range matches {
				stackMatch := strings.HasSuffix(matchedFile, stack+g.DefaultStackConfigFileExtension)
				if stackMatch == true {
					return []string{matchedFile}, true, matchedFile, nil
				}

				include := true

				for _, excludePath := range excludeStackPaths {
					match, err := doublestar.PathMatch(excludePath, matchedFile)
					if err != nil {
						fmt.Println(err)
						include = false
						continue
					}
					if match {
						include = false
						continue
					}
				}

				if include == true {
					res = append(res, matchedFile)
				}
			}
		}
	}

	return res, false, "", nil
}
