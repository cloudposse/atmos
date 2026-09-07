// Command generator regenerates the embedded atmos.yaml JSON Schema artifact
// from the Go configuration structs. It is invoked by `go generate
// ./pkg/config/schema` and never ships in the atmos binary.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	configschema "github.com/cloudposse/atmos/pkg/config/schema"
)

const (
	outputDirPerm  = 0o755
	outputFilePerm = 0o644
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	out, err := configschema.Generate()
	if err != nil {
		return err
	}
	repoRoot, err := configschema.RepoRoot()
	if err != nil {
		return err
	}
	target := filepath.Join(repoRoot, filepath.FromSlash(configschema.EmbeddedPath))
	if err := os.MkdirAll(filepath.Dir(target), outputDirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(target, out, outputFilePerm); err != nil { // #nosec G306 -- the JSON Schema is a non-sensitive project file.
		return err
	}
	fmt.Fprintln(os.Stderr, target)
	return nil
}
