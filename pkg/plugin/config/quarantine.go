package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// QuarantineSuffix marks a file or directory moved aside because it failed verification.
const QuarantineSuffix = ".corrupt-"

// Quarantine renames a damaged file or directory to `<path>.corrupt-<unixts>` next to it.
// The damaged bytes stay available for inspection and are never deleted. A numeric
// suffix disambiguates repeated quarantines within the same second.
func Quarantine(path string, now time.Time) (string, error) {
	defer perf.Track(nil, "pluginconfig.Quarantine")()

	base := path + QuarantineSuffix + fmt.Sprint(now.Unix())
	target := base
	for attempt := 1; ; attempt++ {
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return "", err
		}
		target = fmt.Sprintf("%s-%d", base, attempt)
	}
	if err := os.Rename(path, target); err != nil {
		return "", err
	}
	return target, nil
}
