package step

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/castplan"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	defaultDeclaredWriteRate   = 40 * time.Millisecond
	defaultDeclaredKeyInterval = 20 * time.Millisecond
)

type castCapture struct {
	mu   sync.Mutex
	text strings.Builder
	next iolib.Recorder
}

func (c *castCapture) Record(stream, text string) {
	defer perf.Track(nil, "step.castCapture.Record")()

	c.mu.Lock()
	defer c.mu.Unlock()
	if stream == "o" || stream == "e" {
		c.text.WriteString(text)
	}
	if c.next != nil {
		c.next.Record(stream, text)
	}
}
func (c *castCapture) output() string { c.mu.Lock(); defer c.mu.Unlock(); return c.text.String() }

func (r *declaredCast) action(action *castplan.Action, id string, visible bool) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if action.Kind == "hidden" {
		return r.unrecorded(action.Children, false)
	}
	child, err := r.child(action)
	if err != nil {
		return err
	}
	if err := r.typeCommand(action, child, visible); err != nil {
		return err
	}
	if visible {
		r.marker(castplan.Marker{Kind: "begin", ID: id})
	}
	err = r.executeAction(action, child, id, visible)
	if err != nil {
		return err
	}
	if visible {
		r.marker(castplan.Marker{Kind: "end", ID: id, ExitCode: action.ExitCode})
	}
	r.previousSession = action.Kind == "session" && visible
	return nil
}

func (r *declaredCast) typeCommand(action *castplan.Action, child *schema.WorkflowStep, visible bool) error {
	if action.Kind != "run" || !visible {
		return nil
	}
	typed, err := r.typing(action, child)
	if err != nil {
		return err
	}
	return runCastSimulateStep(r.ctx, r.step, typed, r.vars, r.previousSession)
}

func (r *declaredCast) executeAction(action *castplan.Action, child *schema.WorkflowStep, id string, visible bool) error {
	switch action.Kind {
	case "run":
		return r.command(action, child, visible)
	case "session":
		return r.session(action, child, id, visible)
	case "simulate":
		if !visible {
			return nil
		}
		return runCastSimulateStep(r.ctx, r.step, child, r.vars, r.previousSession)
	case "screenshot":
		if visible {
			iolib.RecordMarker(child.Path)
		}
		return nil
	case "pause":
		duration, err := parseDurationDefault(child.Duration, 0)
		if err != nil {
			return err
		}
		return sleepCastInput(r.ctx, duration)
	default:
		_, err := r.library.execute(r.ctx, child)
		return err
	}
}

func (r *declaredCast) typing(action *castplan.Action, child *schema.WorkflowStep) (*schema.WorkflowStep, error) {
	text := child.Command
	if display, exists := action.Fields["display"].(string); exists {
		text = display
	}
	fields := map[string]any{"text": text}
	for _, key := range []string{"prompt", "rate", "jitter", "cursor", "duration", "interval"} {
		if value, exists := action.Fields[key]; exists {
			fields[key] = value
		}
	}
	typed, err := decodeDeclaredAction(&castplan.Action{Kind: "simulate", Fields: fields})
	if err != nil {
		return nil, err
	}
	applyCastSimulateDefaults(r.step, typed)
	return typed, nil
}

func (r *declaredCast) command(action *castplan.Action, child *schema.WorkflowStep, visible bool) error {
	capture := &castCapture{}
	if visible {
		capture.next = r.recorder
	}
	restore := iolib.SetRecorder(capture)
	result, err := r.library.execute(r.ctx, child)
	restore()
	if r.ctx.Err() != nil {
		return r.ctx.Err()
	}
	code := 0
	if err != nil {
		if result == nil {
			return err
		}
		actual, ok := result.Metadata[exitCodeMetadata].(int)
		if !ok {
			return err
		}
		code = actual
	}
	if err := action.Check(ansi.Strip(capture.output()), code); err != nil {
		return err
	}
	delay, err := castStepPauseDelay(child)
	if err != nil {
		return err
	}
	return sleepCastInput(r.ctx, delay)
}

func (r *declaredCast) session(action *castplan.Action, child *schema.WorkflowStep, id string, visible bool) error {
	writeRate, err := parseDurationDefault(child.WriteRate, defaultDeclaredWriteRate)
	if err != nil {
		return err
	}
	keyInterval, err := parseDurationDefault(child.KeyInterval, defaultDeclaredKeyInterval)
	if err != nil {
		return err
	}
	env, err := castRecorderEnv(child, r.vars)
	if err != nil {
		return err
	}
	if err := applySessionPromptEnv(child, env); err != nil {
		return err
	}
	actions := make([]asciicast.SessionAction, 0, len(action.Children))
	for i := range action.Children {
		item, err := decodeDeclaredAction(&action.Children[i])
		if err != nil {
			return err
		}
		actions = append(actions, asciicast.SessionAction{
			Type: item.Type, Text: item.Text, Regex: item.Regex, Key: item.Key,
			Duration: item.Duration, Timeout: item.Timeout, Rate: item.Rate, Interval: item.Interval, Repeat: item.Repeat, Path: item.Path,
		})
	}
	return asciicast.RunSession(r.ctx, &asciicast.SessionOptions{
		Shell: child.Shell, Dir: child.WorkingDirectory, Env: env,
		Width: child.Width, Height: child.Height, WriteRate: writeRate, KeyInterval: keyInterval, Actions: actions, ScopedWaits: true,
		Observe: func(i int, item *asciicast.SessionAction) error {
			if visible && (item.Type == "wait" || item.Type == "show") {
				r.marker(castplan.Marker{Kind: item.Type, ID: fmt.Sprintf("%s.%d", id, i)})
			}
			return nil
		},
	})
}
