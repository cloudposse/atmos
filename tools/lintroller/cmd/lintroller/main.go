package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	lintroller "github.com/cloudposse/atmos/tools/lintroller"
)

func main() {
	singlechecker.Main(lintroller.Analyzer)
}
