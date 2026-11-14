package templates

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// withoutDistributionMetadata removes the reserved distribution directory (`.atmos/artifact`) from a
// template's files. That directory holds the package README and CHANGELOG of a published scaffold
// (and the manifest of a packaged one); it describes the distribution, not the generated project, so
// no template source strategy (local directory, git, OCI, S3, embedded, or a rendered base re-render)
// may generate it. The root README is ordinary template content and is kept.
//
// The `.atmos` directory entry is dropped only when nothing else lives below it, so scaffolds that
// really keep files under `.atmos` are unaffected.
func withoutDistributionMetadata(files []File) []File {
	const parentPrefix = artifact.DistributionParent + "/"
	hasOtherAtmos := false
	for index := range files {
		name := files[index].Path
		if strings.HasPrefix(name, parentPrefix) && !isDistributionPath(name) {
			hasOtherAtmos = true
			break
		}
	}
	kept := make([]File, 0, len(files))
	for index := range files {
		name := files[index].Path
		if isDistributionPath(name) || (name == artifact.DistributionParent && files[index].IsDirectory && !hasOtherAtmos) {
			continue
		}
		kept = append(kept, files[index])
	}
	return kept
}

func isDistributionPath(name string) bool {
	return name == artifact.DistributionDir || strings.HasPrefix(name, artifact.DistributionDir+"/")
}
