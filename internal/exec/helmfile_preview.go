package exec

import (
	"fmt"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/helmfile"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// previewHelmfile retains configuration evaluation but stops before execution setup:
// authentication, tool installation, source provisioning, plugins, hooks and generated files.
func previewHelmfile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.SubCommand == "version" {
		printHelmfilePreview(info.Command, []string{info.SubCommand})
		return nil
	}

	resolved, err := ProcessStacks(atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return err
	}
	*info = resolved
	if !info.ComponentIsEnabled {
		ui.Writef("Component %q is not enabled and skipped\n", info.ComponentFromArg)
		return nil
	}
	if err := checkHelmfileConfig(atmosConfig); err != nil {
		return err
	}
	if err := checkHelmfileRestrictions(info); err != nil {
		return err
	}

	selected, err := selectHelmfilePreviewTarget(info)
	if err != nil {
		return err
	}
	if atmosConfig.Components.Helmfile.UseEKS {
		if err := previewHelmfileEKS(atmosConfig, info); err != nil {
			return err
		}
	}

	printHelmfileComponentPreview(info, selected)
	return nil
}

func selectHelmfilePreviewTarget(info *schema.ConfigAndStacksInfo) (*target.SelectedTarget, error) {
	flagTarget, nativeArgs := helmfile.ExtractTargetFlag(info.AdditionalArgsAndFlags)
	info.AdditionalArgsAndFlags = nativeArgs
	if flagTarget == "" {
		return nil, nil
	}
	section, _ := info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	return target.SelectTarget(section, flagTarget)
}

func printHelmfileComponentPreview(info *schema.ConfigAndStacksInfo, selected *target.SelectedTarget) {
	command := info.SubCommand
	if command == "deploy" {
		command = "sync"
	}
	if selected != nil && selected.Kind != target.KindKubernetes {
		command = "template"
	}
	printHelmfilePreview(info.Command, buildHelmfileArgs(info, constructHelmfileComponentVarfileName(info), command))
	if selected != nil && selected.Kind != target.KindKubernetes {
		ui.Writef("Would deliver rendered manifests to target %q (%s)\n", selected.Name, selected.Kind)
	}
}

func printHelmfilePreview(command string, args []string) {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellescape.Quote(iolib.MaskString(command)))
	for _, arg := range args {
		parts = append(parts, shellescape.Quote(iolib.MaskString(arg)))
	}
	ui.Writef("Dry run: %s\n", strings.Join(parts, " "))
}

func buildHelmfileArgs(info *schema.ConfigAndStacksInfo, varFile, subCommand string) []string {
	args := []string{"--state-values-file", varFile}
	args = append(args, info.GlobalOptions...)
	args = append(args, subCommand)
	return append(args, info.AdditionalArgsAndFlags...)
}

func checkHelmfileRestrictions(info *schema.ConfigAndStacksInfo) error {
	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	if (info.SubCommand == "sync" || info.SubCommand == "apply" || info.SubCommand == "deploy") && info.ComponentIsAbstract {
		return fmt.Errorf("%w: '%s' cannot be provisioned since it's explicitly prohibited from being deployed by 'metadata.type: abstract' attribute",
			errUtils.ErrAbstractComponentCantBeProvisioned, filepath.Join(info.ComponentFolderPrefix, info.Component))
	}
	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked {
		// Allow read-only commands, block modification commands.
		switch info.SubCommand {
		case "sync", "apply", "deploy", "delete", "destroy":
			return fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
				errUtils.ErrLockedComponentCantBeProvisioned, filepath.Join(info.ComponentFolderPrefix, info.Component))
		}
	}
	return nil
}

func previewHelmfileEKS(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	stackContext := cfg.GetContextFromVars(info.ComponentVarsSection)
	config := atmosConfig.Components.Helmfile
	cluster, err := helmfile.ResolveClusterName(helmfile.ClusterNameInput{
		FlagValue: info.ClusterName, ConfigValue: config.ClusterName,
		Template: config.ClusterNameTemplate, Pattern: config.ClusterNamePattern,
	}, &stackContext, atmosConfig, info.ComponentSection, ProcessTmpl)
	if err != nil {
		return err
	}
	identity := info.Identity
	if identity == "" {
		identity, err = authdeferred.DefaultIdentity(atmosConfig, info)
		if err != nil {
			return err
		}
	}
	auth, err := helmfile.ResolveAWSAuth(helmfile.AuthInput{
		Identity: identity, ProfilePattern: config.HelmAwsProfilePattern,
	}, &stackContext)
	if err != nil {
		return err
	}
	kubeconfigPath := filepath.Join(config.KubeconfigPath, info.ContextPrefix+"-kubecfg")
	printHelmfilePreview("aws", buildHelmfileEKSArgs(cluster.ClusterName, stackContext.Region, kubeconfigPath, auth.UseIdentityAuth, auth.Profile))
	return nil
}

func buildHelmfileEKSArgs(clusterName, region, kubeconfigPath string, useIdentityAuth bool, profile string) []string {
	args := []string{
		"eks", "update-kubeconfig", "--name=" + clusterName,
		"--region=" + region, "--kubeconfig=" + kubeconfigPath,
	}
	// Add profile flag if using deprecated profile pattern (not identity auth).
	if !useIdentityAuth && profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	return args
}
