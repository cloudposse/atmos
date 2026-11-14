package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processEnvTag processes the !env tag.
func processEnvTag(strFunc, nodeValue string) (any, error) {
	envValue, err := u.ProcessTagEnv(strFunc, nil)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, nodeValue, err)
	}
	return strings.TrimSpace(envValue), nil
}

// processExecTag processes the !exec tag.
func processExecTag(strFunc, nodeValue string) (any, error) {
	execValue, err := u.ProcessTagExec(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, nodeValue, err)
	}
	return execValue, nil
}

// processIncludeTag processes the !include tag.
func processIncludeTag(nodeTag, nodeValue, strFunc string) (any, error) {
	includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", "include_data", nodeTag, nodeValue))
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, nodeValue, err)
	}
	if includeValue != nil {
		if data, ok := includeValue[includeDataKey]; ok {
			return data, nil
		}
	}
	return nil, nil
}

// processGitRootTag processes the !repo-root tag.
func processGitRootTag(strFunc, nodeValue string) (any, error) {
	gitRootValue, err := atmosGit.ProcessTagRoot(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitRootValue), nil
}

// processGitShaTag processes the !git.sha and !git.ref tags.
func processGitShaTag(strFunc, nodeValue string) (any, error) {
	gitShaValue, err := atmosGit.ProcessTagSHA(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitShaValue), nil
}

// processGitBranchTag processes the !git.branch tag.
func processGitBranchTag(strFunc, nodeValue string) (any, error) {
	gitBranchValue, err := atmosGit.ProcessTagBranch(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitBranchValue), nil
}

// processGitRepoInfoTag processes the repository-metadata tags (!git.repository,
// !git.owner, !git.name, !git.host, !git.url) using the supplied processor.
func processGitRepoInfoTag(strFunc, nodeValue string, process func(string) (string, error)) (any, error) {
	value, err := process(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(value), nil
}

// processCwdTag processes the !cwd tag.
func processCwdTag(strFunc, nodeValue string) (any, error) {
	cwdValue, err := u.ProcessTagCwd(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncCwd, nodeValue, err)
	}
	return strings.TrimSpace(cwdValue), nil
}

// processRandomTag processes the !random tag.
func processRandomTag(strFunc, nodeValue string) (any, error) {
	randomValue, err := u.ProcessTagRandom(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncRandom, nodeValue, err)
	}
	return randomValue, nil
}

// processScalarNodeValue evaluates a YAML scalar node's custom Atmos tag and returns the resolved value.
// It supports the atmos.yaml YAML tags registered in pkg/function/tag; failures
// during evaluation return an error wrapped with ErrExecuteYamlFunctions, and
// unknown/unsupported custom tags return ErrUnsupportedYamlTag.
func processScalarNodeValue(node *yaml.Node) (any, error) {
	if hasCustomTag(node.Tag) {
		if err := admission.FunctionValue(context.Background(), node.Tag, node.Tag+" "+node.Value); err != nil {
			return nil, err
		}
	}
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)

	if isStandardYAMLTag(node.Tag) {
		return decodePlainYamlNode(node)
	}

	processors := map[string]func(string, string) (any, error){
		u.AtmosYamlFuncEnv:    processEnvTag,
		u.AtmosYamlFuncExec:   processExecTag,
		u.AtmosYamlFuncCwd:    processCwdTag,
		u.AtmosYamlFuncRandom: processRandomTag,
	}
	if process, ok := processors[node.Tag]; ok {
		return process(strFunc, node.Value)
	}

	switch node.Tag {
	case u.AtmosYamlFuncCEL:
		return (yamlsource.Source{Tag: node.Tag, Argument: node.Value, Line: node.Line, Column: node.Column}).Encode(), nil
	case u.AtmosYamlFuncUnset:
		return nil, nil
	case u.AtmosYamlFuncLiteral:
		// !literal keeps the value exactly as written.
		return node.Value, nil
	case u.AtmosYamlFuncInclude, u.AtmosYamlFuncIncludeRaw:
		return processIncludeTag(node.Tag, node.Value, strFunc)
	default:
		return processGitScalarNodeValue(node, strFunc)
	}
}

