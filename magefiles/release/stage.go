package release

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	releaseFileMode      = 0o600
	releaseDirectoryMode = 0o700
	// Artifact types in the pinned GoReleaser v2.18.3 manifest schema.
	artifactUploadBinary = 2
	artifactLinuxPackage = 6
	artifactChecksum     = 12
	artifactSignature    = 13
	artifactCertificate  = 14
	artifactSBOM         = 29
)

type goreleaserArtifact struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type int    `json:"internal_type"`
}

// StageArtifacts copies GoReleaser's uploadable files under their public asset
// names. Binary-format archives keep their original build paths in dist; the
// public name exists only in artifacts.json, not as a top-level file on disk.
func (Release) StageArtifacts(source, destination string) error {
	data, err := os.ReadFile(filepath.Join(source, "dist", "artifacts.json"))
	if err != nil {
		return err
	}
	var artifacts []goreleaserArtifact
	if err = json.Unmarshal(data, &artifacts); err != nil {
		return err
	}
	if err = os.Mkdir(destination, releaseDirectoryMode); err != nil {
		return err // A staging directory must start empty, including on retries.
	}
	for _, artifact := range artifacts {
		// GoReleaser v2.18.3's uploadable binary, Linux package, checksum,
		// signature, certificate and SBOM types. Raw build binaries (4) have
		// duplicate paths and are deliberately excluded. Pin/schema tested.
		switch artifact.Type {
		case artifactUploadBinary, artifactLinuxPackage, artifactChecksum, artifactSignature, artifactCertificate, artifactSBOM:
			if err = stageArtifact(source, destination, artifact); err != nil {
				return err
			}
		}
	}
	for _, name := range []string{"metadata.json", "atmos-sbom.spdx.json"} {
		artifact := goreleaserArtifact{Name: name, Path: filepath.Join("dist", name)}
		if err = stageArtifact(source, destination, artifact); err != nil {
			return err
		}
	}
	return nil
}

func stageArtifact(source, destination string, artifact goreleaserArtifact) error {
	if !validAssetName(artifact.Name) {
		return fmt.Errorf("%w: unsafe asset name %q", ErrReleaseVerification, artifact.Name)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(source, "dist"))
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	path := artifact.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(source, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		return fmt.Errorf("%w: artifact path escapes dist: %q", ErrReleaseVerification, artifact.Path)
	}
	return copyArtifact(resolved, filepath.Join(destination, artifact.Name))
}

func validAssetName(name string) bool {
	return name != "." && name != ".." && name != "" &&
		!strings.ContainsAny(name, `/\`) && assetName.MatchString(name)
}

func copyArtifact(source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, releaseFileMode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, file)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
