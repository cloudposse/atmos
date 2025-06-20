package exec

import (
	"archive/tar"
	"testing"
)

func TestProcessTarHeaderUnsupportedType(t *testing.T) {
	dir := t.TempDir()
	hdr := &tar.Header{Name: "link", Typeflag: tar.TypeSymlink}
	if err := processTarHeader(hdr, nil, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
