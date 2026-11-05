package layout

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/Masterminds/sprig/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/templatefuncs"
)

// maxLocationLength bounds one resolved location, in bytes.
const maxLocationLength = 1024

// windowsReserved lists characters that are invalid in Windows file names. Locations avoid them on
// every platform so a repository root can be moved between hosts.
const windowsReserved = `<>:"|?*`

// deleteRune is the ASCII DEL control character.
const deleteRune = 0x7f

// nonHermeticFunctions are Sprig functions removed from key_pattern rendering. A location must
// depend only on the captured publication inputs: not on the clock, the process environment, the
// network, or randomness.
var nonHermeticFunctions = []string{
	"now", "env", "expandenv", "getHostByName", "uuidv4", "randAlphaNum", "randAlpha", "randNumeric",
	"randAscii", "randBytes", "shuffle", "genPrivateKey", "genCA", "genCAWithKey", "genSelfSignedCert",
	"genSelfSignedCertWithKey", "genSignedCert", "genSignedCertWithKey", "buildCustomCert", "derivePassword",
	"htpasswd", "bcrypt", "encryptAES", "decryptAES",
}

// PatternInput holds the values a key_pattern may reference: .Name, .Version, .Kind, and
// .CreatedAt. CreatedAt is the creation time captured once for a publication.
type PatternInput struct {
	// Name is the artifact name.
	Name string

	// Version is the artifact version.
	Version string

	// Kind is the artifact kind.
	Kind string

	// CreatedAt is the captured creation time; it is rendered in UTC.
	CreatedAt time.Time
}

// patternData is the template data. CreatedAt is a time.Time so date formatting applies.
type patternData struct {
	Name      string
	Version   string
	Kind      string
	CreatedAt time.Time
}

// DefaultLocation returns the default payload location below the repository root for the
// packaging: "<name>/<version>" for copied files, "<name>/<version>.<ext>" for an archive.
func DefaultLocation(name, version string, packaging artifact.Packaging) string {
	defer perf.Track(nil, "layout.DefaultLocation")()

	return name + "/" + version + ArchiveExtension(packaging)
}

// ArchiveExtension returns the file extension (with the dot) of an archive packaging, or the empty
// string for copied files and unknown packagings.
func ArchiveExtension(packaging artifact.Packaging) string {
	defer perf.Track(nil, "layout.ArchiveExtension")()

	switch packaging {
	case artifact.PackagingTar:
		return ".tar"
	case artifact.PackagingTgz:
		return ".tgz"
	case artifact.PackagingZip:
		return ".zip"
	default:
		return ""
	}
}

// RenderLocation resolves the payload location for one publication. An empty pattern selects
// [DefaultLocation]. A non-empty pattern is rendered as a Go template with the Sprig functions
// (minus the non-hermetic ones) and a UTC "date". The result is validated: it must stay below the
// repository root, avoid the reserved namespace, and for archives agree with the packaging.
//
// Callers render once, at publication, persist the result in the representation record, and use
// the persisted value for every later read, delete, and retry; they never render again.
func RenderLocation(pattern string, in *PatternInput, packaging artifact.Packaging) (string, error) {
	defer perf.Track(nil, "layout.RenderLocation")()

	raw := DefaultLocation(in.Name, in.Version, packaging)
	if pattern != "" {
		rendered, err := renderPattern(pattern, in)
		if err != nil {
			return "", err
		}
		raw = rendered
	}
	location, err := CleanLocation(raw)
	if err != nil {
		return "", fmt.Errorf("key_pattern %q rendered %q: %w", pattern, raw, err)
	}
	if err := checkExtension(location, packaging); err != nil {
		return "", fmt.Errorf("key_pattern %q rendered %q: %w", pattern, raw, err)
	}
	return location, nil
}

