package parser

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2/lexer"
)

// Position identifies a byte position in function arguments.
type Position struct {
	Offset int
	Line   int
	Column int
}

// Error describes an invalid function-argument grammar.
type Error struct {
	Position Position
	Message  string
}

// Error implements error.
func (e *Error) Error() string {
	return fmt.Sprintf("invalid function arguments at %d:%d: %s", e.Position.Line, e.Position.Column, e.Message)
}

// TerraformArgs is the parsed form of `component [stack] expression`.
// An empty Stack means the caller should use the execution-context stack.
type TerraformArgs struct {
	Component  string
	Stack      string
	Expression string
}

// EnvArgs contains an environment variable name and its optional default value.
type EnvArgs struct {
	Name    string
	Default string
}

// RandomArgs contains the optional minimum and maximum random values as source text.
type RandomArgs struct {
	Values []string
}

// IncludeArgs contains an include target and its optional YQ query.
type IncludeArgs struct {
	Path  string
	Query string
}

// StoreArgs contains the parsed !store arguments.
type StoreArgs struct {
	Store     string
	Stack     string
	Component string
	Key       string
	Default   *string
	Query     string
}

// StoreGetArgs contains the parsed !store.get arguments.
type StoreGetArgs struct {
	Store   string
	Key     string
	Default *string
	Query   string
}

const (
	tokenWhitespace = "Whitespace"
	tokenQuoted     = "Quoted"
	tokenPipe       = "Pipe"
	tokenText       = "Text"
)

var argumentLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: tokenWhitespace, Pattern: `\s+`},
	{Name: tokenQuoted, Pattern: `"(?:[^"\r\n]|"")*"|'(?:[^'\r\n]|'')*'`},
	{Name: tokenPipe, Pattern: `\|`},
	{Name: tokenText, Pattern: `[^\s|]+`},
})

type token struct {
	value    string
	typeName string
	position Position
}

func tokenize(input string) ([]token, error) {
	lex, err := argumentLexer.LexString("arguments", input)
	if err != nil {
		return nil, err
	}

	symbols := argumentLexer.Symbols()
	result := make([]token, 0)
	for {
		item, nextErr := lex.Next()
		if nextErr != nil {
			return nil, nextErr
		}
		if item.EOF() {
			return result, nil
		}
		typeName := symbolName(symbols, item.Type)
		if typeName == tokenWhitespace {
			continue
		}
		parsed := token{
			value:    item.Value,
			typeName: typeName,
			position: Position{Offset: item.Pos.Offset, Line: item.Pos.Line, Column: item.Pos.Column},
		}
		if typeName == tokenText && startsQuote(parsed.value) {
			return nil, parseError(parsed, "unterminated quoted value")
		}
		result = append(result, parsed)
	}
}

func startsQuote(value string) bool {
	return strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'")
}

func symbolName(symbols map[string]lexer.TokenType, tokenType lexer.TokenType) string {
	for name, candidate := range symbols {
		if candidate == tokenType {
			return name
		}
	}
	return "Unknown"
}

func parseError(item token, message string) error {
	return &Error{Position: item.position, Message: message}
}

func emptyError() error {
	return &Error{Position: Position{Line: 1, Column: 1}, Message: "expected arguments"}
}

func rawFrom(input string, item token) string {
	return strings.TrimSpace(input[item.position.Offset:])
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != value[len(value)-1] {
		return value
	}
	switch value[0] {
	case '\'':
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	case '"':
		return strings.ReplaceAll(value[1:len(value)-1], `""`, `"`)
	default:
		return value
	}
}

func isExpressionStart(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return strings.ContainsRune(".[{|'\"", rune(value[0]))
}

