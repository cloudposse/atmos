// A native CLI stand-in for adoption tests. Keep its startup independent of
// Atmos initialization and the test process's race instrumentation.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := filepath.Base(os.Args[0])
	switch {
	case len(os.Args) == 2 && os.Args[1] == "--version" && strings.HasPrefix(name, "terragrunt"):
		fmt.Fprintln(os.Stdout, "terragrunt version v1.2.1")
	case len(os.Args) == 2 && os.Args[1] == "version" && strings.HasPrefix(name, "tofu"):
		fmt.Fprintln(os.Stdout, "OpenTofu v8.7.6")
	case len(os.Args) == 2 && os.Args[1] == "version":
		fmt.Fprintln(os.Stdout, "Terraform v8.7.6")
	default:
		os.Exit(1)
	}
}
