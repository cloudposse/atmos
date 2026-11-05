package bench

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ARM Linux commonly exposes CPU identity as MIDR fields rather than model name.
// Exclude processor indices and core count so identical CPUs have stable identities.
func benchCPUIdentity(cpuinfo string) (string, error) {
	for line := range strings.SplitSeq(cpuinfo, "\n") {
		key, value, _ := strings.Cut(line, ":")
		if strings.TrimSpace(key) == "model name" && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return benchARMCPUIdentity(cpuinfo)
}

func benchARMCPUIdentity(cpuinfo string) (string, error) {
	fields := []string{"CPU implementer", "CPU architecture", "CPU variant", "CPU part", "CPU revision"}
	identities := map[string]bool{}
	values := map[string]string{}
	flush := func() error {
		if len(values) == 0 {
			return nil
		}
		tuple, err := benchARMTuple(fields, values)
		if err != nil {
			return err
		}
		identities[tuple] = true
		clear(values)
		return nil
	}
	for line := range strings.SplitSeq(cpuinfo, "\n") {
		key, value, _ := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if key == "processor" || strings.TrimSpace(line) == "" {
			if err := flush(); err != nil {
				return "", err
			}
		}
		if slices.Contains(fields, key) {
			values[key] = benchCPUField(value)
		}
	}
	if err := flush(); err != nil {
		return "", err
	}
	if len(identities) == 0 {
		return "", Error("cannot identify benchmark CPU")
	}
	keys := slices.Sorted(maps.Keys(identities))
	return "ARM " + strings.Join(keys, "; "), nil
}

func benchARMTuple(fields []string, values map[string]string) (string, error) {
	tuple := make([]string, 0, len(fields))
	for _, field := range fields {
		value := values[field]
		if value == "" {
			return "", Error("cannot identify benchmark CPU: missing %s", field)
		}
		tuple = append(tuple, strings.TrimPrefix(field, "CPU ")+"="+value)
	}
	return strings.Join(tuple, ","), nil
}

func benchCPUField(value string) string {
	const hexadecimal = 16
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	base := Decimal
	if strings.HasPrefix(value, "0x") {
		base, value = hexadecimal, strings.TrimPrefix(value, "0x")
	}
	if n, err := strconv.ParseUint(value, base, FloatBits); err == nil {
		return fmt.Sprint(n)
	}
	return value
}