func normaliseLegacyCSV(input string) ([]string, bool) {
	parts, err := readLegacyCSV(input)
	if err != nil || len(parts) < 2 || len(parts) > 3 {
		return nil, false
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts, true
}

func isLegacyCSV(tokens []token) bool {
	if len(tokens) != 2 && len(tokens) != 3 {
		return false
	}
	expression := tokens[len(tokens)-1]
	return expression.typeName == tokenQuoted && strings.HasPrefix(expression.value, `"`) && strings.Contains(expression.value, `""`)
}

// ParseTerraform parses `component [stack] expression`.
func ParseTerraform(input string) (TerraformArgs, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return TerraformArgs{}, err
	}
	if isLegacyCSV(tokens) {
		if parts, ok := normaliseLegacyCSV(input); ok {
			return terraformFromLegacy(parts)
		}
	}
	if len(tokens) == 0 {
		return TerraformArgs{}, emptyError()
	}
	if len(tokens) == 1 {
		return TerraformArgs{}, parseError(tokens[0], "terraform function requires 2 or 3 arguments")
	}

	args := TerraformArgs{Component: unquote(tokens[0].value)}
	if len(tokens) == 2 || isExpressionStart(tokens[1].value) {
		args.Expression = unquote(rawFrom(input, tokens[1]))
		return validateTerraform(args, tokens[1])
	}
	if len(tokens) > 3 && !isExpressionStart(tokens[2].value) {
		return TerraformArgs{}, parseError(tokens[3], "terraform function requires 2 or 3 arguments")
	}
	args.Stack = unquote(tokens[1].value)
	args.Expression = unquote(rawFrom(input, tokens[2]))
	return validateTerraform(args, tokens[2])
}

func terraformFromLegacy(parts []string) (TerraformArgs, error) {
	if len(parts) == 2 {
		return validateTerraform(TerraformArgs{Component: parts[0], Expression: parts[1]}, token{})
	}
	if len(parts) == 3 {
		return validateTerraform(TerraformArgs{Component: parts[0], Stack: parts[1], Expression: parts[2]}, token{})
	}
	return TerraformArgs{}, emptyError()
}

func validateTerraform(args TerraformArgs, item token) (TerraformArgs, error) {
	if args.Component == "" {
		return TerraformArgs{}, parseError(item, "component must not be empty")
	}
	if args.Expression == "" {
		return TerraformArgs{}, parseError(item, "output expression must not be empty")
	}
	return args, nil
}

// ParseEnv parses `name [default]`.
func ParseEnv(input string) (EnvArgs, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return EnvArgs{}, err
	}
	if len(tokens) == 0 {
		return EnvArgs{}, emptyError()
	}
	if len(tokens) > 2 {
		return EnvArgs{}, parseError(tokens[2], "env function accepts 1 or 2 arguments")
	}
	args := EnvArgs{Name: unquote(tokens[0].value)}
	if len(tokens) == 2 {
		args.Default = unquote(tokens[1].value)
	}
	return args, nil
}

// ParseRandom parses zero, one, or two integer source values.
func ParseRandom(input string) (RandomArgs, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return RandomArgs{}, err
	}
	if len(tokens) > 2 {
		return RandomArgs{}, parseError(tokens[2], "random function accepts 0, 1, or 2 arguments")
	}
	args := RandomArgs{Values: make([]string, len(tokens))}
	for index, item := range tokens {
		args.Values[index] = unquote(item.value)
	}
	return args, nil
}

// ParseInclude parses `path [query]` and preserves whitespace in the query.
func ParseInclude(input string) (IncludeArgs, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return IncludeArgs{}, err
	}
	if len(tokens) == 0 {
		return IncludeArgs{}, emptyError()
	}
	args := IncludeArgs{Path: unquote(tokens[0].value)}
	if len(tokens) > 1 {
		args.Query = unquote(rawFrom(input, tokens[1]))
	}
	return args, nil
}

