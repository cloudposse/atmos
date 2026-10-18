package progress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/approval"
)

// fakeIndicator records the calls made to the spinner.
type fakeIndicator struct {
	calls       []string
	onInterrupt func()
}

func (f *fakeIndicator) Start()                             { f.calls = append(f.calls, "start") }
func (f *fakeIndicator) Stop()                              { f.calls = append(f.calls, "stop") }
func (f *fakeIndicator) Update(message string)              { f.calls = append(f.calls, "update:"+message) }
func (f *fakeIndicator) SetInterruptHandler(handler func()) { f.onInterrupt = handler }

// fakeReporter is a provider that reports progress events.
type fakeReporter struct {
	handler func(approval.Event)
}

func (f *fakeReporter) SetProgressHandler(handler func(approval.Event)) { f.handler = handler }

var _ approval.ProgressReporter = (*fakeReporter)(nil)

const thinking = "Thinking…"

func TestProgress_StartStopLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		run         func(p *Progress)
		want        []string
	}{
		{
			name:        "start then stop",
			interactive: true,
			run:         func(p *Progress) { p.Start(); p.Stop() },
			want:        []string{"start", "stop"},
		},
		{
			name:        "second start does not start the spinner twice",
			interactive: true,
			run:         func(p *Progress) { p.Start(); p.Start(); p.Stop() },
			want:        []string{"start", "stop"},
		},
		{
			name:        "stop without start does nothing",
			interactive: true,
			run:         func(p *Progress) { p.Stop() },
			want:        nil,
		},
		{
			name:        "stop is idempotent",
			interactive: true,
			run:         func(p *Progress) { p.Start(); p.Stop(); p.Stop() },
			want:        []string{"start", "stop"},
		},
		{
			name:        "outside a terminal the message is emitted once as a status line",
			interactive: false,
			run:         func(p *Progress) { p.Start(); p.Stop() },
			want:        []string{"start", "update:" + thinking, "stop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ind := &fakeIndicator{}
			tt.run(newProgress(ind, thinking, tt.interactive))
			assert.Equal(t, tt.want, ind.calls)
		})
	}
}

func TestProgress_PauseResume(t *testing.T) {
	tests := []struct {
		name string
		run  func(p *Progress)
		want []string
	}{
		{
			name: "pause and resume restart the spinner",
			run:  func(p *Progress) { p.Start(); p.Pause(); p.Resume(); p.Stop() },
			want: []string{"start", "stop", "start", "stop"},
		},
		{
			name: "resume after stop does not bring the spinner back",
			run:  func(p *Progress) { p.Start(); p.Stop(); p.Resume() },
			want: []string{"start", "stop"},
		},
		{
			name: "pause before start does nothing",
			run:  func(p *Progress) { p.Pause(); p.Resume() },
			want: nil,
		},
		{
			name: "pause twice stops once",
			run:  func(p *Progress) { p.Start(); p.Pause(); p.Pause(); p.Resume() },
			want: []string{"start", "stop", "start"},
		},
		{
			name: "resume while running does not start twice",
			run:  func(p *Progress) { p.Start(); p.Resume() },
			want: []string{"start"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ind := &fakeIndicator{}
			tt.run(newProgress(ind, thinking, true))
			assert.Equal(t, tt.want, ind.calls)
		})
	}
}

func TestProgress_PromptHooksPauseAndResume(t *testing.T) {
	ind := &fakeIndicator{}
	p := newProgress(ind, thinking, true)
	before, after := p.PromptHooks()

	p.Start()
	before()
	after()

	assert.Equal(t, []string{"start", "stop", "start"}, ind.calls)
}

func TestProgress_HandleEvents(t *testing.T) {
	running := "Running `atmos list stacks`…"

	tests := []struct {
		name        string
		interactive bool
		events      []approval.Event
		want        []string
	}{
		{
			name:        "tool start shows the summary and tool done goes back to thinking",
			interactive: true,
			events: []approval.Event{
				{Kind: approval.ToolStart, Tool: "Bash", Summary: "atmos list stacks"},
				{Kind: approval.ToolDone, Tool: "Bash"},
			},
			want: []string{"start", "update:" + running, "update:" + thinking},
		},
		{
			name:        "tool start without a summary falls back to the tool name",
			interactive: true,
			events:      []approval.Event{{Kind: approval.ToolStart, Tool: "Bash"}},
			want:        []string{"start", "update:Running `Bash`…"},
		},
		{
			name:        "outside a terminal only tool starts produce a status line",
			interactive: false,
			events: []approval.Event{
				{Kind: approval.ToolStart, Tool: "Bash", Summary: "atmos list stacks"},
				{Kind: approval.ToolDone, Tool: "Bash"},
			},
			want: []string{"start", "update:" + thinking, "update:" + running},
		},
		{
			name:        "unknown event kinds are ignored",
			interactive: true,
			events:      []approval.Event{{Kind: approval.EventKind(99), Tool: "Bash"}},
			want:        []string{"start"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ind := &fakeIndicator{}
			p := newProgress(ind, thinking, tt.interactive)
			p.Start()
			for _, event := range tt.events {
				p.Handle(event)
			}
			assert.Equal(t, tt.want, ind.calls)
		})
	}
}

func TestProgress_HandleWhilePausedIsRememberedNotShown(t *testing.T) {
	ind := &fakeIndicator{}
	p := newProgress(ind, thinking, true)
	p.Start()
	p.Pause()

	p.Handle(approval.Event{Kind: approval.ToolStart, Tool: "Bash", Summary: "atmos list stacks"})
	assert.Equal(t, []string{"start", "stop"}, ind.calls, "a paused spinner must not be updated")

	p.Resume()
	assert.Equal(t, []string{"start", "stop", "start", "update:Running `atmos list stacks`…"}, ind.calls,
		"resume restores the latest tool label")
}

func TestProgress_HandleAfterStopIsIgnored(t *testing.T) {
	ind := &fakeIndicator{}
	p := newProgress(ind, thinking, true)
	p.Start()
	p.Stop()

	p.Handle(approval.Event{Kind: approval.ToolStart, Tool: "Bash"})

	assert.Equal(t, []string{"start", "stop"}, ind.calls)
}

func TestProgress_Attach(t *testing.T) {
	t.Run("subscribes to providers that report progress", func(t *testing.T) {
		ind := &fakeIndicator{}
		p := newProgress(ind, thinking, true)
		reporter := &fakeReporter{}

		require.True(t, p.Attach(reporter))
		require.NotNil(t, reporter.handler)

		p.Start()
		reporter.handler(approval.Event{Kind: approval.ToolStart, Tool: "Bash", Summary: "atmos list stacks"})
		assert.Equal(t, []string{"start", "update:Running `atmos list stacks`…"}, ind.calls)
	})

	t.Run("ignores providers that do not report progress", func(t *testing.T) {
		p := newProgress(&fakeIndicator{}, thinking, true)
		assert.False(t, p.Attach(struct{}{}))
	})
}

func TestProgress_OnInterrupt(t *testing.T) {
	ind := &fakeIndicator{}
	p := newProgress(ind, thinking, true)

	called := false
	p.OnInterrupt(func() { called = true })

	require.NotNil(t, ind.onInterrupt, "the handler is passed to the spinner")
	ind.onInterrupt()
	assert.True(t, called)
}
