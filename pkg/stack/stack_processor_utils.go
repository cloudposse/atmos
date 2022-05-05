package stack

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	g "github.com/cloudposse/atmos/pkg/globals"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	getFileContentSyncMap = sync.Map{}
	getGlobMatchesSyncMap = sync.Map{}
)

// FindComponentStacks finds all infrastructure stack config files where the component or the base component is defined
func FindComponentStacks(
	componentType string,
	component string,
	baseComponent string,
	componentStackMap map[string]map[string][]string) ([]string, error) {

	var stacks []string

	if componentStackConfig, componentStackConfigExists := componentStackMap[componentType]; componentStackConfigExists {
		if componentStacks, componentStacksExist := componentStackConfig[component]; componentStacksExist {
			stacks = append(stacks, componentStacks...)
		}

		if baseComponent != "" {
			if baseComponentStacks, baseComponentStacksExist := componentStackConfig[baseComponent]; baseComponentStacksExist {
				stacks = append(stacks, baseComponentStacks...)
			}
		}
	}

	unique := u.UniqueStrings(stacks)
	sort.Strings(unique)
	return unique, nil
}

// FindComponentDependencies finds all imports where the component or the base component is defined
// Component depends on the imported config file if any of the following conditions is true:
// 1. The imported file has any of the global `backend`, `backend_type`, `env`, `remote_state_backend`, `remote_state_backend_type`,
//    `settings` or `vars` sections which are not empty
// 2. The imported file has the component type section, which has any of the `backend`, `backend_type`, `env`, `remote_state_backend`,
//    `remote_state_backend_type`, `settings` or `vars` sections which are not empty
// 3. The imported config file has the "components" section, which has the component type section, which has the component section
// 4. The imported config file has the "components" section, which has the component type section, which has the base component section
func FindComponentDependencies(
	stack string,
	componentType string,
	component string,
	baseComponent string,
	importsConfig map[string]map[interface{}]interface{}) ([]string, error) {

	var deps []string

	for imp, importConfig := range importsConfig {
		if sectionContainsAnyNotEmptySections(importConfig, []string{
			"backend",
			"backend_type",
			"env",
			"remote_state_backend",
			"remote_state_backend_type",
			"settings",
			"vars",
		}) {
			deps = append(deps, imp)
			continue
		}

		if sectionContainsAnyNotEmptySections(importConfig, []string{componentType}) {
			if sectionContainsAnyNotEmptySections(importConfig[componentType].(map[interface{}]interface{}), []string{
				"backend",
				"backend_type",
				"env",
				"remote_state_backend",
				"remote_state_backend_type",
				"settings",
				"vars",
			}) {
				deps = append(deps, imp)
				continue
			}
		}

		if i, ok := importConfig["components"]; ok {
			componentsSection := i.(map[interface{}]interface{})

			if i2, ok2 := componentsSection[componentType]; ok2 {
				componentTypeSection := i2.(map[interface{}]interface{})

				if i3, ok3 := componentTypeSection[component]; ok3 {
					componentSection := i3.(map[interface{}]interface{})

					if len(componentSection) > 0 {
						deps = append(deps, imp)
						continue
					}
				}

				if baseComponent != "" {
					if i3, ok3 := componentTypeSection[baseComponent]; ok3 {
						baseComponentSection := i3.(map[interface{}]interface{})

						if len(baseComponentSection) > 0 {
							deps = append(deps, imp)
							continue
						}
					}
				}
			}
		}
	}

	deps = append(deps, stack)
	unique := u.UniqueStrings(deps)
	sort.Strings(unique)
	return unique, nil
}

// sectionContainsAnyNotEmptySections checks if a section contains any of the provided low-level sections and it's not empty
func sectionContainsAnyNotEmptySections(section map[interface{}]interface{}, sectionsToCheck []string) bool {
	for _, s := range sectionsToCheck {
		if len(s) > 0 {
			if v, ok := section[s]; ok {
				if v2, ok2 := v.(map[interface{}]interface{}); ok2 && len(v2) > 0 {
					return true
				}
				if v2, ok2 := v.(string); ok2 && len(v2) > 0 {
					return true
				}
			}
		}
	}
	return false
}