// ParseStore parses !store arguments and their optional default/query clauses.
func ParseStore(input string) (StoreArgs, error) {
	head, options, err := splitOptions(input)
	if err != nil {
		return StoreArgs{}, err
	}
	words, err := words(head)
	if err != nil {
		return StoreArgs{}, err
	}
	if len(words) != 3 && len(words) != 4 {
		return StoreArgs{}, invalidArity(words, "store function requires 3 or 4 parameters")
	}
	args := StoreArgs{Store: words[0]}
	if len(words) == 3 {
		args.Component, args.Key = words[1], words[2]
	} else {
		args.Stack, args.Component, args.Key = words[1], words[2], words[3]
	}
	args.Default, args.Query = options.defaultValue, options.query
	return args, nil
}

// ParseStoreGet parses !store.get arguments and optional default/query clauses.
func ParseStoreGet(input string) (StoreGetArgs, error) {
	head, options, err := splitOptions(input)
	if err != nil {
		return StoreGetArgs{}, err
	}
	words, err := words(head)
	if err != nil {
		return StoreGetArgs{}, err
	}
	if len(words) != 2 {
		return StoreGetArgs{}, invalidArity(words, "store.get function requires 2 parameters")
	}
	return StoreGetArgs{Store: words[0], Key: words[1], Default: options.defaultValue, Query: options.query}, nil
}

func words(input string) ([]string, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(tokens))
	for index, item := range tokens {
		if item.typeName == tokenPipe {
			return nil, parseError(item, "unexpected pipe")
		}
		result[index] = unquote(item.value)
	}
	return result, nil
}

func invalidArity(values []string, message string) error {
	if len(values) == 0 {
		return emptyError()
	}
	return &Error{Position: Position{Line: 1, Column: 1}, Message: message}
}

type options struct {
	defaultValue *string
	query        string
}

func splitOptions(input string) (string, options, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return "", options{}, err
	}
	for index, item := range tokens {
		if item.typeName != tokenPipe || index+1 >= len(tokens) {
			continue
		}
		keyword := unquote(tokens[index+1].value)
		if keyword == "default" || keyword == "query" {
			return parseOptions(input, tokens, index)
		}
	}
	return input, options{}, nil
}

func parseOptions(input string, tokens []token, firstOption int) (string, options, error) {
	head := strings.TrimSpace(input[:tokens[firstOption].position.Offset])
	result := options{}
	for index := firstOption; index < len(tokens); {
		if tokens[index].typeName != tokenPipe || index+1 >= len(tokens) {
			return "", options{}, parseError(tokens[index], "expected option delimiter")
		}
		keyword := unquote(tokens[index+1].value)
		if keyword != "default" && keyword != "query" {
			return "", options{}, parseError(tokens[index+1], "expected default or query option")
		}
		if index+2 >= len(tokens) {
			return "", options{}, parseError(tokens[index+1], "expected option value")
		}
		next := nextOption(tokens, index+2)
		if keyword == "default" && index+3 != next {
			return "", options{}, parseError(tokens[index+3], "default option accepts one value")
		}
		end := optionEnd(tokens, index+2)
		value := unquote(strings.TrimSpace(input[tokens[index+2].position.Offset:end]))
		if value == "" {
			return "", options{}, parseError(tokens[index+2], "option value must not be empty")
		}
		if keyword == "default" {
			result.defaultValue = &value
		} else {
			result.query = value
		}
		index = next
		if index == len(tokens) {
			break
		}
	}
	return head, result, nil
}

func optionEnd(tokens []token, valueStart int) int {
	next := nextOption(tokens, valueStart)
	if next == len(tokens) {
		return tokens[len(tokens)-1].position.Offset + len(tokens[len(tokens)-1].value)
	}
	return tokens[next].position.Offset
}

func nextOption(tokens []token, start int) int {
	for index := start; index+1 < len(tokens); index++ {
		if tokens[index].typeName == tokenPipe {
			keyword := unquote(tokens[index+1].value)
			if keyword == "default" || keyword == "query" {
				return index
			}
		}
	}
	return len(tokens)
}

// readLegacyCSV is isolated from the grammar so legacy CSV compatibility cannot
// leak into callers or new syntaxes.
func readLegacyCSV(input string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(input))
	reader.Comma = ' '
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	return reader.Read()
}
