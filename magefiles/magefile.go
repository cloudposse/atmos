//go:build mage

// Package main assembles the repository's Mage targets from topic packages.
package main

import (
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/acceptance"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/bench"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/build"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/ci"
	//mage:import
	"github.com/cloudposse/atmos/magefiles/lint"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/notice"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/qualitygate"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/release"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/s3"
	//mage:import
	_ "github.com/cloudposse/atmos/magefiles/test"
)

// Default runs the changed-code lint checks.
var Default = lint.Lint.Changed
