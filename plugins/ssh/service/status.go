package service

import (
	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// fillVersions sets the version fields of st. The installed version comes from the stable file
// name of the installed executable. The daemon version comes from the state file, but only when
// the state file belongs to the process the service manager says is running, so a state file left
// by a crashed daemon is never mistaken for the live one.
func fillVersions(st *Status, p *paths.Paths) {
	if v, ok := ParseStableVersion(st.Executable); ok {
		st.InstalledVersion = v
	}
	if st.Running {
		state, err := agent.ReadState(p.StateFile())
		if err == nil && (st.PID == 0 || state.PID == st.PID) {
			st.DaemonVersion = state.Version
		}
	}
	st.VersionMismatch = st.InstalledVersion != "" && st.DaemonVersion != "" &&
		st.InstalledVersion != st.DaemonVersion
}
