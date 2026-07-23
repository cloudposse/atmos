package aks

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// updateKubeconfigParser handles flag parsing with Viper precedence.
var updateKubeconfigParser *flags.StandardParser

// updateKubeconfigCmd executes 'azure aks update-kubeconfig' command.
var updateKubeconfigCmd = &cobra.Command{
	Use:   "update-kubeconfig",
	Short: "Update `kubeconfig` for an AKS cluster",
	Long: `Download the ` + "`" + `kubeconfig` + "`" + ` for an AKS cluster and save it to a file, using the Azure Go SDK.

The command executes in two ways:

1. If ` + "`" + `--integration` + "`" + ` is provided, ` + "`" + `atmos` + "`" + ` executes the named ` + "`" + `auth.integrations` + "`" + ` entry
   (authenticates via the auth manager, describes the cluster, and writes the kubeconfig).

2. If ` + "`" + `--cluster-name` + "`" + `, ` + "`" + `--resource-group` + "`" + `, and ` + "`" + `--identity` + "`" + ` are provided directly, ` + "`" + `atmos` + "`" + `
   authenticates the identity and writes the kubeconfig without requiring an ` + "`" + `atmos.yaml` + "`" + ` integration entry.

See https://atmos.tools/cli/commands/azure/aks/update-kubeconfig for more information.`,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "aks.updateKubeconfig.RunE")()

		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := updateKubeconfigParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		flags := cmd.Flags()

		// If --integration is specified, use the auth manager to execute the named integration.
		integration, _ := flags.GetString("integration")
		if integration != "" {
			return executeAKSUpdateKubeconfigViaIntegration(integration)
		}

		// If --cluster-name and --resource-group are provided with --identity, use the direct SDK path.
		clusterName, _ := flags.GetString("cluster-name")
		resourceGroup, _ := flags.GetString("resource-group")
		identity, _ := flags.GetString("identity")
		if clusterName != "" && resourceGroup != "" && identity != "" {
			subscriptionID, _ := flags.GetString("subscription-id")
			kubeconfig, _ := flags.GetString("kubeconfig")
			alias, _ := flags.GetString("alias")
			return executeAKSUpdateKubeconfigDirect(&aksKubeconfigDirectParams{
				clusterName:    clusterName,
				resourceGroup:  resourceGroup,
				subscriptionID: subscriptionID,
				kubeconfigPath: kubeconfig,
				alias:          alias,
				identityName:   identity,
			})
		}

		return fmt.Errorf("%w: specify --integration, or --cluster-name, --resource-group, and --identity", errUtils.ErrAKSIntegrationFailed)
	},
}

func init() {
	// Create parser with update-kubeconfig-specific flags using functional options.
	updateKubeconfigParser = flags.NewStandardParser(
		flags.WithViperPrefix("aks"),
		flags.WithStringFlag("cluster-name", "", "", "Specify the name of the AKS cluster to update the kubeconfig for"),
		flags.WithStringFlag("resource-group", "", "", "Specify the Azure resource group where the AKS cluster is located"),
		flags.WithStringFlag("subscription-id", "", "", "Specify the Azure subscription ID (optional, falls back to identity's subscription)"),
		flags.WithStringFlag("kubeconfig", "", "", "Specify the path to the kubeconfig file to be updated or created for accessing the AKS cluster."),
		flags.WithStringFlag("alias", "", "", "Specify an alias to use for the cluster context name in the kubeconfig file."),
		flags.WithStringFlag("integration", "", "", "Named integration from auth.integrations"),
		flags.WithStringFlag("identity", "i", "", "Atmos identity to authenticate with"),
		// Environment variable bindings.
		flags.WithEnvVars("kubeconfig", "ATMOS_KUBECONFIG", "KUBECONFIG"),
	)

	// Register flags with Cobra command.
	updateKubeconfigParser.RegisterFlags(updateKubeconfigCmd)

	// Bind to Viper for environment variable support.
	if err := updateKubeconfigParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	AksCmd.AddCommand(updateKubeconfigCmd)
}
