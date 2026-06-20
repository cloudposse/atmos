package workflow

import (
	"sort"
	"strings"
)

func expandMatrix(matrix map[string][]string) []map[string]string {
	axes := make([]string, 0, len(matrix))
	for axis := range matrix {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	rows := []map[string]string{{}}
	for _, axis := range axes {
		next := make([]map[string]string, 0, len(rows)*len(matrix[axis]))
		for _, row := range rows {
			for _, value := range matrix[axis] {
				copied := make(map[string]string, len(row)+1)
				for k, v := range row {
					copied[k] = v
				}
				copied[axis] = value
				next = append(next, copied)
			}
		}
		rows = next
	}
	return rows
}

func matrixRowSuffix(row map[string]string) string {
	axes := make([]string, 0, len(row))
	for axis := range row {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	parts := make([]string, 0, len(axes))
	for _, axis := range axes {
		parts = append(parts, sanitizeControlName(row[axis]))
	}
	return strings.Join(parts, controlNameSep)
}

func sanitizeControlName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), controlNameSep)
}

func controlPrefix(outputCfg controlOutputConfig, stepName string, matrix map[string]string, dataFunc ControlTemplateDataFunc) string {
	prefix, err := resolveControlTemplate(outputCfg.prefix, stepName, matrix, dataFunc)
	if err != nil || strings.TrimSpace(prefix) == "" {
		return stepName
	}
	return prefix
}