// CreateComponentStackMap accepts a config file and creates a map of component-stack dependencies
func CreateComponentStackMap(basePath string, filePath string) (map[string]map[string][]string, error) {
	stackComponentMap := map[string]map[string][]string{}
	stackComponentMap["terraform"] = map[string][]string{}
	stackComponentMap["helmfile"] = map[string][]string{}

	componentStackMap := map[string]map[string][]string{}
	componentStackMap["terraform"] = map[string][]string{}
	componentStackMap["helmfile"] = map[string][]string{}

	dir := path.Dir(filePath)

	err := filepath.Walk(dir,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			isDirectory, err := u.IsDirectory(p)
			if err != nil {
				return err
			}

			isYaml := u.IsYaml(p)

			if !isDirectory && isYaml {
				config, _, err := ProcessYAMLConfigFile(basePath, p, map[string]map[interface{}]interface{}{})
				if err != nil {
					return err
				}

				finalConfig, err := ProcessConfig(
					basePath,
					p,
					config,
					false,
					false,
					"",
					nil,
					nil)
				if err != nil {
					return err
				}

				if componentsConfig, componentsConfigExists := finalConfig["components"]; componentsConfigExists {
					componentsSection := componentsConfig.(map[string]interface{})
					stackName := strings.Replace(p, basePath+"/", "", 1)

					if terraformConfig, terraformConfigExists := componentsSection["terraform"]; terraformConfigExists {
						terraformSection := terraformConfig.(map[string]interface{})

						for k := range terraformSection {
							stackComponentMap["terraform"][stackName] = append(stackComponentMap["terraform"][stackName], k)
						}
					}

					if helmfileConfig, helmfileConfigExists := componentsSection["helmfile"]; helmfileConfigExists {
						helmfileSection := helmfileConfig.(map[string]interface{})

						for k := range helmfileSection {
							stackComponentMap["helmfile"][stackName] = append(stackComponentMap["helmfile"][stackName], k)
						}
					}
				}
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	for stack, components := range stackComponentMap["terraform"] {
		for _, component := range components {
			componentStackMap["terraform"][component] = append(componentStackMap["terraform"][component], strings.Replace(stack, g.DefaultStackConfigFileExtension, "", 1))
		}
	}

	for stack, components := range stackComponentMap["helmfile"] {
		for _, component := range components {
			componentStackMap["helmfile"][component] = append(componentStackMap["helmfile"][component], strings.Replace(stack, g.DefaultStackConfigFileExtension, "", 1))
		}
	}

	return componentStackMap, nil
}

// getFileContent tries to read and return the file content from the sync map if it exists in the map,
// otherwise it reads the file, stores its content in the map and returns the content
func getFileContent(filePath string) (string, error) {
	existingContent, found := getFileContentSyncMap.Load(filePath)
	if found == true && existingContent != nil {
		return fmt.Sprintf("%s", existingContent), nil
	}

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	getFileContentSyncMap.Store(filePath, content)

	return string(content), nil
}

// GetGlobMatches tries to read and return the Glob matches content from the sync map if it exists in the map,
// otherwise it finds and returns all files matching the pattern, stores the files in the map and returns the files
func GetGlobMatches(pattern string) ([]string, error) {
	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found == true && existingMatches != nil {
		return strings.Split(fmt.Sprintf("%s", existingMatches), ","), nil
	}

	base, cleanPattern := doublestar.SplitPattern(pattern)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		u.PrintError(errors.New(fmt.Sprintf("Import of %s (-> %s + %s) failed to find a match.", pattern, base, cleanPattern)))
		return nil, nil
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, path.Join(base, match))
	}

	getGlobMatchesSyncMap.Store(pattern, strings.Join(fullMatches, ","))

	return fullMatches, nil
}
