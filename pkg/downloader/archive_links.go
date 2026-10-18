package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"strconv"

	"github.com/hashicorp/go-getter"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// linkAudit summarizes the symbolic and hard link entries of one archive.
type linkAudit struct {
	// Count is the number of link entries.
	Count int
	// First is the archive path of the first link entry.
	First string
}

// warnArchiveLinks reports an audit with links in it. It is a variable so tests can observe it.
var warnArchiveLinks = func(audit linkAudit) {
	log.Warn("Archive contains symbolic or hard links; go-getter extracts each as an empty regular file instead of a link, so the vendored copy will not contain the link targets",
		"links", strconv.Itoa(audit.Count), "first", audit.First)
}

// linkAuditingDecompressor decorates a go-getter decompressor. The go-getter tar decompressors write
// the (empty) body of a link entry to a regular file, and its zip decompressor does the same for
// symlinks, so a vendored archive silently loses its links. The decorator scans the archive first
// and warns once per archive; extraction itself is delegated unchanged.
type linkAuditingDecompressor struct {
	ext   string
	inner getter.Decompressor
}

// Decompress audits src for link entries, warns once if there are any, then extracts it unchanged.
func (d *linkAuditingDecompressor) Decompress(dst, src string, dir bool, umask os.FileMode) error {
	if audit, err := auditArchiveLinks(src, d.ext); err != nil {
		log.Debug("Could not scan archive for links", "extension", d.ext, "error", err)
	} else if audit.Count > 0 {
		warnArchiveLinks(audit)
	}
	return d.inner.Decompress(dst, src, dir, umask)
}

// linkAuditedDecompressors returns go-getter's decompressors, each wrapped to warn about link entries.
func linkAuditedDecompressors() map[string]getter.Decompressor {
	wrapped := make(map[string]getter.Decompressor, len(getter.Decompressors))
	for ext, decompressor := range getter.Decompressors {
		wrapped[ext] = &linkAuditingDecompressor{ext: ext, inner: decompressor}
	}
	return wrapped
}

// auditArchiveLinks counts link entries in a tar (optionally gzip or bzip2 compressed) or zip
// archive. Formats it cannot read (for example xz and zstd) yield an empty audit and no error.
func auditArchiveLinks(src, ext string) (linkAudit, error) {
	switch ext {
	case "zip":
		return auditZipLinks(src)
	case "tar", "tar.gz", "tgz", "tar.bz2", "tbz2":
		return auditTarLinks(src, ext)
	default:
		return linkAudit{}, nil
	}
}

// auditTarLinks scans a tar archive's headers.
func auditTarLinks(src, ext string) (linkAudit, error) {
	file, err := os.Open(src)
	if err != nil {
		return linkAudit{}, err
	}
	defer file.Close()

	var reader io.Reader = file
	switch ext {
	case "tar.gz", "tgz":
		gz, err := gzip.NewReader(file)
		if err != nil {
			return linkAudit{}, err
		}
		defer gz.Close()
		reader = gz
	case "tar.bz2", "tbz2":
		reader = bzip2.NewReader(file)
	}

	var audit linkAudit
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return audit, nil
		}
		if err != nil {
			return audit, err
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			audit.add(header.Name)
		}
	}
}

// auditZipLinks scans a zip archive's central directory.
func auditZipLinks(src string) (linkAudit, error) {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return linkAudit{}, err
	}
	defer reader.Close()

	var audit linkAudit
	for _, entry := range reader.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			audit.add(entry.Name)
		}
	}
	return audit, nil
}

// add records one link entry.
func (a *linkAudit) add(name string) {
	if a.Count == 0 {
		a.First = name
	}
	a.Count++
}
