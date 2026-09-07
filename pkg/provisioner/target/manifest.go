package target

import (
	"bytes"

	"github.com/cloudposse/atmos/pkg/perf"
)

// MergeYAMLDocuments concatenates already-rendered YAML documents into a single
// "---\n"-separated multi-document stream, matching Kubernetes' native
// multi-document YAML convention. Used wherever a target must deliver several
// rendered manifests as one file rather than one file per manifest.
func MergeYAMLDocuments(docs [][]byte) []byte {
	defer perf.Track(nil, "target.MergeYAMLDocuments")()

	var buffer bytes.Buffer
	for i, doc := range docs {
		if i > 0 {
			buffer.WriteString("---\n")
		}
		buffer.Write(doc)
		if !bytes.HasSuffix(doc, []byte("\n")) {
			buffer.WriteByte('\n')
		}
	}
	return buffer.Bytes()
}
