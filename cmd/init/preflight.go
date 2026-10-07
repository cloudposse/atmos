package initcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Preflight rejects occupied copy destinations using only arguments, environment,
// and local source metadata, before startup loads configuration or starts progress.
// Sources whose mode depends on configuration or a remote manifest are checked later.
func Preflight(args []string) error {
	defer perf.Track(nil, "initcmd.Preflight")()

	probe := &cobra.Command{}
	globalParser := flags.NewGlobalOptionsBuilder().Build()
	globalParser.RegisterPersistentFlags(probe)
	parser := newInitParser()
	parser.RegisterFlags(probe)
	if err := probe.ParseFlags(args); err != nil {
		return nil // Preserve normal command parsing and its error presentation.
	}
	positional := probe.Flags().Args()
	if len(positional) < 1 || len(positional) > 2 {
		return nil
	}
	v, err := bindPreflightOptions(probe, globalParser, parser)
	if err != nil {
		return err
	}
	if v.GetBool("force") || v.GetBool("update") || v.GetString("use-version") != "" {
		return nil
	}
	src, copySource := earlyCopySource(positional[0], v.GetBool("copy"))
	if !copySource {
		return nil
	}
	opts := &initOptions{copy: true}
	if len(positional) == 2 {
		opts.targetDir = resolveTargetDir(positional[1])
	}
	return preflightInitTarget(opts, src)
}

func bindPreflightOptions(probe *cobra.Command, globalParser, parser *flags.StandardParser) (*viper.Viper, error) {
	v := viper.New()
	if err := globalParser.BindFlagsToViper(probe, v); err != nil {
		return nil, err
	}
	if err := parser.BindFlagsToViper(probe, v); err != nil {
		return nil, err
	}
	return v, nil
}

func earlyCopySource(src string, explicitCopy bool) (string, bool) {
	configs, err := loadInitTemplateConfigs("")
	if err != nil {
		return src, false
	}
	if _, named := configs[src]; named {
		return src, false
	}
	// The examples shorthand may refer to a configured repository, whose
	// manifest is not available yet. Explicit --copy makes its mode unambiguous.
	if strings.HasPrefix(src, "examples/") {
		return src, explicitCopy
	}
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		_, manifestErr := os.Lstat(filepath.Join(src, "scaffold.yaml"))
		return resolveTargetDir(src), explicitCopy || errors.Is(manifestErr, os.ErrNotExist)
	}
	normalized, err := source.NormalizeInitSource(src, "")
	return src, err == nil && (explicitCopy || normalized.Copy)
}
