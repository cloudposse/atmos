//go:build !windows

package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// daemon holds the sockets Serve currently runs.
type daemon struct {
	opts  Options
	audit *AuditLog
	// peer returns the connecting user's id. Tests replace it.
	peer func(*net.UnixConn) (uint32, error)

	mu      sync.Mutex
	sockets map[string]*socketEntry
	wg      sync.WaitGroup
}

// Serve runs the agent daemon: one socket per configured identity, reloaded when the configuration
// changes. It blocks until ctx ends, then closes every socket and removes the state file.
func Serve(ctx context.Context, opts Options) error {
	opts, err := opts.withDefaults()
	if err != nil {
		return err
	}
	if err := opts.Paths.EnsureDirs(); err != nil {
		return err
	}
	audit, err := OpenAuditLog(opts.Paths.AuditLogFile())
	if err != nil {
		return err
	}
	defer audit.Close()

	pid := os.Getpid()
	state := State{PID: pid, Version: opts.Version, Started: time.Now().UTC()}
	if err := WriteState(opts.Paths.StateFile(), state); err != nil {
		return err
	}
	defer func() {
		if err := RemoveState(opts.Paths.StateFile(), pid); err != nil {
			opts.Logf("%v", err)
		}
	}()

	d := &daemon{opts: opts, audit: audit, peer: peerUID, sockets: map[string]*socketEntry{}}
	if opts.peer != nil {
		d.peer = opts.peer
	}
	defer d.shutdown()
	d.reload()
	if opts.Ready != nil {
		opts.Ready()
	}
	d.watch(ctx)
	return nil
}

// watch reloads the configuration on SIGHUP and when the file changes, until ctx ends.
func (d *daemon) watch(ctx context.Context) {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)

	ticker := time.NewTicker(d.opts.PollInterval)
	defer ticker.Stop()

	last := fingerprintFile(d.opts.Paths.AgentConfigFile())
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			d.reload()
			last = fingerprintFile(d.opts.Paths.AgentConfigFile())
		case <-ticker.C:
			if now := fingerprintFile(d.opts.Paths.AgentConfigFile()); now != last {
				last = now
				d.reload()
			}
		}
	}
}

// fileFingerprint identifies a version of the configuration file.
type fileFingerprint struct {
	exists bool
	mtime  time.Time
	size   int64
}

func fingerprintFile(path string) fileFingerprint {
	info, err := os.Stat(path)
	if err != nil {
		return fileFingerprint{}
	}
	return fileFingerprint{exists: true, mtime: info.ModTime(), size: info.Size()}
}

// reload applies the configuration file. An invalid file leaves the running sockets untouched.
func (d *daemon) reload() {
	cfg, err := loadOrEmpty(d.opts.Paths.AgentConfigFile())
	if err != nil {
		d.opts.Logf("keeping the current configuration: %v", err)
		return
	}
	if err := d.apply(cfg); err != nil {
		d.opts.Logf("%v", err)
	}
}

// apply starts, restarts and stops sockets so they match cfg. Sockets whose settings did not
// change are left alone. Failures on one identity do not stop the others.
func (d *daemon) apply(cfg *Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	want := map[string]Identity{}
	for _, id := range cfg.Identities {
		want[id.Name] = id
	}
	for name, e := range d.sockets {
		if id, keep := want[name]; !keep || !id.equal(e.id) {
			e.close()
			delete(d.sockets, name)
		}
	}
	var errs []error
	for _, id := range cfg.Identities {
		if _, running := d.sockets[id.Name]; running {
			continue
		}
		e, err := d.startSocket(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("identity %q: %w", id.Name, err))
			continue
		}
		d.sockets[id.Name] = e
	}
	return errors.Join(errs...)
}

// shutdown closes every socket and waits for the connection goroutines.
func (d *daemon) shutdown() {
	d.mu.Lock()
	for name, e := range d.sockets {
		e.close()
		delete(d.sockets, name)
	}
	d.mu.Unlock()
	d.wg.Wait()
}
