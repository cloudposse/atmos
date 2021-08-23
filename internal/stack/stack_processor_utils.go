package stack

import (
	g "atmos/internal/globals"
	u "atmos/internal/utils"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
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
// 1. The imported config file has the global `vars` section and it's not empty
// 2. The imported config file has the component type section, which has a `vars` section which is not empty
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
		if i, ok := importConfig["vars"]; ok {
			globalVarsSection := i.(map[interface{}]interface{})

			if len(globalVarsSection) > 0 {
				deps = append(deps, imp)
				continue
			}
		}

		if i, ok := importConfig[componentType]; ok {
			componentTypeSection := i.(map[interface{}]interface{})

			if i2, ok2 := componentTypeSection["vars"]; ok2 {
				componentTypeVarsSection := i2.(map[interface{}]interface{})

				if len(componentTypeVarsSection) > 0 {
					deps = append(deps, imp)
					continue
				}
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

// CreateComponentStackMap accepts a config file and creates a map of component-stack dependencies
func CreateComponentStackMap(filePath string) (map[string]map[string][]string, error) {
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
				config, _, err := ProcessYAMLConfigFile(dir, p, map[string]map[interface{}]interface{}{})
				if err != nil {
					return err
				}

				finalConfig, err := ProcessConfig(p,
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
					stackName := strings.Replace(p, dir+"/", "", 1)

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

// FindAllStackConfigsInPaths finds all stack config files in the paths specified by globs
func FindAllStackConfigsInPaths(pathGlobs []string) ([]string, error) {
	res := []string{}

	for _, p := range pathGlobs {
		pathWithExt := p

		ext := filepath.Ext(p)
		if ext == "" {
			ext = g.DefaultStackConfigFileExtension
			pathWithExt = p + ext
		}

		// Find all matches in the glob
		matches, err := filepath.Glob(pathWithExt)
		if err != nil {
			return nil, err
		}

		if matches != nil && len(matches) > 0 {
			res = append(res, matches...)
		}

	}

	return res, nil
}
