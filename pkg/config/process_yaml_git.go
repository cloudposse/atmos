package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// handleGitRoot evaluates an `!repo-root` YAML tag and stores the resulting repository root string into Viper at the given path.
// If evaluation fails, it returns an error wrapped with ErrExecuteYamlFunctions; if the result is empty it logs a debug warning but still sets the value.
func handleGitRoot(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	gitRootValue, err := atmosGit.ProcessTagRoot(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, node.Value, err)
	}
	gitRootValue = strings.TrimSpace(gitRootValue)
	if gitRootValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper .
	v.Set(currentPath, gitRootValue)
	node.Tag = "" // Avoid re-processing .
	return nil
}

// handleGitSha evaluates a `!git.sha` or `!git.ref` YAML tag and stores the resulting commit SHA into Viper.
func handleGitSha(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	gitShaValue, err := atmosGit.ProcessTagSHA(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, node.Value, err)
	}
	gitShaValue = strings.TrimSpace(gitShaValue)
	if gitShaValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	v.Set(currentPath, gitShaValue)
	node.Tag = ""
	return nil
}

// handleGitBranch evaluates a `!git.branch` YAML tag and stores the resulting branch name into Viper.
func handleGitBranch(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	gitBranchValue, err := atmosGit.ProcessTagBranch(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, node.Value, err)
	}
	gitBranchValue = strings.TrimSpace(gitBranchValue)
	if gitBranchValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	v.Set(currentPath, gitBranchValue)
	node.Tag = ""
	return nil
}

// handleGitRepoInfo evaluates a repository-metadata YAML tag (!git.repository,
// !git.owner, !git.name, !git.host, !git.url) using the supplied processor and
// stores the resulting string into Viper at the given path.
func handleGitRepoInfo(node *yaml.Node, v *viper.Viper, currentPath string, process func(string) (string, error)) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	value, err := process(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, node.Value, err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	v.Set(currentPath, value)
	node.Tag = "" // Avoid re-processing.
	return nil
}
