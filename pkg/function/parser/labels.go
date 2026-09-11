package parser

import "github.com/cloudposse/atmos/pkg/perf"

// LabelsArgs selects the full label map (Key == nil), or a literal key with
// an optional fallback. A pointer distinguishes an empty fallback from none.
type LabelsArgs struct {
	Key     *string
	Default *string
}

// ParseLabels parses `[key [default]]` using the shared argument quoting rules.
func ParseLabels(input string) (LabelsArgs, error) {
	defer perf.Track(nil, "parser.ParseLabels")()

	tokens, err := tokenize(input)
	if err != nil {
		return LabelsArgs{}, err
	}
	if len(tokens) > 2 {
		return LabelsArgs{}, parseError(tokens[2], "labels function accepts 0, 1, or 2 arguments")
	}
	args := LabelsArgs{}
	if len(tokens) == 0 {
		return args, nil
	}
	for _, item := range tokens {
		if item.typeName == tokenPipe {
			return LabelsArgs{}, parseError(item, "labels function accepts literal keys and defaults, not a query")
		}
	}
	key := unquote(tokens[0].value)
	if key == "" {
		return LabelsArgs{}, parseError(tokens[0], "label key must not be empty")
	}
	args.Key = &key
	if len(tokens) == 2 {
		fallback := unquote(tokens[1].value)
		args.Default = &fallback
	}
	return args, nil
}
