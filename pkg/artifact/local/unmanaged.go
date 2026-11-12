package local

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// rejectUnmanagedPrior refuses to retire a directory containing files outside its Record.
func (r *Repository) rejectUnmanagedPrior(t *tree, job *publishJob, plan *publishPlan) error {
	if r.kind != artifact.RepositoryKindLocalDir || plan.prior == nil || plan.prior.Location == "" {
		return nil
	}
	record, _, err := t.loadRecord(plan.prior.RecordDigest)
	if err != nil {
		return err
	}
	priorRecords := recordsForPrior(record, job, plan)
	if err := checkOwnedDirectory(t, plan.prior.Location, priorRecords...); err != nil {
		return err
	}
	locations := []string{job.retiredDir() + "/payload"}
	if plan.oldOp != nil {
		locations = append(locations, layout.StagingPath(plan.oldOp.OperationID)+"/retired/payload")
	}
	for _, location := range locations {
		if err := checkOwnedDirectory(t, location, record); err != nil {
			return err
		}
	}
	return nil
}

func recordsForPrior(record *artifact.Record, job *publishJob, plan *publishPlan) []*artifact.Record {
	if plan.admission.Resuming() && layout.FoldPath(plan.prior.Location) == layout.FoldPath(job.location) {
		return []*artifact.Record{record, job.record}
	}
	return []*artifact.Record{record}
}

func checkOwnedDirectory(t *tree, location string, records ...*artifact.Record) error {
	allowed := map[string]bool{location: true}
	add := func(relative string) {
		//nolint:forbidigo // io/fs paths are slash-separated on all platforms.
		for name := path.Join(location, relative); name != location && name != "."; name = path.Dir(name) {
			allowed[name] = true
		}
	}
	for _, record := range records {
		for _, entry := range record.Inventory {
			add(entry.Path)
		}
	}
	for _, name := range []string{layout.ManifestName, "README.md", "CHANGELOG.md"} {
		add(layout.DistributionDir + "/" + name)
	}
	err := fs.WalkDir(t.FS(), location, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !allowed[name] {
			return fmt.Errorf("%w: cannot replace %q: %q is not managed by the artifact", errUtils.ErrArtifactConflict, location, name)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
