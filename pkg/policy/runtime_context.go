package policy

import (
	"context"
	"crypto/fips140"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"time"
	_ "time/tzdata" // Calendar policies work without an OS timezone database.

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// RuntimeContext is host-owned evidence, separate from the document being validated.
// Unknown measurements are null, never fabricated zeroes. It can contain secrets.
type RuntimeContext struct {
	Env     map[string]string `json:"env"`
	Atmos   AtmosFacts        `json:"atmos"`
	Runtime HostFacts         `json:"runtime"`
	Time    ClockFacts        `json:"time"`
}

// AtmosFacts describes the running executable and active crypto mode.
type AtmosFacts struct {
	Version string `json:"version"`
	FIPS    bool   `json:"fips"`
}

// HostFacts identifies the executable platform and sampled resource capacity.
type HostFacts struct {
	OS     string      `json:"os"`
	Arch   string      `json:"arch"`
	User   UserFacts   `json:"user"`
	Memory MemoryFacts `json:"memory"`
	Disk   DiskFacts   `json:"disk"`
}

// UserFacts uses effective process IDs, not environment-supplied identity.
type UserFacts struct {
	UID      *int    `json:"uid"`
	GID      *int    `json:"gid"`
	Username *string `json:"username"`
}

// MemoryFacts reports available host memory and a detected container ceiling.
type MemoryFacts struct {
	AvailableBytes *uint64 `json:"available_bytes"`
	TotalBytes     *uint64 `json:"total_bytes"`
	Scope          string  `json:"scope"`
}

// DiskFacts names the sampled filesystem location explicitly.
type DiskFacts struct {
	Path           string  `json:"path"`
	AvailableBytes *uint64 `json:"available_bytes"`
	TotalBytes     *uint64 `json:"total_bytes"`
}

// ClockFacts is one UTC instant shared by every rule in an evaluation.
type ClockFacts struct {
	Now      string `json:"now"`
	Date     string `json:"date"`
	Weekday  string `json:"weekday"`
	Timezone string `json:"timezone"`
}

// ContextProvider collects evidence outside pure language evaluators.
// Callers can inject deterministic snapshots without changing engine behavior.
//
//go:generate go run go.uber.org/mock/mockgen -source=runtime_context.go -destination=mock_runtime_context_test.go -package=policy
type ContextProvider interface {
	Collect(context.Context) (*RuntimeContext, error)
}

// HostContextProvider samples the process and its current working filesystem.
type HostContextProvider struct{}

// Collect reads OS facts without invoking commands, templates, or Atmos authentication.
func (HostContextProvider) Collect(ctx context.Context) (*RuntimeContext, error) {
	defer perf.Track(nil, "policy.HostContextProvider.Collect")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	facts := &RuntimeContext{
		Env: env.EnvironToMap(), Atmos: AtmosFacts{Version: version.Version, FIPS: fips140.Enabled()},
		Runtime: HostFacts{OS: runtime.GOOS, Arch: runtime.GOARCH, User: effectiveUser()},
		Time:    ClockFacts{Now: now.Format(time.RFC3339Nano), Date: now.Format(time.DateOnly), Weekday: now.Weekday().String(), Timezone: "UTC"},
	}
	if usage, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		facts.Runtime.Memory = MemoryFacts{AvailableBytes: &usage.Available, TotalBytes: &usage.Total, Scope: "host"}
		constrainContainerMemory(&facts.Runtime.Memory)
	}
	if path, err := os.Getwd(); err == nil {
		facts.Runtime.Disk.Path = path
		if usage, err := disk.UsageWithContext(ctx, path); err == nil {
			facts.Runtime.Disk.AvailableBytes, facts.Runtime.Disk.TotalBytes = &usage.Free, &usage.Total
		}
	}
	return facts, ctx.Err()
}

func effectiveUser() UserFacts {
	var result UserFacts
	uid, gid := os.Geteuid(), os.Getegid()
	if uid >= 0 {
		result.UID = &uid
	}
	if gid >= 0 {
		result.GID = &gid
	}
	// LookupId binds the display name to the effective ID, including after seteuid.
	if uid >= 0 {
		if account, err := user.LookupId(strconv.Itoa(uid)); err == nil {
			result.Username = &account.Username
		}
	} else if account, err := user.Current(); err == nil {
		result.Username = &account.Username
	}
	return result
}

// WithContextProvider replaces host collection for embedding or deterministic tests.
func WithContextProvider(provider ContextProvider) EngineOption {
	defer perf.Track(nil, "policy.WithContextProvider")()

	return func(engine *Engine) { engine.contextProvider = provider }
}

// EngineOption configures an engine before concurrent use.
type EngineOption func(*Engine)

func (e *Engine) prepareContext(ctx context.Context, request *Request) (*Request, error) {
	result := *request
	if result.Context != nil {
		return &result, nil
	}
	provider := e.contextProvider
	if provider == nil {
		provider = HostContextProvider{}
	}
	snapshot, err := provider.Collect(ctx)
	if err != nil {
		return nil, err
	}
	result.Context = snapshot
	return &result, nil
}

// FactCollector materializes requested external evidence before pure evaluation.
type FactCollector interface {
	CollectFacts(context.Context, *Request, []Rule) error
}

// WithFactCollector configures the host's optional provider evidence collector.
func WithFactCollector(collector FactCollector) EngineOption {
	defer perf.Track(nil, "policy.WithFactCollector")()

	return func(engine *Engine) { engine.factCollector = collector }
}
