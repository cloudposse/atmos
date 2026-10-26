//go:build ignore

// gen_schema.go regenerates the embedded JSON Schema for the
// AtmosScaffoldConfig manifest kind from its Go spec type.
//
// Run via: go generate ./pkg/project/config/
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/project/config"
)

func main() {
	def, ok := manifest.GetDefinition(config.ScaffoldKind)
	if !ok {
		fmt.Fprintf(os.Stderr, "manifest kind %s is not registered\n", config.ScaffoldKind)
		os.Exit(1)
	}

	out := filepath.Join("..", "..", "datafetcher", "schema", "scaffold", "scaffold-config", "1.0.json")
	if err := os.WriteFile(out, []byte(def.SchemaJSON()+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write schema: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s\n", out)
}
