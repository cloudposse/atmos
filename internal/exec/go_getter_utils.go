// https://github.com/hashicorp/go-getter

package exec

import (
	"fmt"
	"strings"
)

var (
	ErrURIEmpty                      = fmt.Errorf("URI cannot be empty")
	ErrURIExceedsMaxLength           = fmt.Errorf("URI exceeds maximum length of 2048 characters")
	ErrURICannotContainPathTraversal = fmt.Errorf("URI cannot contain path traversal sequences")
	ErrURICannotContainSpaces        = fmt.Errorf("URI cannot contain spaces")
	ErrUnsupportedURIScheme          = fmt.Errorf("unsupported URI scheme")
	ErrInvalidOCIURIFormat           = fmt.Errorf("invalid OCI URI format")
)

const MaxURISize = 2048

// ValidateURI validates URIs.
func ValidateURI(uri string) error {
	if uri == "" {
		return ErrURIEmpty
	}
	// Maximum length check
	if len(uri) > MaxURISize {
		return ErrURIExceedsMaxLength
	}
	// Validate URI format
	if strings.Contains(uri, "..") {
		return ErrURICannotContainPathTraversal
	}
	if strings.Contains(uri, " ") {
		return ErrURICannotContainSpaces
	}
	// Validate scheme-specific format
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return ErrInvalidOCIURIFormat
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !IsValidScheme(scheme) {
			return ErrUnsupportedURIScheme
		}
	}
	return nil
}

// IsValidScheme checks if the URL scheme is valid
func IsValidScheme(scheme string) bool {
	validSchemes := map[string]bool{
		"http":       true,
		"https":      true,
		"git":        true,
		"ssh":        true,
		"git::https": true,
		"git::ssh":   true,
	}
	return validSchemes[scheme]
}

/*
Supported schemes:

file, dir, tar, zip
http, https
git, hg
s3, gcs
oci
scp, sftp
Shortcuts like github.com, bitbucket.org

- File-related Schemes:
file - Local filesystem paths
dir - Local directories
tar - Tar files, potentially compressed (tar.gz, tar.bz2, etc.)
zip - Zip files

- HTTP/HTTPS:
http - HTTP URLs
https - HTTPS URLs

- Git:
git - Git repositories, which can be accessed via HTTPS or SSH

- Mercurial:
hg - Mercurial repositories, accessed via HTTP/S or SSH

- Amazon S3:
s3 - Amazon S3 bucket URLs

- Google Cloud Storage:
gcs - Google Cloud Storage URLs

- OCI:
oci - Open Container Initiative (OCI) images

- Other Protocols:
scp - Secure Copy Protocol for SSH-based transfers
sftp - SSH File Transfer Protocol

- GitHub/Bitbucket/Other Shortcuts:
github.com - Direct GitHub repository shortcuts
bitbucket.org - Direct Bitbucket repository shortcuts

- Composite Schemes:
go-getter allows for composite schemes, where multiple operations can be combined. For example:
git::https://github.com/user/repo - Forces the use of git over an HTTPS URL.
tar::http://example.com/archive.tar.gz - Treats the HTTP resource as a tarball.

*/
