package shell

import (
	"os"
	"os/signal"
	"sync"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/signals"
)

var (
	// The terminationGrace value bounds how long Atmos waits, after forwarding a termination
	// signal, for the child to exit before Atmos itself exits.
	terminationGrace = 30 * time.Second

	// The escalationDelay value is how long Atmos waits after each forwarded termination signal
	// before sending the next one in the escalation list (for example SIGHUP after
	// SIGTERM, which an interactive shell ignores).
	escalationDelay = 2 * time.Second

	// The terminalDeliversInterruptFn function reports whether the terminal already delivers Ctrl-C
	// to the child. Tests replace it to exercise both paths without a terminal.
	terminalDeliversInterruptFn = terminalDeliversInterrupt
)

// superviseChild keeps a running child process in step with signals sent to Atmos
// until the returned stop function is called (after the child has been waited for).
//
//   - SIGINT: Atmos does not exit while the child runs (the child owns Ctrl-C). When
//     the terminal already delivered the interrupt to the child's process group it is
//     not sent again, so a program such as Terraform does not see a second Ctrl-C;
//     otherwise (for example `kill -INT <atmos pid>`) it is forwarded to the child.
//   - SIGTERM: the main signal handler runs the exit cleanups before it exits, and the
//     cleanup registered here forwards the escalation signals to the child (one at a
//     time, escalationDelay apart) and waits up to terminationGrace for it to exit, so
//     the child is not left running after Atmos is gone.
//
// On Windows the console delivers Ctrl-C to every attached process and there is no
// SIGTERM, so only the SIGINT suspension applies.
func superviseChild(proc *os.Process, escalation []os.Signal) (stop func()) {
	done := make(chan struct{})
	releaseInterrupt := signals.SuspendInterruptExit()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	listenerDone := make(chan struct{})
	go func() {
		defer close(listenerDone)
		for {
			select {
			case <-interrupts:
				if !terminalDeliversInterruptFn() {
					forwardSignal(proc, os.Interrupt)
				}
			case <-done:
				return
			}
		}
	}()

	deregister := signals.RegisterExitCleanup(func() {
		terminateChild(proc, escalation, done)
	})

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			signal.Stop(interrupts)
			<-listenerDone
			deregister()
			releaseInterrupt()
		})
	}
}

// terminateChild forwards the escalation signals to the child until it exits, then
// waits for it up to terminationGrace. The done channel is closed once the child has exited.
func terminateChild(proc *os.Process, escalation []os.Signal, done <-chan struct{}) {
	deadline := time.NewTimer(terminationGrace)
	defer deadline.Stop()

	for i, sig := range escalation {
		forwardSignal(proc, sig)
		if i == len(escalation)-1 {
			break
		}
		step := time.NewTimer(escalationDelay)
		select {
		case <-done:
			step.Stop()
			return
		case <-deadline.C:
			step.Stop()
			log.Debug("Child process did not exit after the forwarded signal", "pid", proc.Pid)
			return
		case <-step.C:
		}
	}

	select {
	case <-done:
	case <-deadline.C:
		log.Debug("Child process did not exit after the forwarded signal", "pid", proc.Pid)
	}
}

// forwardSignal sends sig to the child, ignoring a child that already exited and
// signals the platform cannot deliver.
func forwardSignal(proc *os.Process, sig os.Signal) {
	if err := proc.Signal(sig); err != nil {
		log.Debug("Could not forward signal to child process", "pid", proc.Pid, "signal", sig, "error", err)
	}
}