func processGitScalarNodeValue(node *yaml.Node, strFunc string) (any, error) {
	switch node.Tag {
	case u.AtmosYamlFuncGitRoot, u.AtmosYamlFuncGitRootAlias:
		return processGitRootTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitSha, u.AtmosYamlFuncGitRef:
		return processGitShaTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitBranch:
		return processGitBranchTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitRepository:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagRepository)
	case u.AtmosYamlFuncGitOwner:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagOwner)
	case u.AtmosYamlFuncGitName:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagName)
	case u.AtmosYamlFuncGitHost:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagHost)
	case u.AtmosYamlFuncGitUrl:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagURL)
	default:
		return nil, unsupportedAtmosYamlTagError(node.Tag, "")
	}
}

// processScalarNode processes a YAML scalar node tagged with an Atmos custom function and stores the resolved value in v.
// It dispatches handling for atmos.yaml-supported YAML tags to their respective handlers.
// If the node has no tag or a native YAML tag, the function is a no-op.
// It returns any error produced by the invoked handler.
func processScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if hasCustomTag(node.Tag) {
		if err := admission.FunctionValue(context.Background(), node.Tag, node.Tag+" "+node.Value); err != nil {
			return err
		}
	}
	if node.Tag == "" {
		return nil
	}

	if isStandardYAMLTag(node.Tag) {
		return nil
	}

	switch node.Tag {
	case u.AtmosYamlFuncCEL:
		node.Value = (yamlsource.Source{Tag: node.Tag, Argument: node.Value, File: v.ConfigFileUsed(), Line: node.Line, Column: node.Column}).Encode()
		node.Tag = "!!str"
		v.Set(currentPath, node.Value)
		return nil
	case "!version":
		return handleSkillVersion(node, v, currentPath)
	case u.AtmosYamlFuncUnset:
		// The !unset tag is handled in processMappingNode by skipping the key.
		// If we reach here, it means !unset was used in a context where it can't
		// prevent the key from being added (e.g., scalar value context).
		// In this case, we simply don't set any value and clear the tag.
		log.Debug("Unsetting configuration key", "path", currentPath)
		node.Tag = "" // Avoid re-processing.
		return nil
	case u.AtmosYamlFuncLiteral:
		// !literal keeps the value exactly as written. Viper already holds that value, because it
		// reads an explicitly tagged scalar as a plain string, so there is nothing to set. Pin the
		// node to a string so a later decode of the node cannot re-type the value (for example 123).
		node.Tag = "!!str"
		return nil

	default:
		return handleScalarFunction(node, v, currentPath)
	}
}

func handleSkillVersion(node *yaml.Node, v *viper.Viper, currentPath string) error {
	parts := strings.Split(currentPath, ".")
	if len(parts) != 4 || parts[0] != "ai" || parts[1] != "skills" || parts[3] != "ref" || strings.TrimSpace(node.Value) == "" {
		return unsupportedAtmosYamlTagError(node.Tag, currentPath)
	}
	node.Value = schema.SkillVersionMarker + node.Value
	node.Tag = "!!str"
	v.Set(currentPath, node.Value)
	return nil
}

func handleScalarFunction(node *yaml.Node, v *viper.Viper, currentPath string) error {
	handlers := map[string]func(*yaml.Node, *viper.Viper, string) error{
		u.AtmosYamlFuncEnv:        handleEnv,
		u.AtmosYamlFuncExec:       handleExec,
		u.AtmosYamlFuncInclude:    handleInclude,
		u.AtmosYamlFuncIncludeRaw: handleInclude,
		u.AtmosYamlFuncCwd:        handleCwd,
		u.AtmosYamlFuncRandom:     handleRandom,
	}
	if handler, ok := handlers[node.Tag]; ok {
		return handler(node, v, currentPath)
	}
	return handleGitScalarNode(node, v, currentPath)
}

func handleGitScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	switch node.Tag {
	case u.AtmosYamlFuncGitRoot, u.AtmosYamlFuncGitRootAlias:
		return handleGitRoot(node, v, currentPath)
	case u.AtmosYamlFuncGitSha, u.AtmosYamlFuncGitRef:
		return handleGitSha(node, v, currentPath)
	case u.AtmosYamlFuncGitBranch:
		return handleGitBranch(node, v, currentPath)
	case u.AtmosYamlFuncGitRepository:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagRepository)
	case u.AtmosYamlFuncGitOwner:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagOwner)
	case u.AtmosYamlFuncGitName:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagName)
	case u.AtmosYamlFuncGitHost:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagHost)
	case u.AtmosYamlFuncGitUrl:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagURL)
	default:
		return unsupportedAtmosYamlTagError(node.Tag, currentPath)
	}
}