func renderPattern(pattern string, in *PatternInput) (string, error) {
	funcs := sprig.TxtFuncMap()
	for _, name := range nonHermeticFunctions {
		delete(funcs, name)
	}
	funcs["date"] = utcDate
	for name, fn := range templatefuncs.FuncMap() {
		funcs[name] = fn
	}
	tmpl, err := template.New("key_pattern").Funcs(funcs).Option("missingkey=error").Parse(pattern)
	if err != nil {
		return "", fmt.Errorf("%w: key_pattern %q: %w", errUtils.ErrArtifactConfigInvalid, pattern, err)
	}
	data := patternData{Name: in.Name, Version: in.Version, Kind: in.Kind, CreatedAt: in.CreatedAt.UTC()}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("%w: key_pattern %q: %w", errUtils.ErrArtifactConfigInvalid, pattern, err)
	}
	return out.String(), nil
}

// utcDate formats a time in UTC. Sprig's date uses the local zone, which would make the persisted
// location depend on the host that published it.
func utcDate(format string, value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC().Format(format)
	case *time.Time:
		if typed != nil {
			return typed.UTC().Format(format)
		}
	}
	return ""
}

// CleanLocation validates a resolved or persisted payload location and returns it in canonical
// form (a single trailing slash is removed). A location is relative, slash separated, free of
// empty, "." and ".." segments, control and Windows-reserved characters, and never enters the
// reserved namespace, however the case is written. Locations are never rewritten otherwise.
func CleanLocation(location string) (string, error) {
	defer perf.Track(nil, "layout.CleanLocation")()

	location = strings.TrimSuffix(location, "/")
	if err := checkLocationShape(location); err != nil {
		return "", err
	}
	for _, segment := range strings.Split(location, "/") {
		switch {
		case segment == "" || segment == "." || segment == "..":
			return "", fmt.Errorf("%w: location %q has an empty, '.', or '..' segment", errUtils.ErrArtifactUnsafePath, location)
		case strings.EqualFold(segment, artifact.ReservedNamespace):
			return "", fmt.Errorf("%w: location %q enters the reserved %s namespace", errUtils.ErrArtifactUnsafePath, location, artifact.ReservedNamespace)
		}
	}
	return location, nil
}

// checkLocationShape checks length, encoding, rootedness, separators, and character classes.
func checkLocationShape(location string) error {
	switch {
	case location == "" || len(location) > maxLocationLength:
		return fmt.Errorf("%w: location must be 1-%d bytes", errUtils.ErrArtifactUnsafePath, maxLocationLength)
	case !utf8.ValidString(location):
		return fmt.Errorf("%w: location %q is not valid UTF-8", errUtils.ErrArtifactUnsafePath, location)
	case strings.HasPrefix(location, "/") || strings.Contains(location, `\`):
		return fmt.Errorf("%w: location %q must be relative and slash-separated", errUtils.ErrArtifactUnsafePath, location)
	case strings.ContainsAny(location, windowsReserved) || strings.IndexFunc(location, isControl) >= 0:
		return fmt.Errorf("%w: location %q contains control or reserved characters", errUtils.ErrArtifactUnsafePath, location)
	}
	return nil
}

func isControl(r rune) bool {
	return r < ' ' || r == deleteRune
}

// archiveSuffixes maps archive file extensions to their packaging, longest first.
var archiveSuffixes = []struct {
	suffix    string
	packaging artifact.Packaging
}{
	{".tar.gz", artifact.PackagingTgz},
	{".tgz", artifact.PackagingTgz},
	{".tar", artifact.PackagingTar},
	{".zip", artifact.PackagingZip},
}

// checkExtension rejects an archive extension that disagrees with the packaging. A location
// without an archive extension is accepted; it is never silently renamed.
func checkExtension(location string, packaging artifact.Packaging) error {
	if ArchiveExtension(packaging) == "" {
		return nil
	}
	lower := strings.ToLower(location)
	for _, candidate := range archiveSuffixes {
		if !strings.HasSuffix(lower, candidate.suffix) {
			continue
		}
		if candidate.packaging != packaging {
			return fmt.Errorf("%w: %w: location ends in %q, which does not match packaging %q", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactPackagingInvalid, candidate.suffix, packaging)
		}
		return nil
	}
	return nil
}
