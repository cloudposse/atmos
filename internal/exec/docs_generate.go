package exec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/docs"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteDocsGenerateCmd implements the 'atmos docs generate <doc-type>' logic.
func ExecuteDocsGenerateCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteDocsGenerateCmd")()

	if len(args) == 0 {
		return errUtils.ErrMissingDocType
	}
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}
	rootConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	// Retrieve the <doc-type> generation config from atmos.yaml.
	// The target directory is taken from docs.generate.<doc-type>.base-dir.
	docsGenerate, ok := rootConfig.Docs.Generate[args[0]]
	if !ok {
		return fmt.Errorf("%w: %q", errUtils.ErrNoDocsGenerateEntry, args[0])
	}

	basedir := docsGenerate.BaseDir
	var targetDir string
	currDir, err := os.Getwd()
	if err != nil {
		return err
	}
	log.Debug("Current directory", "dir", currDir, "basedir", basedir)

	targetDir = filepath.Join(currDir, basedir)

	if len(docsGenerate.Input) == 0 {
		log.Debug("docs.generate['<doc-type>'].input is empty in atmos.yaml")
	}
	if len(docsGenerate.Template) == 0 {
		log.Debug("No docs.generate['<doc-type>'].template' defined, generating minimal <doc-type>.")
	}

	// Generate a single document in the targetDir using the default renderer.
	return docs.Generate(&rootConfig, targetDir, &docsGenerate)
}
