package cloudformation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	configFileMode = 0o644
	configDirMode  = 0o755
	lockDirMode    = 0o700
)

const starterConfig = `base_path: ./
components:
  aws/cloudformation:
    base_path: .
stacks:
  base_path: stacks
  included_paths:
    - "*.yaml"
`

var (
	awsName      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,127}$`)
	manifestName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
)

func newInitCommand() *cobra.Command {
	var names []string
	var apply bool
	cmd := &cobra.Command{
		Use: "init [template]", Short: "Choose a CloudFormation template and register one or more AWS stacks", Args: cobra.MaximumNArgs(1),
		Example: "  atmos aws cfn init\n  atmos aws cfn init templates/queue.yaml -s dev --name orders-blue --name orders-green",
	}
	flags.NewStandardParser(flags.WithIdentityFlag()).RegisterFlags(cmd)
	cmd.Flags().StringArrayVar(&names, "name", nil, "AWS stack and component name (repeatable, defaults to the template filename stem)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Preview and interactively apply each requested component after initialization")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		info := buildConfigAndStacksInfo(cmd)
		interactive := flags.IsInteractive() && !ci.ModeEnabled(cmd)
		path, err := selectInitTemplate(cmd, args, interactive)
		if err != nil {
			return err
		}
		source, err := initDeployment(cmd, path, names, &info, interactive)
		if err != nil {
			return err
		}
		return initializeTemplate(cmd, source, &info, apply)
	}
	internal.RegisterCommandMetadata(cmd, internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/cloudformation/init"))
	return cmd
}

func initializeTemplate(cmd *cobra.Command, source templateSource, info *schema.ConfigAndStacksInfo, apply bool) error {
	config, fresh, err := initTemplateConfig(info)
	if err != nil {
		return err
	}
	register := func() error { return registerTemplate(cmd, config, fresh, info, source) }
	if info.DryRun {
		return register()
	}
	if err := withInitLock(cmd, config, register); err != nil {
		return err
	}
	if !apply {
		return nil
	}
	return applyInitialized(cmd, info.Stack, source.names)
}

func applyInitialized(cmd *cobra.Command, stackName string, names []string) error {
	// Start apply with fresh project discovery. The current process may already
	// have cached both the old project configuration and the old stack inventory.
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	forwarded := []string{}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "apply", "name", "stack", "chdir":
			return
		}
		if values, ok := f.Value.(pflag.SliceValue); ok {
			for _, value := range values.GetSlice() {
				forwarded = append(forwarded, "--"+f.Name+"="+value)
			}
			return
		}
		forwarded = append(forwarded, "--"+f.Name+"="+f.Value.String())
	})
	for _, name := range names {
		args := append([]string{"aws", "cfn", "apply", name, "-s", stackName}, forwarded...)
		result := process.NewDefaultRunner().Run(cmd.Context(), process.TaskSpec{
			Command: binary, Args: args, Streams: process.Streams{Stdin: cmd.InOrStdin(), Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()},
		})
		if err := result.Err; err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func initNames(path string, names []string) ([]string, error) {
	if len(names) == 0 {
		base := filepath.Base(path)
		names = []string{strings.TrimSuffix(base, filepath.Ext(base))}
	}
	distinct := make([]string, 0, len(names))
	for _, name := range names {
		if !awsName.MatchString(name) {
			return nil, fmt.Errorf("%w: invalid AWS stack name %q: use --name with a letter followed by up to 127 letters, digits or hyphens", errUtils.ErrInvalidFlagValue, name)
		}
		if !slices.Contains(distinct, name) {
			distinct = append(distinct, name)
		}
	}
	return distinct, nil
}

func templatePath(template string) (string, error) {
	path, err := filepath.Abs(template)
	if err != nil {
		return "", err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !stat.Mode().IsRegular() {
		return "", fmt.Errorf("%w: template must be a regular file: %s", errUtils.ErrInvalidFlagValue, template)
	}
	return path, nil
}

func initTemplateConfig(info *schema.ConfigAndStacksInfo) (*schema.AtmosConfiguration, bool, error) {
	config, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		return nil, false, err
	}
	fresh := len(cfg.LoadedConfigFiles()) == 0
	if fresh {
		root, err := os.Getwd()
		if err != nil {
			return nil, false, err
		}
		config.BasePathAbsolute = root
		config.CloudFormationDirAbsolutePath = root
		config.StacksBaseAbsolutePath = filepath.Join(root, "stacks")
		config.Stacks = schema.Stacks{IncludedPaths: []string{"*.yaml"}}
	}
	return &config, fresh, nil
}

func withInitLock(cmd *cobra.Command, config *schema.AtmosConfiguration, register func() error) error {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	lockDir := filepath.Join(cacheDir, "atmos", "locks")
	if err := os.MkdirAll(lockDir, lockDirMode); err != nil {
		return err
	}
	lock := filelock.New(filepath.Join(lockDir, fmt.Sprintf("cfn-init-%x.lock", sha256.Sum256([]byte(config.StacksBaseAbsolutePath)))))
	if err := lock.WithExclusive(cmd.Context(), register); err != nil {
		return err
	}
	return nil
}
