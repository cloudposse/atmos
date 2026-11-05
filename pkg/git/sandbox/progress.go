package sandbox

import "context"

// Progress describes checkout preparation without tying Git operations to a renderer.
type Progress struct {
	Phase            string
	Completed, Total int
}

func reportProgress(ctx context.Context, phase string, completed, total int) {
	if report := services(ctx).progress; report != nil {
		report(Progress{Phase: phase, Completed: completed, Total: total})
	}
}