func unsupportedAtmosYamlTagError(tag, currentPath string) error {
	supportedTags := strings.Join(fntag.AtmosConfigYAML(), ", ")
	if currentPath == "" {
		return fmt.Errorf("%w: '%s'. Supported tags for atmos.yaml are: %s",
			errUtils.ErrUnsupportedYamlTag, tag, supportedTags)
	}
	return fmt.Errorf("%w: '%s' at path '%s'. Supported tags for atmos.yaml are: %s",
		errUtils.ErrUnsupportedYamlTag, tag, currentPath, supportedTags)
}

// handleEnv processes a YAML node with an !env tag and sets the value in Viper, returns an error if the processing fails, warns if the value is empty.
func handleEnv(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	// In atmos.yaml processing, we don't have stack context, so pass nil.
	// This will make !env fall back to OS environment variables only.
	envValue, err := u.ProcessTagEnv(strFunc, nil)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, node.Value, err)
	}
	envValue = strings.TrimSpace(envValue)
	if envValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper .
	v.Set(currentPath, envValue)
	node.Tag = "" // Avoid re-processing .
	return nil
}

// handleExec Process the !exec tag and set the value in Viper , returns an error if the processing fails, warns if the value is empty.
func handleExec(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	execValue, err := u.ProcessTagExec(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, node.Value, err)
	}
	if execValue != nil {
		// Set the value in Viper .
		v.Set(currentPath, execValue)
	} else {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	node.Tag = "" // Avoid re-processing
	return nil
}

// handleInclude Process the !include tag and set the value in Viper , returns an error if the processing fails, warns if the value is empty.
func handleInclude(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", includeDataKey, node.Tag, node.Value))
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
	}
	if includeValue == nil {
		log.Debug(emptyValueWarning, functionKey, strFunc)
		node.Tag = "" // Avoid re-processing.
		return nil
	}
	data, ok := includeValue[includeDataKey]
	if !ok {
		log.Warn("Invalid value returned from the YAML function", functionKey, strFunc, "value", includeValue)
		return nil
	}
	return storeIncludeValue(node, v, currentPath, data)
}

func storeIncludeValue(node *yaml.Node, v *viper.Viper, currentPath string, data any) error {
	if err := node.Encode(data); err != nil {
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
	}
	encodeDevcontainerKeys(node, currentPath)
	if err := node.Decode(&data); err != nil {
		return err
	}
	v.Set(currentPath, data)
	if err := node.Encode(data); err != nil {
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
	}
	if err := preserveEmptyGitPushRetry(node, v, currentPath); err != nil {
		return err
	}
	return preserveEmptyGitPushRetryInPush(node, v, currentPath)
}

// handleCwd evaluates a `!cwd` YAML tag and stores the current working directory string into Viper at the given path.
// If a path argument is provided, it is joined with CWD.
// If evaluation fails, it returns an error wrapped with ErrExecuteYamlFunctions.
func handleCwd(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	cwdValue, err := u.ProcessTagCwd(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncCwd, node.Value, err)
	}
	cwdValue = strings.TrimSpace(cwdValue)
	if cwdValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper.
	v.Set(currentPath, cwdValue)
	node.Tag = "" // Avoid re-processing.
	return nil
}

// handleRandom evaluates a YAML scalar tagged with !random and stores the result in the provided Viper instance.
//
// If evaluation succeeds, the resulting value is stored at the given Viper key path (`currentPath`) and the node's
// tag is cleared to avoid re-processing. If the underlying random tag processor returns an error, that error is
// logged and returned wrapped with ErrExecuteYamlFunctions.
func handleRandom(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	randomValue, err := u.ProcessTagRandom(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncRandom, node.Value, err)
	}
	// Set the value in Viper.
	v.Set(currentPath, randomValue)
	node.Tag = "" // Avoid re-processing.
	return nil
}
