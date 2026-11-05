package process

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestDarwinProcessGroupAlive(t *testing.T) {
	const exitingFlag = 0x2000 // P_WEXIT in Darwin's sys/proc.h.
	for _, tt := range []struct {
		name      string
		members   []unix.KinfoProc
		err       error
		wantAlive bool
	}{
		{name: "empty group"},
		{name: "unreaped zombie", members: []unix.KinfoProc{{Proc: unix.ExternProc{P_stat: 5}}}},
		{name: "running", members: []unix.KinfoProc{{Proc: unix.ExternProc{P_stat: 2}}}, wantAlive: true},
		{name: "sleeping", members: []unix.KinfoProc{{Proc: unix.ExternProc{P_stat: 3}}}, wantAlive: true},
		{name: "exit in progress", members: []unix.KinfoProc{{Proc: unix.ExternProc{P_stat: 2, P_flag: exitingFlag}}}, wantAlive: true},
		{name: "zombie leader with running descendant", members: []unix.KinfoProc{{Proc: unix.ExternProc{P_stat: 5}}, {Proc: unix.ExternProc{P_stat: 2}}}, wantAlive: true},
		{name: "snapshot unavailable", err: syscall.EPERM, wantAlive: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantAlive, darwinProcessGroupAlive(tt.members, tt.err))
		})
	}
}
