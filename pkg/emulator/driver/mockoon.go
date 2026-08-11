package driver

import (
	"fmt"

	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/emulator/target"
)

const (
	mockoonImage               = "mockoon/cli:latest"
	mockoonPort                = 3000
	mockoonOnePasswordDataFile = "/data/1password-connect.json"
)

func init() {
	healthCheck := shellHealthCheck(fmt.Sprintf("node -e \"fetch('http://localhost:%d/v1/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))\"", mockoonPort))
	emu.RegisterDriver(&builtinDriver{
		name:        "mockoon/1password-connect",
		target:      emu.TargetOnePassword,
		image:       mockoonImage,
		ports:       []int{mockoonPort},
		command:     []string{"--data", mockoonOnePasswordDataFile, "--port", "3000", "--hostname", "0.0.0.0", "--disable-log-to-file"},
		healthCheck: healthCheck,
		restart:     defaultEmulatorRestart,
		profile:     target.OnePasswordProfile,
	})
}
